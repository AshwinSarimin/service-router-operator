/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package routing

import (
	"context"
	"fmt"
	"reflect"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	externaldnsv1alpha1 "sigs.k8s.io/external-dns/apis/v1alpha1"
	externaldnsendpoint "sigs.k8s.io/external-dns/endpoint"

	clusterv1alpha1 "github.com/vecozo/service-router-operator/api/cluster/v1alpha1"
	routingv1alpha1 "github.com/vecozo/service-router-operator/api/routing/v1alpha1"
	"github.com/vecozo/service-router-operator/internal/clusteridentity"
	"github.com/vecozo/service-router-operator/internal/dnsconfiguration"
	"github.com/vecozo/service-router-operator/pkg/consts"
)

// ServiceRouteReconciler reconciles a ServiceRoute object
type ServiceRouteReconciler struct {
	client.Client
	Scheme                        *runtime.Scheme
	DefaultRouterGatewayNamespace string
}

//+kubebuilder:rbac:groups=routing.router.io,resources=serviceroutes,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=routing.router.io,resources=serviceroutes/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=routing.router.io,resources=serviceroutes/finalizers,verbs=update
//+kubebuilder:rbac:groups=externaldns.k8s.io,resources=dnsendpoints,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=routing.router.io,resources=dnspolicies,verbs=get;list;watch
//+kubebuilder:rbac:groups=routing.router.io,resources=gateways,verbs=get;list;watch
//+kubebuilder:rbac:groups=cluster.router.io,resources=clusteridentities,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.16.0/pkg/reconcile
func (r *ServiceRouteReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Retrieve the ServiceRoute to check for deletion or updates.
	// If the resource is missing, we must clean up any orphaned DNSEndpoints to avoid dangling DNS records.
	var serviceRoute routingv1alpha1.ServiceRoute
	if err := r.Get(ctx, req.NamespacedName, &serviceRoute); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("ServiceRoute deleted", "name", req.Name, "namespace", req.Namespace, "action", "cleaning up DNSEndpoints")
			if err := r.deleteDNSEndpointsForServiceRoute(ctx, req.NamespacedName); err != nil {
				logger.Error(err, "failed to delete DNSEndpoints for deleted ServiceRoute")
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, nil
		}
		logger.Error(err, "unable to fetch ServiceRoute")
		return ctrl.Result{}, err
	}

	// Validate to ensure we have a complete specification before attempting generation.
	if err := r.validateServiceRoute(&serviceRoute); err != nil {
		logger.Error(err, "validation failed")
		return r.updateStatusFailed(ctx, &serviceRoute, consts.ReasonValidationFailed, err.Error())
	}

	// Fetch the DNSPolicy for the namespace to determine the routing strategy.
	dnsPolicy, err := r.getDNSPolicyForNamespace(ctx, serviceRoute.Namespace)
	if err != nil {
		logger.Error(err, "failed to get DNSPolicy")
		return ctrl.Result{}, err
	}
	if dnsPolicy == nil {
		logger.Info("DNSPolicy not found, requeueing", "namespace", serviceRoute.Namespace)
		return r.updateStatusPending(ctx, &serviceRoute, consts.ReasonDNSPolicyNotFound,
			"Waiting for DNSPolicy to be configured in namespace")
	}

	// If the associated policy is inactive (e.g., wrong region), delete any existing endpoints.
	if !dnsPolicy.Status.Active {
		logger.Info("DNSPolicy is not active, cleaning up DNSEndpoints", "namespace", serviceRoute.Namespace)

		// Delete DNSEndpoints to prevent race conditions with external-dns controllers
		if err := r.deleteDNSEndpointsForServiceRoute(ctx, types.NamespacedName{
			Name:      serviceRoute.Name,
			Namespace: serviceRoute.Namespace,
		}); err != nil {
			logger.Error(err, "failed to delete DNSEndpoints for inactive DNSPolicy")
			return ctrl.Result{}, err
		}

		return r.updateStatusPending(ctx, &serviceRoute, consts.ReasonDNSPolicyInactive,
			"DNSPolicy is not active for this cluster (sourceRegion/sourceCluster mismatch). DNSEndpoints have been removed to prevent conflicts.")
	}

	// Need DNSConfiguration to map controller names to regions.
	// We use the cached configuration with CRD fallback to ensure availability.
	dnsConfig, err := dnsconfiguration.Fetch(ctx, r.Client)
	if err != nil {
		logger.Error(err, "failed to get DNSConfiguration")
		return r.updateStatusPending(ctx, &serviceRoute, consts.ReasonDNSConfigurationNotAvailable,
			"Waiting for DNSConfiguration to be configured")
	}
	if dnsConfig == nil {
		logger.Info("DNSConfiguration not available, requeueing")
		return r.updateStatusPending(ctx, &serviceRoute, consts.ReasonDNSConfigurationNotAvailable,
			"Waiting for DNSConfiguration to be configured")
	}

	// Fetch the Gateway to determine the target host and postfix.
	var gateway routingv1alpha1.Gateway
	gatewayNamespace := serviceRoute.Spec.GatewayNamespace
	if gatewayNamespace == "" {
		gatewayNamespace = r.DefaultRouterGatewayNamespace
	}
	if err := r.Get(ctx, client.ObjectKey{
		Name:      serviceRoute.Spec.GatewayName,
		Namespace: gatewayNamespace,
	}, &gateway); err != nil {
		if apierrors.IsNotFound(err) {
			return r.updateStatusPending(ctx, &serviceRoute, consts.ReasonGatewayNotFound,
				fmt.Sprintf("Gateway %s not found in namespace %s", serviceRoute.Spec.GatewayName, gatewayNamespace))
		}
		logger.Error(err, "failed to fetch Gateway")
		return ctrl.Result{}, err
	}

	// ClusterIdentity provides the domain and region information for hostname generation.
	clusterIdentity, err := clusteridentity.Fetch(ctx, r.Client)
	if err != nil {
		logger.Error(err, "failed to get ClusterIdentity")
		return r.updateStatusPending(ctx, &serviceRoute, consts.ReasonClusterIdentityNotAvailable,
			"Waiting for ClusterIdentity to be configured")
	}
	if clusterIdentity == nil {
		logger.Info("ClusterIdentity not available, requeueing")
		return r.updateStatusPending(ctx, &serviceRoute, consts.ReasonClusterIdentityNotAvailable,
			"Waiting for ClusterIdentity to be configured")
	}

	// Generate the desired DNSEndpoints based on the active controllers and route spec.
	dnsEndpoints, err := r.generateDNSEndpoints(&serviceRoute, dnsPolicy, &gateway, clusterIdentity, dnsConfig)
	if err != nil {
		logger.Error(err, "failed to generate DNSEndpoints")
		return r.updateStatusFailed(ctx, &serviceRoute, consts.ReasonDNSEndpointGenerationFailed, err.Error())
	}

	// Reconcile the DNSEndpoints to match the generated desired state.
	// This ensures that the actual cluster resources match what we calculated.
	if err := r.reconcileDNSEndpoints(ctx, &serviceRoute, dnsEndpoints); err != nil {
		logger.Error(err, "failed to reconcile DNSEndpoints")
		return ctrl.Result{}, err
	}

	// Reflect the successful reconciliation in the status.
	return r.updateStatusActive(ctx, &serviceRoute, dnsEndpoints)
}

