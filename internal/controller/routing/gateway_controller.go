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
	"regexp"
	"time"

	networkingv1beta1 "istio.io/api/networking/v1beta1"
	istioclientv1beta1 "istio.io/client-go/pkg/apis/networking/v1beta1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	externaldnsv1alpha1 "sigs.k8s.io/external-dns/apis/v1alpha1"

	clusterv1alpha1 "github.com/vecozo/service-router-operator/api/cluster/v1alpha1"
	routingv1alpha1 "github.com/vecozo/service-router-operator/api/routing/v1alpha1"
	"github.com/vecozo/service-router-operator/internal/clusteridentity"
	"github.com/vecozo/service-router-operator/pkg/consts"
)

// gatewayControllerConfig identifies a unique controller configuration
type gatewayControllerConfig struct {
	controller    string
	targetPostfix string
}

// GatewayReconciler reconciles a Gateway object
type GatewayReconciler struct {
	client.Client
	Scheme                        *runtime.Scheme
	DefaultRouterGatewayNamespace string
}

//+kubebuilder:rbac:groups=routing.router.io,resources=gateways,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=routing.router.io,resources=gateways/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=routing.router.io,resources=gateways/finalizers,verbs=update
//+kubebuilder:rbac:groups=networking.istio.io,resources=gateways,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=routing.router.io,resources=serviceroutes,verbs=get;list;watch
//+kubebuilder:rbac:groups=cluster.router.io,resources=clusteridentities,verbs=get;list;watch
//+kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch
//+kubebuilder:rbac:groups=externaldns.k8s.io,resources=dnsendpoints,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=cluster.router.io,resources=dnsconfigurations,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.16.0/pkg/reconcile
func (r *GatewayReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Retrieve the Gateway resource to determine the desired configuration.
	var gateway routingv1alpha1.Gateway
	if err := r.Get(ctx, req.NamespacedName, &gateway); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("Gateway deleted", "name", req.Name, "namespace", req.Namespace)
			return ctrl.Result{}, nil
		}
		logger.Error(err, "unable to fetch Gateway")
		return ctrl.Result{}, err
	}

	// Validate configuration to fail early on invalid input.
	if err := r.validateGateway(&gateway); err != nil {
		logger.Error(err, "validation failed")
		return r.updateStatusFailed(ctx, &gateway, consts.ReasonValidationFailed, err.Error())
	}

	// ClusterIdentity is needed to generate correct hostnames (region, domain).
	clusterIdentity, err := clusteridentity.Fetch(ctx, r.Client)
	if err != nil {
		logger.Error(err, "failed to get ClusterIdentity")
		return r.updateStatusPending(ctx, &gateway, consts.ReasonClusterIdentityNotAvailable,
			"Waiting for ClusterIdentity to be configured", "", false, "ClusterIdentity not available")
	}

	if clusterIdentity == nil {
		logger.Info("ClusterIdentity not available, requeueing")
		return r.updateStatusPending(ctx, &gateway, consts.ReasonClusterIdentityNotAvailable,
			"Waiting for ClusterIdentity to be configured", "", false, "ClusterIdentity not available")
	}

	// We need to aggregate all hosts from ServiceRoutes that reference this Gateway
	// to configure the Istio Gateway's servers block.
	hosts, err := r.collectHostsFromServiceRoutes(ctx, &gateway, clusterIdentity)
	if err != nil {
		logger.Error(err, "failed to collect hosts from ServiceRoutes")
		return ctrl.Result{}, err
	}

	// Check DNS status for LoadBalancer IP (independent of ServiceRoutes)
	lbIP, dnsReady, dnsMsg := r.checkDNSStatus(ctx, &gateway)

	// Check if no ServiceRoutes reference this Gateway
	if len(hosts) == 0 {
		logger.Info("No ServiceRoutes found for Gateway, deleting Istio Gateway if it exists")

		// Delete Istio Gateway if it exists
		if err := r.deleteIstioGateway(ctx, &gateway); err != nil {
			logger.Error(err, "failed to delete Istio Gateway")
			return ctrl.Result{}, err
		}

		return r.updateStatusPending(ctx, &gateway, consts.ReasonNoServiceRoutes,
			"Waiting for ServiceRoutes to reference this Gateway", lbIP, dnsReady, dnsMsg)
	}

	// Translate the generic Gateway CRD into an Istio-specific Gateway resource.
	istioGateway, err := r.generateIstioGateway(&gateway, hosts)
	if err != nil {
		logger.Error(err, "failed to generate Istio Gateway")
		return r.updateStatusFailed(ctx, &gateway, consts.ReasonIstioGatewayGenerationFailed, err.Error())
	}

	// Enforce the Istio Gateway configuration to match the desired state.
	if err := r.reconcileIstioGateway(ctx, istioGateway); err != nil {
		logger.Error(err, "failed to reconcile Istio Gateway")
		return ctrl.Result{}, err
	}

	// DNS status was already checked earlier, now update status to Active
	return r.updateStatusActive(ctx, &gateway, lbIP, dnsReady, dnsMsg)
}

