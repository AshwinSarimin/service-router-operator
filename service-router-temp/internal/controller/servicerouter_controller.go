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

package controller

import (
	"context"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	networkingv1 "github.com/AshwinSarimin/service-router-operator/api/v1"
	externaldnsv1alpha1 "sigs.k8s.io/external-dns/endpoint/v1alpha1"
	istiov1 "istio.io/client-go/pkg/apis/networking/v1"
)

const (
	// TypeAvailableServiceRouter represents the status of the ServiceRouter reconciliation
	TypeAvailableServiceRouter = "Available"
	
	// TypeDegradedServiceRouter represents a degraded status
	TypeDegradedServiceRouter = "Degraded"
)

// ServiceRouterReconciler reconciles a ServiceRouter object
type ServiceRouterReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=networking.acme.com,resources=servicerouters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.acme.com,resources=servicerouters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=networking.acme.com,resources=servicerouters/finalizers,verbs=update
// +kubebuilder:rbac:groups=externaldns.k8s.io,resources=dnsendpoints,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.istio.io,resources=gateways,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *ServiceRouterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// Fetch the ServiceRouter instance
	serviceRouter := &networkingv1.ServiceRouter{}
	err := r.Get(ctx, req.NamespacedName, serviceRouter)
	if err != nil {
		if errors.IsNotFound(err) {
			// Object not found, could have been deleted after reconcile request
			log.Info("ServiceRouter resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request
		log.Error(err, "Failed to get ServiceRouter")
		return ctrl.Result{}, err
	}

	// Validate the ServiceRouter spec
	if err := r.validateServiceRouter(serviceRouter); err != nil {
		log.Error(err, "ServiceRouter validation failed")
		r.updateStatus(ctx, serviceRouter, TypeDegradedServiceRouter, metav1.ConditionFalse, "ValidationFailed", err.Error())
		return ctrl.Result{}, err
	}

	// Reconcile DNSEndpoint resources
	dnsEndpointsCreated, err := r.reconcileDNSEndpoints(ctx, serviceRouter)
	if err != nil {
		log.Error(err, "Failed to reconcile DNSEndpoints")
		r.updateStatus(ctx, serviceRouter, TypeDegradedServiceRouter, metav1.ConditionFalse, "DNSEndpointsFailed", err.Error())
		return ctrl.Result{}, err
	}

	// Reconcile Gateway resources
	gatewaysCreated, err := r.reconcileGateways(ctx, serviceRouter)
	if err != nil {
		log.Error(err, "Failed to reconcile Gateways")
		r.updateStatus(ctx, serviceRouter, TypeDegradedServiceRouter, metav1.ConditionFalse, "GatewaysFailed", err.Error())
		return ctrl.Result{}, err
	}

	// Update status
	serviceRouter.Status.DNSEndpointsCreated = int32(dnsEndpointsCreated)
	serviceRouter.Status.GatewaysCreated = int32(gatewaysCreated)
	serviceRouter.Status.ObservedGeneration = serviceRouter.Generation
	now := metav1.Now()
	serviceRouter.Status.LastReconciled = &now

	if err := r.updateStatus(ctx, serviceRouter, TypeAvailableServiceRouter, metav1.ConditionTrue, "Reconciled", "ServiceRouter reconciled successfully"); err != nil {
		log.Error(err, "Failed to update ServiceRouter status")
		return ctrl.Result{}, err
	}

	log.Info("Successfully reconciled ServiceRouter", 
		"dnsEndpoints", dnsEndpointsCreated, 
		"gateways", gatewaysCreated)

	// Requeue after 10 minutes for continuous reconciliation
	return ctrl.Result{RequeueAfter: 10 * time.Minute}, nil
}

// validateServiceRouter validates the ServiceRouter configuration
func (r *ServiceRouterReconciler) validateServiceRouter(sr *networkingv1.ServiceRouter) error {
	// Check that all gateways referenced in apps exist
	gatewayMap := make(map[string]bool)
	for _, gw := range sr.Spec.Gateways {
		gatewayMap[gw.Name] = true
	}

	for _, app := range sr.Spec.Apps {
		// Validate region-bound apps have region set
		if app.Mode == "regionbound" && app.Region == "" {
			return fmt.Errorf("app '%s' is regionbound but has no region set", app.Name)
		}

		// Validate all gateway references exist
		for gatewayName := range app.Services {
			if !gatewayMap[gatewayName] {
				return fmt.Errorf("gateway '%s' referenced in app '%s' not found in gateways list", gatewayName, app.Name)
			}
		}
	}

	return nil
}

// reconcileDNSEndpoints creates or updates DNSEndpoint resources
func (r *ServiceRouterReconciler) reconcileDNSEndpoints(ctx context.Context, sr *networkingv1.ServiceRouter) (int, error) {
	log := log.FromContext(ctx)
	count := 0

	for _, controller := range sr.Spec.ExternalDNS {
		endpoints := r.calculateEndpointsForController(sr, controller)
		
		if len(endpoints) == 0 {
			log.Info("No endpoints for controller, skipping", "controller", controller.Controller)
			continue
		}

		dnsEndpoint := &externaldnsv1alpha1.DNSEndpoint{
			ObjectMeta: metav1.ObjectMeta{
				Name:      controller.Controller,
				Namespace: sr.Namespace,
				Labels: map[string]string{
					"app": controller.Controller,
				},
			},
			Spec: externaldnsv1alpha1.DNSEndpointSpec{
				Endpoints: endpoints,
			},
		}

		// Set ServiceRouter as owner of the DNSEndpoint
		if err := controllerutil.SetControllerReference(sr, dnsEndpoint, r.Scheme); err != nil {
			return count, err
		}

		// Create or update DNSEndpoint
		found := &externaldnsv1alpha1.DNSEndpoint{}
		err := r.Get(ctx, types.NamespacedName{Name: dnsEndpoint.Name, Namespace: dnsEndpoint.Namespace}, found)
		if err != nil && errors.IsNotFound(err) {
			log.Info("Creating DNSEndpoint", "name", dnsEndpoint.Name)
			if err := r.Create(ctx, dnsEndpoint); err != nil {
				return count, err
			}
			count++
		} else if err != nil {
			return count, err
		} else {
			// Update existing DNSEndpoint
			found.Spec = dnsEndpoint.Spec
			found.Labels = dnsEndpoint.Labels
			log.Info("Updating DNSEndpoint", "name", found.Name)
			if err := r.Update(ctx, found); err != nil {
				return count, err
			}
			count++
		}
	}

	return count, nil
}

// calculateEndpointsForController calculates DNS endpoints for a specific controller
func (r *ServiceRouterReconciler) calculateEndpointsForController(sr *networkingv1.ServiceRouter, controller networkingv1.ExternalDNSController) []*externaldnsv1alpha1.Endpoint {
	var endpoints []*externaldnsv1alpha1.Endpoint

	// Create a gateway lookup map
	gatewayMap := make(map[string]networkingv1.Gateway)
	for _, gw := range sr.Spec.Gateways {
		gatewayMap[gw.Name] = gw
	}

	for _, app := range sr.Spec.Apps {
		appMode := app.Mode
		if appMode == "" {
			appMode = "active"
		}

		appRegion := app.Region
		if appRegion == "" {
			appRegion = sr.Spec.Region
		}

		appCluster := app.Cluster
		if appCluster == "" {
			appCluster = sr.Spec.Cluster
		}

		// Determine if we should create records for this controller
		shouldCreate := false
		if appMode == "regionbound" {
			shouldCreate = (appRegion == controller.Region)
		} else if appMode == "active" {
			shouldCreate = (sr.Spec.Region == controller.Region)
		}

		if !shouldCreate {
			continue
		}

		// Process each gateway's services
		for gatewayName, services := range app.Services {
			gateway, exists := gatewayMap[gatewayName]
			if !exists {
				continue
			}

			targetPostfix := gateway.TargetPostfix
			if targetPostfix == "" {
				targetPostfix = "external"
			}

			for _, serviceName := range services {
				// Build DNS name: {service}-ns-{env-letter}-{env-name}-{app-name}.{domain}
				dnsName := fmt.Sprintf("%s-ns-%s-%s-%s.%s",
					serviceName,
					sr.Spec.EnvironmentLetter,
					app.Environment,
					app.Name,
					sr.Spec.Domain)

				// Build target: {cluster}-{region}-{targetPostfix}.{domain}
				var target string
				if appMode == "regionbound" {
					target = fmt.Sprintf("%s-%s-%s.%s",
						appCluster,
						appRegion,
						targetPostfix,
						sr.Spec.Domain)
				} else {
					target = fmt.Sprintf("%s-%s-%s.%s",
						sr.Spec.Cluster,
						sr.Spec.Region,
						targetPostfix,
						sr.Spec.Domain)
				}

				endpoint := &externaldnsv1alpha1.Endpoint{
					DNSName:    dnsName,
					RecordType: "CNAME",
					Targets:    []string{target},
				}

				endpoints = append(endpoints, endpoint)
			}
		}
	}

	return endpoints
}

// reconcileGateways creates or updates Istio Gateway resources
func (r *ServiceRouterReconciler) reconcileGateways(ctx context.Context, sr *networkingv1.ServiceRouter) (int, error) {
	log := log.FromContext(ctx)
	count := 0

	for _, gatewaySpec := range sr.Spec.Gateways {
		// Build list of hosts for this gateway
		var hosts []string
		for _, app := range sr.Spec.Apps {
			// Check if this app uses this gateway
			services, exists := app.Services[gatewaySpec.Name]
			if !exists {
				continue
			}

			// Add each service to the hosts list
			for _, serviceName := range services {
				host := fmt.Sprintf("%s-ns-%s-%s-%s.%s",
					serviceName,
					sr.Spec.EnvironmentLetter,
					app.Environment,
					app.Name,
					sr.Spec.Domain)
				hosts = append(hosts, host)
			}
		}

		if len(hosts) == 0 {
			log.Info("No hosts for gateway, skipping", "gateway", gatewaySpec.Name)
			continue
		}

		httpsPort := gatewaySpec.HTTPSPortNumber
		if httpsPort == 0 {
			httpsPort = 443
		}

		gateway := &istiov1.Gateway{
			ObjectMeta: metav1.ObjectMeta{
				Name:      gatewaySpec.Name,
				Namespace: sr.Namespace,
			},
			Spec: istiov1.GatewaySpec{
				Selector: map[string]string{
					"istio": gatewaySpec.Controller,
				},
				Servers: []*istiov1.Server{
					{
						Port: &istiov1.Port{
							Number:   uint32(httpsPort),
							Name:     "https",
							Protocol: "HTTPS",
						},
						Tls: &istiov1.ServerTLSSettings{
							Mode:           istiov1.ServerTLSSettings_SIMPLE,
							CredentialName: gatewaySpec.CredentialName,
						},
						Hosts: hosts,
					},
				},
			},
		}

		// Set ServiceRouter as owner of the Gateway
		if err := controllerutil.SetControllerReference(sr, gateway, r.Scheme); err != nil {
			return count, err
		}

		// Create or update Gateway
		found := &istiov1.Gateway{}
		err := r.Get(ctx, types.NamespacedName{Name: gateway.Name, Namespace: gateway.Namespace}, found)
		if err != nil && errors.IsNotFound(err) {
			log.Info("Creating Gateway", "name", gateway.Name)
			if err := r.Create(ctx, gateway); err != nil {
				return count, err
			}
			count++
		} else if err != nil {
			return count, err
		} else {
			// Update existing Gateway
			found.Spec = gateway.Spec
			log.Info("Updating Gateway", "name", found.Name)
			if err := r.Update(ctx, found); err != nil {
				return count, err
			}
			count++
		}
	}

	return count, nil
}

// updateStatus updates the ServiceRouter status
func (r *ServiceRouterReconciler) updateStatus(ctx context.Context, sr *networkingv1.ServiceRouter, conditionType string, status metav1.ConditionStatus, reason, message string) error {
	condition := metav1.Condition{
		Type:               conditionType,
		Status:             status,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: metav1.Now(),
		ObservedGeneration: sr.Generation,
	}

	// Update or append the condition
	conditionFound := false
	for i, existingCondition := range sr.Status.Conditions {
		if existingCondition.Type == conditionType {
			sr.Status.Conditions[i] = condition
			conditionFound = true
			break
		}
	}
	if !conditionFound {
		sr.Status.Conditions = append(sr.Status.Conditions, condition)
	}

	return r.Status().Update(ctx, sr)
}

// SetupWithManager sets up the controller with the Manager.
func (r *ServiceRouterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&networkingv1.ServiceRouter{}).
		Owns(&externaldnsv1alpha1.DNSEndpoint{}).
		Owns(&istiov1.Gateway{}).
		Complete(r)
}