// deleteDNSEndpointsForServiceRoute deletes DNSEndpoints created for the given ServiceRoute.
func (r *ServiceRouteReconciler) deleteDNSEndpointsForServiceRoute(ctx context.Context, namespacedName types.NamespacedName) error {
	logger := log.FromContext(ctx)

	var list externaldnsv1alpha1.DNSEndpointList
	if err := r.List(ctx, &list,
		client.InNamespace(namespacedName.Namespace),
		client.MatchingLabels{
			"app.kubernetes.io/managed-by": "service-router-operator",
			"router.io/serviceroute":       namespacedName.Name,
			"router.io/source-namespace":   namespacedName.Namespace,
		},
	); err != nil {
		return err
	}

	deletedCount := 0
	for i := range list.Items {
		de := &list.Items[i]
		if err := r.Delete(ctx, de); err != nil && !apierrors.IsNotFound(err) {
			logger.Error(err, "failed to delete DNSEndpoint", "dnsEndpoint", de.Name)
			return err
		}
		logger.Info("Deleted DNSEndpoint for ServiceRoute", "dnsEndpoint", de.Name)
		deletedCount++
	}

	if deletedCount > 0 {
		logger.Info("DNSEndpoint cleanup completed",
			"serviceRoute", namespacedName.Name,
			"namespace", namespacedName.Namespace,
			"deletedCount", deletedCount)
	} else {
		logger.V(1).Info("No DNSEndpoints found to delete (idempotent operation)",
			"serviceRoute", namespacedName.Name,
			"namespace", namespacedName.Namespace)
	}

	return nil
}