// validateGateway validates the Gateway configuration
func (r *GatewayReconciler) validateGateway(gateway *routingv1alpha1.Gateway) error {
	// Validate controller is not empty
	if gateway.Spec.Controller == "" {
		return fmt.Errorf("controller must be specified")
	}

	// Validate credentialName is not empty
	if gateway.Spec.CredentialName == "" {
		return fmt.Errorf("credentialName must be specified")
	}

	// Validate targetPostfix is not empty
	if gateway.Spec.TargetPostfix == "" {
		return fmt.Errorf("targetPostfix must be specified")
	}

	// Validate targetPostfix format (should be lowercase alphanumeric with hyphens)
	if !regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`).MatchString(gateway.Spec.TargetPostfix) {
		return fmt.Errorf("targetPostfix must be lowercase alphanumeric with hyphens: %s", gateway.Spec.TargetPostfix)
	}

	return nil
}

// collectHostsFromServiceRoutes collects all unique hosts from ServiceRoutes using this Gateway
func (r *GatewayReconciler) collectHostsFromServiceRoutes(
	ctx context.Context,
	gateway *routingv1alpha1.Gateway,
	clusterIdentity *clusteridentity.ClusterIdentity,
) ([]string, error) {
	// List all ServiceRoutes
	var serviceRoutes routingv1alpha1.ServiceRouteList
	if err := r.List(ctx, &serviceRoutes); err != nil {
		return nil, err
	}

	// Collect unique hosts
	hostSet := make(map[string]bool)
	for _, route := range serviceRoutes.Items {
		// Skip if not using this gateway (check both name AND namespace)
		if route.Spec.GatewayName != gateway.Name {
			continue
		}

		// Check namespace match
		gatewayNamespace := route.Spec.GatewayNamespace
		if gatewayNamespace == "" {
			gatewayNamespace = r.DefaultRouterGatewayNamespace // default
		}
		if gatewayNamespace != gateway.Namespace {
			continue
		}

		// Construct the source hostname
		if clusterIdentity != nil {
			sourceHost := fmt.Sprintf("%s-ns-%s-%s-%s.%s",
				route.Spec.ServiceName,
				clusterIdentity.EnvironmentLetter,
				route.Spec.Environment,
				route.Spec.Application,
				clusterIdentity.Domain,
			)
			hostSet[sourceHost] = true
		}
	}

	// Convert to slice
	hosts := make([]string, 0, len(hostSet))
	for host := range hostSet {
		hosts = append(hosts, host)
	}

	return hosts, nil
}

// generateIstioGateway generates an Istio Gateway resource
func (r *GatewayReconciler) generateIstioGateway(
	gateway *routingv1alpha1.Gateway,
	hosts []string,
) (*istioclientv1beta1.Gateway, error) {
	return &istioclientv1beta1.Gateway{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "networking.istio.io/v1",
			Kind:       "Gateway",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      gateway.Name,
			Namespace: gateway.Namespace, // Istio Gateway in same namespace as Gateway resource
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "service-router-operator",
				"router.io/gateway":            gateway.Name,
			},
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(gateway, routingv1alpha1.GroupVersion.WithKind("Gateway")),
			},
		},
		Spec: networkingv1beta1.Gateway{
			Selector: map[string]string{
				"istio": gateway.Spec.Controller,
			},
			Servers: []*networkingv1beta1.Server{
				{
					Port: &networkingv1beta1.Port{
						Number:   443,
						Name:     "https",
						Protocol: "HTTPS",
					},
					Hosts: hosts,
					Tls: &networkingv1beta1.ServerTLSSettings{
						Mode:           networkingv1beta1.ServerTLSSettings_SIMPLE,
						CredentialName: gateway.Spec.CredentialName,
					},
				},
			},
		},
	}, nil
}

// reconcileIstioGateway manages the Istio Gateway resource
func (r *GatewayReconciler) reconcileIstioGateway(
	ctx context.Context,
	desired *istioclientv1beta1.Gateway,
) error {
	var existing istioclientv1beta1.Gateway
	err := r.Get(ctx, client.ObjectKey{
		Name:      desired.Name,
		Namespace: desired.Namespace,
	}, &existing)

	if err != nil {
		if apierrors.IsNotFound(err) {
			// Create
			return r.Create(ctx, desired)
		}
		return err
	}

	// Update if needed
	if r.istioGatewayNeedsUpdate(&existing, desired) {
		patch := client.MergeFrom(existing.DeepCopy())
		desired.Spec.DeepCopyInto(&existing.Spec)
		existing.Labels = desired.Labels
		return r.Patch(ctx, &existing, patch)
	}

	return nil
}

// deleteIstioGateway deletes the Istio Gateway resource if it exists
func (r *GatewayReconciler) deleteIstioGateway(
	ctx context.Context,
	gateway *routingv1alpha1.Gateway,
) error {
	var existing istioclientv1beta1.Gateway
	err := r.Get(ctx, client.ObjectKey{
		Name:      gateway.Name,
		Namespace: gateway.Namespace,
	}, &existing)

	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}

	return r.Delete(ctx, &existing)
}

// istioGatewayNeedsUpdate checks if the Istio Gateway needs updating
func (r *GatewayReconciler) istioGatewayNeedsUpdate(existing, desired *istioclientv1beta1.Gateway) bool {
	// Compare selectors
	if len(existing.Spec.Selector) != len(desired.Spec.Selector) {
		return true
	}
	for k, v := range desired.Spec.Selector {
		if existing.Spec.Selector[k] != v {
			return true
		}
	}

	// Compare servers
	if len(existing.Spec.Servers) != len(desired.Spec.Servers) {
		return true
	}

	if len(desired.Spec.Servers) > 0 && len(existing.Spec.Servers) > 0 {
		existingServer := existing.Spec.Servers[0]
		desiredServer := desired.Spec.Servers[0]

		// Compare hosts (order-independent)
		if len(existingServer.Hosts) != len(desiredServer.Hosts) {
			return true
		}
		existingHostSet := make(map[string]bool)
		for _, h := range existingServer.Hosts {
			existingHostSet[h] = true
		}
		for _, h := range desiredServer.Hosts {
			if !existingHostSet[h] {
				return true
			}
		}

		// Compare TLS settings
		if existingServer.Tls != nil && desiredServer.Tls != nil {
			if existingServer.Tls.CredentialName != desiredServer.Tls.CredentialName ||
				existingServer.Tls.Mode != desiredServer.Tls.Mode {
				return true
			}
		}
	}

	// Compare labels
	for k, v := range desired.Labels {
		if existing.Labels[k] != v {
			return true
		}
	}

	return false
}

// updateStatusActive updates the Gateway status to Active
func (r *GatewayReconciler) updateStatusActive(
	ctx context.Context,
	gateway *routingv1alpha1.Gateway,
	lbIP string,
	dnsReady bool,
	dnsMsg string,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	gateway.Status.Phase = consts.PhaseActive
	gateway.Status.LoadBalancerIP = lbIP

	// Gateway Ready Condition
	meta.SetStatusCondition(&gateway.Status.Conditions, metav1.Condition{
		Type:               consts.ConditionTypeReady,
		Status:             metav1.ConditionTrue,
		ObservedGeneration: gateway.Generation,
		Reason:             consts.ReasonReconciliationSucceeded,
		Message:            "Gateway is active",
	})

	// DNS Ready Condition
	dnsStatus := metav1.ConditionFalse
	dnsReason := consts.ReasonDNSNotReady
	if dnsReady {
		dnsStatus = metav1.ConditionTrue
		dnsReason = consts.ReasonDNSEndpointsCreated
	} else if lbIP == "" {
		dnsReason = consts.ReasonLoadBalancerIPPending
	}

	meta.SetStatusCondition(&gateway.Status.Conditions, metav1.Condition{
		Type:               consts.ConditionTypeDNSReady,
		Status:             dnsStatus,
		ObservedGeneration: gateway.Generation,
		Reason:             dnsReason,
		Message:            dnsMsg,
	})

	if err := r.Status().Update(ctx, gateway); err != nil {
		if apierrors.IsConflict(err) {
			logger.Info("Gateway status update conflict (Active), will retry")
			return ctrl.Result{Requeue: true}, nil
		}
		logger.Error(err, "failed to update Gateway status to Active")
		return ctrl.Result{}, err
	}

	// If DNS is not ready (likely waiting for IP), requeue after delay
	if !dnsReady {
		logger.Info("Gateway waiting for LoadBalancer IP/DNS", "phase", gateway.Status.Phase, "dnsReady", dnsReady)
		return ctrl.Result{RequeueAfter: time.Second * 30}, nil
	}

	logger.Info("Gateway reconciled successfully", "phase", gateway.Status.Phase, "dnsReady", dnsReady)

	return ctrl.Result{}, nil
}

// updateStatusPending updates the Gateway status to Pending
func (r *GatewayReconciler) updateStatusPending(
	ctx context.Context,
	gateway *routingv1alpha1.Gateway,
	reason, message string,
	lbIP string,
	dnsReady bool,
	dnsMsg string,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	gateway.Status.Phase = consts.PhasePending
	gateway.Status.LoadBalancerIP = lbIP

	// Gateway Ready Condition (False because no ServiceRoutes)
	meta.SetStatusCondition(&gateway.Status.Conditions, metav1.Condition{
		Type:               consts.ConditionTypeReady,
		Status:             metav1.ConditionFalse,
		ObservedGeneration: gateway.Generation,
		Reason:             reason,
		Message:            message,
	})

	// DNS Ready Condition (can be True even when Gateway is Pending)
	dnsStatus := metav1.ConditionFalse
	dnsReason := consts.ReasonDNSNotReady
	if dnsReady {
		dnsStatus = metav1.ConditionTrue
		dnsReason = consts.ReasonDNSEndpointsCreated
	} else if lbIP == "" {
		dnsReason = consts.ReasonLoadBalancerIPPending
	}

	meta.SetStatusCondition(&gateway.Status.Conditions, metav1.Condition{
		Type:               consts.ConditionTypeDNSReady,
		Status:             dnsStatus,
		ObservedGeneration: gateway.Generation,
		Reason:             dnsReason,
		Message:            dnsMsg,
	})

	if err := r.Status().Update(ctx, gateway); err != nil {
		if apierrors.IsConflict(err) {
			logger.Info("Gateway status update conflict (Pending), will retry")
			return ctrl.Result{Requeue: true}, nil
		}
		logger.Error(err, "failed to update Gateway status to Pending")
		return ctrl.Result{}, err
	}

	// If DNS is not ready (likely waiting for IP), requeue after delay
	if !dnsReady {
		logger.Info("Gateway Pending, waiting for LoadBalancer IP/DNS", "phase", gateway.Status.Phase, "dnsReady", dnsReady)
		return ctrl.Result{RequeueAfter: time.Second * 30}, nil
	}

	logger.Info("Gateway marked as Pending", "reason", reason, "message", message, "dnsReady", dnsReady)
	return ctrl.Result{}, nil
}

// updateStatusFailed updates the Gateway status to Failed
func (r *GatewayReconciler) updateStatusFailed(ctx context.Context, gateway *routingv1alpha1.Gateway, reason, message string) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	gateway.Status.Phase = consts.PhaseFailed
	meta.SetStatusCondition(&gateway.Status.Conditions, metav1.Condition{
		Type:               consts.ConditionTypeReady,
		Status:             metav1.ConditionFalse,
		ObservedGeneration: gateway.Generation,
		Reason:             reason,
		Message:            message,
	})

	if err := r.Status().Update(ctx, gateway); err != nil {
		if apierrors.IsConflict(err) {
			logger.Info("Gateway status update conflict (Failed), will retry")
			return ctrl.Result{Requeue: true}, nil
		}
		logger.Error(err, "failed to update Gateway status to Failed")
		return ctrl.Result{}, err
	}

	logger.Info("Gateway marked as Failed", "reason", reason, "message", message)
	return ctrl.Result{}, nil
}

// mapServiceRouteToGateway returns reconcile requests for Key(Gateway) derived from Key(ServiceRoute)
func (r *GatewayReconciler) mapServiceRouteToGateway(
	ctx context.Context,
	obj client.Object,
) []reconcile.Request {
	serviceRoute := obj.(*routingv1alpha1.ServiceRoute)

	// Use the actual namespace specified in the ServiceRoute
	gatewayNamespace := serviceRoute.Spec.GatewayNamespace
	if gatewayNamespace == "" {
		gatewayNamespace = r.DefaultRouterGatewayNamespace // default
	}

	return []reconcile.Request{
		{
			NamespacedName: types.NamespacedName{
				Name:      serviceRoute.Spec.GatewayName,
				Namespace: gatewayNamespace,
			},
		},
	}
}

// getLoadBalancerService finds the LoadBalancer Service for a given controller
func (r *GatewayReconciler) getLoadBalancerService(ctx context.Context, controller string) (*corev1.Service, error) {
	var services corev1.ServiceList
	// We list all services in all namespaces because the Istio gateway service
	// can be in any namespace (typically istio-system, but not guaranteed)
	if err := r.List(ctx, &services, client.MatchingLabels{"istio": controller}); err != nil {
		return nil, err
	}

	for _, svc := range services.Items {
		if svc.Spec.Type == corev1.ServiceTypeLoadBalancer {
			return &svc, nil
		}
	}

	return nil, nil
}

// checkDNSStatus returns the LoadBalancer IP and status of DNS provisioning for a Gateway
func (r *GatewayReconciler) checkDNSStatus(
	ctx context.Context,
	gateway *routingv1alpha1.Gateway,
) (string, bool, string) {
	svc, err := r.getLoadBalancerService(ctx, gateway.Spec.Controller)
	if err != nil || svc == nil {
		return "", false, "LoadBalancer Service not found"
	}

	if len(svc.Status.LoadBalancer.Ingress) == 0 {
		return "", false, "LoadBalancer IP pending"
	}

	ip := svc.Status.LoadBalancer.Ingress[0].IP
	if ip == "" {
		return "", false, "LoadBalancer IP empty"
	}

	return ip, true, "DNSEndpoints provisioned"
}

// mapServiceToGateways returns reconcile requests for Gateways using the updated Service
func (r *GatewayReconciler) mapServiceToGateways(ctx context.Context, obj client.Object) []reconcile.Request {
	svc := obj.(*corev1.Service)
	if svc.Spec.Type != corev1.ServiceTypeLoadBalancer {
		return nil
	}

	// Check if it has 'istio' label
	controller, ok := svc.Labels["istio"]
	if !ok {
		return nil
	}

	// Find all Gateways using this controller
	var gateways routingv1alpha1.GatewayList
	if err := r.List(ctx, &gateways); err != nil {
		return nil
	}

	var requests []reconcile.Request
	for _, gw := range gateways.Items {
		if gw.Spec.Controller == controller {
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      gw.Name,
					Namespace: gw.Namespace,
				},
			})
		}
	}
	return requests
}

// mapDNSConfigToGateways returns reconcile requests for all Gateways when DNSConfiguration changes
func (r *GatewayReconciler) mapDNSConfigToGateways(ctx context.Context, obj client.Object) []reconcile.Request {
	var gateways routingv1alpha1.GatewayList
	if err := r.List(ctx, &gateways); err != nil {
		return nil
	}

	var requests []reconcile.Request
	for _, gw := range gateways.Items {
		requests = append(requests, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      gw.Name,
				Namespace: gw.Namespace,
			},
		})
	}
	return requests
}

// SetupWithManager sets up the controller with the Manager.
func (r *GatewayReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Register Istio Gateway type with the scheme
	if err := istioclientv1beta1.AddToScheme(mgr.GetScheme()); err != nil {
		return err
	}
	if err := corev1.AddToScheme(mgr.GetScheme()); err != nil {
		return err
	}
	if err := externaldnsv1alpha1.AddToScheme(mgr.GetScheme()); err != nil {
		return err
	}
	if err := routingv1alpha1.AddToScheme(mgr.GetScheme()); err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&routingv1alpha1.Gateway{}).
		WithEventFilter(predicate.GenerationChangedPredicate{}).
		Owns(&istioclientv1beta1.Gateway{}).
		Watches(
			&routingv1alpha1.ServiceRoute{},
			handler.EnqueueRequestsFromMapFunc(r.mapServiceRouteToGateway),
		).
		Watches(
			&corev1.Service{},
			handler.EnqueueRequestsFromMapFunc(r.mapServiceToGateways),
		).
		Watches(
			&clusterv1alpha1.DNSConfiguration{},
			handler.EnqueueRequestsFromMapFunc(r.mapDNSConfigToGateways),
		).
		Complete(r)
}