// validateServiceRoute validates the ServiceRoute specification
func (r *ServiceRouteReconciler) validateServiceRoute(serviceRoute *routingv1alpha1.ServiceRoute) error {
	if serviceRoute.Spec.ServiceName == "" {
		return fmt.Errorf("serviceName must be specified")
	}
	if serviceRoute.Spec.GatewayName == "" {
		return fmt.Errorf("gatewayName must be specified")
	}
	if serviceRoute.Spec.Environment == "" {
		return fmt.Errorf("environment must be specified")
	}
	if serviceRoute.Spec.Application == "" {
		return fmt.Errorf("application must be specified")
	}
	return nil
}

// getDNSPolicyForNamespace fetches the DNSPolicy for a namespace
func (r *ServiceRouteReconciler) getDNSPolicyForNamespace(ctx context.Context, namespace string) (*routingv1alpha1.DNSPolicy, error) {
	var dnsPolicies routingv1alpha1.DNSPolicyList
	if err := r.List(ctx, &dnsPolicies, client.InNamespace(namespace)); err != nil {
		return nil, err
	}

	if len(dnsPolicies.Items) == 0 {
		return nil, nil
	}

	// Return the first DNSPolicy found (namespace-scoped singleton)
	return &dnsPolicies.Items[0], nil
}

// generateDNSEndpoints generates DNSEndpoint resources based on active controllers.
//
// This function orchestrates the creation of DNSEndpoint CRDs that ExternalDNS will
// consume to create actual DNS records in Azure Private DNS (or other DNS providers).
//
// The DNSPolicy controller determines which ExternalDNS controllers should be active
// based on the DNSPolicy mode (Active or RegionBound) and the current cluster's region.
// This function creates one DNSEndpoint per active controller.
//
// DNSEndpoints are always created in the ServiceRoute's namespace to enable:
//   - Automatic cleanup via OwnerReferences when ServiceRoute is deleted
//   - Namespace-based RBAC and isolation
//   - Co-location of related resources for easier debugging
func (r *ServiceRouteReconciler) generateDNSEndpoints(
	serviceRoute *routingv1alpha1.ServiceRoute,
	dnsPolicy *routingv1alpha1.DNSPolicy,
	gateway *routingv1alpha1.Gateway,
	clusterIdentity *clusteridentity.ClusterIdentity,
	dnsConfig *dnsconfiguration.DNSConfiguration,
) ([]*externaldnsv1alpha1.DNSEndpoint, error) {
	var dnsEndpoints []*externaldnsv1alpha1.DNSEndpoint

	activeControllers := dnsPolicy.Status.ActiveControllers
	if len(activeControllers) == 0 {
		return nil, fmt.Errorf("no active controllers in DNSPolicy")
	}

	targetNamespace := serviceRoute.Namespace

	// Pattern: {serviceName}-ns-{envLetter}-{environment}-{application}.{domain}
	sourceHost := fmt.Sprintf("%s-ns-%s-%s-%s.%s",
		serviceRoute.Spec.ServiceName,
		clusterIdentity.EnvironmentLetter,
		serviceRoute.Spec.Environment,
		serviceRoute.Spec.Application,
		clusterIdentity.Domain,
	)

	// Pattern: {cluster}-{region}-{gatewayPostfix}.{domain}
	targetHost := fmt.Sprintf("%s-%s-%s.%s",
		clusterIdentity.Cluster,
		clusterIdentity.Region,
		gateway.Spec.TargetPostfix,
		clusterIdentity.Domain,
	)

	controllerMap := make(map[string]dnsconfiguration.ExternalDNSController)
	for _, controller := range dnsConfig.ExternalDNSControllers {
		controllerMap[controller.Name] = controller
	}

	for _, controllerName := range activeControllers {
		controller, exists := controllerMap[controllerName]
		if !exists {
			// Controller listed in status but not in spec - skip silently
			// This can occur during DNSPolicy updates
			continue
		}

		dnsEndpoint := r.buildDNSEndpoint(serviceRoute, controller, targetNamespace, sourceHost, targetHost)
		dnsEndpoints = append(dnsEndpoints, dnsEndpoint)
	}

	return dnsEndpoints, nil
}

// buildDNSEndpoint constructs a DNSEndpoint resource for ExternalDNS to process.
//
// It sets up:
// 1. Owner ID (TXT Record) for cross-cluster takeover (e.g., "_external-dns-owner.{sourceHost}").
// 2. Controller Annotation ("external-dns.alpha.kubernetes.io/controller") matching the region.
// 3. CNAME and TXT records.
// 4. Labels for tracking and filtering.
func (r *ServiceRouteReconciler) buildDNSEndpoint(
	serviceRoute *routingv1alpha1.ServiceRoute,
	controller dnsconfiguration.ExternalDNSController,
	targetNamespace string,
	sourceHost string,
	targetHost string,
) *externaldnsv1alpha1.DNSEndpoint {

	endpoints := []*externaldnsendpoint.Endpoint{
		{
			DNSName:    sourceHost,
			RecordType: "CNAME",
			Targets:    externaldnsendpoint.Targets{targetHost},
		},
	}

	labels := map[string]string{
		"app.kubernetes.io/managed-by": "service-router-operator",
		"router.io/controller":         controller.Name,
		"router.io/region":             controller.Region,
		"router.io/serviceroute":       serviceRoute.Name,
		"router.io/source-namespace":   serviceRoute.Namespace,
	}

	dnsEndpoint := &externaldnsv1alpha1.DNSEndpoint{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "externaldns.k8s.io/v1alpha1",
			Kind:       "DNSEndpoint",
		},
		ObjectMeta: metav1.ObjectMeta{
			// Name uses controller.Name to ensure uniqueness when multiple controllers
			// in the same region create DNSEndpoints for the same ServiceRoute
			Name:      fmt.Sprintf("%s-%s", serviceRoute.Name, controller.Name),
			Namespace: targetNamespace,
			Labels:    labels,
			Annotations: map[string]string{
				// Controller annotation MUST match the ExternalDNS deployment's filter.
				// Using region (not controller.Name) enables cross-cluster takeover.
				"external-dns.alpha.kubernetes.io/controller": fmt.Sprintf("external-dns-%s", controller.Region),
			},
		},
		Spec: externaldnsv1alpha1.DNSEndpointSpec{
			Endpoints: endpoints,
		},
	}

	// Set owner reference for automatic cleanup when ServiceRoute is deleted.
	// Only works if DNSEndpoint is in the same namespace as the ServiceRoute.
	if targetNamespace == serviceRoute.Namespace {
		dnsEndpoint.OwnerReferences = []metav1.OwnerReference{
			*metav1.NewControllerRef(serviceRoute, routingv1alpha1.GroupVersion.WithKind("ServiceRoute")),
		}
	}

	return dnsEndpoint
}

// reconcileDNSEndpoints manages DNSEndpoint resources
func (r *ServiceRouteReconciler) reconcileDNSEndpoints(
	ctx context.Context,
	serviceRoute *routingv1alpha1.ServiceRoute,
	desired []*externaldnsv1alpha1.DNSEndpoint,
) error {
	// Collect all namespaces where DNSEndpoints might exist
	namespaces := make(map[string]bool)
	for _, endpoint := range desired {
		namespaces[endpoint.Namespace] = true
	}

	// List existing DNSEndpoints managed by this ServiceRoute across all relevant namespaces
	var existing []*externaldnsv1alpha1.DNSEndpoint
	for namespace := range namespaces {
		var existingList externaldnsv1alpha1.DNSEndpointList

		if err := r.List(ctx, &existingList,
			client.InNamespace(namespace),
			client.MatchingLabels{
				"app.kubernetes.io/managed-by": "service-router-operator",
				"router.io/serviceroute":       serviceRoute.Name,
				"router.io/source-namespace":   serviceRoute.Namespace,
			},
		); err != nil {
			return err
		}

		for i := range existingList.Items {
			existing = append(existing, &existingList.Items[i])
		}
	}

	// Build map of desired DNSEndpoints by namespace+name
	desiredMap := make(map[string]*externaldnsv1alpha1.DNSEndpoint)
	for _, endpoint := range desired {
		key := endpoint.Namespace + "/" + endpoint.Name
		desiredMap[key] = endpoint
	}

	// Build map of existing DNSEndpoints by namespace+name
	existingMap := make(map[string]*externaldnsv1alpha1.DNSEndpoint)
	for _, endpoint := range existing {
		key := endpoint.Namespace + "/" + endpoint.Name
		existingMap[key] = endpoint
	}

	// Create or update desired
	for key, desiredEndpoint := range desiredMap {
		if existingEndpoint, exists := existingMap[key]; exists {
			// Update if different
			if !reflect.DeepEqual(existingEndpoint.Spec, desiredEndpoint.Spec) {
				patch := client.MergeFrom(existingEndpoint.DeepCopy())
				existingEndpoint.Spec = desiredEndpoint.Spec
				existingEndpoint.Labels = desiredEndpoint.Labels
				existingEndpoint.Annotations = desiredEndpoint.Annotations
				if err := r.Patch(ctx, existingEndpoint, patch); err != nil {
					return err
				}
			}
		} else {
			// Create
			if err := r.Create(ctx, desiredEndpoint); err != nil {
				return err
			}
		}
	}

	// Delete stale
	for key, existingEndpoint := range existingMap {
		if _, desired := desiredMap[key]; !desired {
			if err := r.Delete(ctx, existingEndpoint); err != nil {
				return err
			}
		}
	}

	return nil
}

// updateStatusActive updates the ServiceRoute status to Active
func (r *ServiceRouteReconciler) updateStatusActive(
	ctx context.Context,
	serviceRoute *routingv1alpha1.ServiceRoute,
	dnsEndpoints []*externaldnsv1alpha1.DNSEndpoint,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Store first DNSEndpoint name
	if len(dnsEndpoints) > 0 {
		serviceRoute.Status.DNSEndpoint = dnsEndpoints[0].Name
	}

	serviceRoute.Status.Phase = consts.PhaseActive
	meta.SetStatusCondition(&serviceRoute.Status.Conditions, metav1.Condition{
		Type:               consts.ConditionTypeReady,
		Status:             metav1.ConditionTrue,
		ObservedGeneration: serviceRoute.Generation,
		Reason:             consts.ReasonReconciliationSucceeded,
		Message:            "ServiceRoute is active",
	})

	if err := r.Status().Update(ctx, serviceRoute); err != nil {
		if apierrors.IsConflict(err) {
			// Object has been modified, we can safely retry
			logger.Info("ServiceRoute status update conflict (Active), will retry")
			return ctrl.Result{Requeue: true}, nil
		}
		logger.Error(err, "failed to update ServiceRoute status to Active")
		return ctrl.Result{}, err
	}

	logger.Info("ServiceRoute reconciled successfully", "endpointsCreated", len(dnsEndpoints))
	return ctrl.Result{}, nil
}

// updateStatusPending updates the ServiceRoute status to Pending
func (r *ServiceRouteReconciler) updateStatusPending(
	ctx context.Context,
	serviceRoute *routingv1alpha1.ServiceRoute,
	reason, message string,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	serviceRoute.Status.Phase = consts.PhasePending
	meta.SetStatusCondition(&serviceRoute.Status.Conditions, metav1.Condition{
		Type:               consts.ConditionTypeReady,
		Status:             metav1.ConditionFalse,
		ObservedGeneration: serviceRoute.Generation,
		Reason:             reason,
		Message:            message,
	})

	if err := r.Status().Update(ctx, serviceRoute); err != nil {
		if apierrors.IsConflict(err) {
			logger.Info("ServiceRoute status update conflict (Pending), will retry")
			return ctrl.Result{Requeue: true}, nil
		}
		logger.Error(err, "failed to update ServiceRoute status to Pending")
		return ctrl.Result{}, err
	}

	logger.Info("ServiceRoute marked as Pending", "reason", reason, "message", message)
	return ctrl.Result{}, nil
}

// updateStatusFailed updates the ServiceRoute status to Failed
func (r *ServiceRouteReconciler) updateStatusFailed(
	ctx context.Context,
	serviceRoute *routingv1alpha1.ServiceRoute,
	reason, message string,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	serviceRoute.Status.Phase = consts.PhaseFailed
	meta.SetStatusCondition(&serviceRoute.Status.Conditions, metav1.Condition{
		Type:               consts.ConditionTypeReady,
		Status:             metav1.ConditionFalse,
		ObservedGeneration: serviceRoute.Generation,
		Reason:             reason,
		Message:            message,
	})

	if err := r.Status().Update(ctx, serviceRoute); err != nil {
		if apierrors.IsConflict(err) {
			logger.Info("ServiceRoute status update conflict (Failed), will retry")
			return ctrl.Result{Requeue: true}, nil
		}
		logger.Error(err, "failed to update ServiceRoute status to Failed")
		return ctrl.Result{}, err
	}

	logger.Info("ServiceRoute marked as Failed", "reason", reason, "message", message)
	// Return nil to stop the reconciliation loop, as the status is correctly reported as Failed.
	return ctrl.Result{}, nil
}

// mapDNSPolicyToServiceRoutes returns ServiceRoutes for a DNSPolicy
func (r *ServiceRouteReconciler) mapDNSPolicyToServiceRoutes(
	ctx context.Context,
	obj client.Object,
) []reconcile.Request {
	policy := obj.(*routingv1alpha1.DNSPolicy)

	// List all ServiceRoutes in the same namespace
	var serviceRoutes routingv1alpha1.ServiceRouteList
	if err := r.List(ctx, &serviceRoutes, client.InNamespace(policy.Namespace)); err != nil {
		return nil
	}

	var requests []reconcile.Request
	for _, route := range serviceRoutes.Items {
		requests = append(requests, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      route.Name,
				Namespace: route.Namespace,
			},
		})
	}

	return requests
}

// mapGatewayToServiceRoutes returns ServiceRoutes for a Gateway
func (r *ServiceRouteReconciler) mapGatewayToServiceRoutes(
	ctx context.Context,
	obj client.Object,
) []reconcile.Request {
	gateway := obj.(*routingv1alpha1.Gateway)

	// List all ServiceRoutes that reference this Gateway
	var serviceRoutes routingv1alpha1.ServiceRouteList
	if err := r.List(ctx, &serviceRoutes); err != nil {
		return nil
	}

	var requests []reconcile.Request
	for _, route := range serviceRoutes.Items {
		// Determine effective gateway namespace for comparison
		endpointGatewayNamespace := route.Spec.GatewayNamespace
		if endpointGatewayNamespace == "" {
			endpointGatewayNamespace = r.DefaultRouterGatewayNamespace
		}

		if route.Spec.GatewayName == gateway.Name && endpointGatewayNamespace == gateway.Namespace {
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      route.Name,
					Namespace: route.Namespace,
				},
			})
		}
	}

	return requests
}

// mapClusterIdentityToServiceRoutes returns all ServiceRoutes for ClusterIdentity changes
func (r *ServiceRouteReconciler) mapClusterIdentityToServiceRoutes(
	ctx context.Context,
	obj client.Object,
) []reconcile.Request {
	// When ClusterIdentity changes, reconcile all ServiceRoutes
	var serviceRoutes routingv1alpha1.ServiceRouteList
	if err := r.List(ctx, &serviceRoutes); err != nil {
		return nil
	}

	var requests []reconcile.Request
	for _, route := range serviceRoutes.Items {
		requests = append(requests, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      route.Name,
				Namespace: route.Namespace,
			},
		})
	}

	return requests
}

// SetupWithManager sets up the controller with the Manager.
func (r *ServiceRouteReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Register ExternalDNS types with the scheme
	if err := externaldnsv1alpha1.AddToScheme(mgr.GetScheme()); err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&routingv1alpha1.ServiceRoute{}).
		Owns(&externaldnsv1alpha1.DNSEndpoint{}).
		Watches(
			&routingv1alpha1.DNSPolicy{},
			handler.EnqueueRequestsFromMapFunc(r.mapDNSPolicyToServiceRoutes),
		).
		Watches(
			&routingv1alpha1.Gateway{},
			handler.EnqueueRequestsFromMapFunc(r.mapGatewayToServiceRoutes),
		).
		Watches(
			&clusterv1alpha1.ClusterIdentity{},
			handler.EnqueueRequestsFromMapFunc(r.mapClusterIdentityToServiceRoutes),
		).
		Complete(r)
}
