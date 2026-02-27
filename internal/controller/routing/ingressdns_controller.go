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
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
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
)

// IngressDNSReconciler reconciles global DNS infrastructure for Gateways
type IngressDNSReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=routing.router.io,resources=gateways,verbs=get;list;watch
//+kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch
//+kubebuilder:rbac:groups=externaldns.k8s.io,resources=dnsendpoints,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=cluster.router.io,resources=clusteridentities,verbs=get;list;watch
//+kubebuilder:rbac:groups=cluster.router.io,resources=dnsconfigurations,verbs=get;list;watch

// Reconcile manages the lifecycle of infrastructure DNS records (A/TXT) for Ingress Gateways.
// It aggregates all Gateway configurations to ensure:
// 1. DNSEndpoints exist for every active Controller+Postfix combination.
// 2. Orphaned DNSEndpoints (no active Gateway remaining) are garbage collected.
func (r *IngressDNSReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Since we map everything to "global", we don't use req.NamespacedName.
	// This acts as a singleton reconciler.

	var gateways routingv1alpha1.GatewayList
	if err := r.List(ctx, &gateways); err != nil {
		return ctrl.Result{}, err
	}

	activeConfigs := make(map[gatewayControllerConfig]bool)

	for _, gw := range gateways.Items {
		// Skip gateways that are being deleted
		if gw.DeletionTimestamp != nil {
			continue
		}

		config := gatewayControllerConfig{
			controller:    gw.Spec.Controller,
			targetPostfix: gw.Spec.TargetPostfix,
		}
		activeConfigs[config] = true
	}

	// Cleanup orphaned DNSEndpoints
	if err := r.cleanupOrphanedDNSEndpoints(ctx, activeConfigs); err != nil {
		logger.Error(err, "failed to cleanup orphaned DNS endpoints")
	}

	clusterIdentity, err := clusteridentity.Fetch(ctx, r.Client)
	if err != nil {
		return ctrl.Result{}, err
	}
	if clusterIdentity == nil {
		logger.Info("ClusterIdentity not available, skipping DNS creation/update")
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	dnsConfig, err := dnsconfiguration.Fetch(ctx, r.Client)
	if err != nil {
		return ctrl.Result{}, err
	}
	if dnsConfig == nil {
		logger.Info("DNSConfiguration not available, skipping DNS creation/update")
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	for config := range activeConfigs {
		if err := r.reconcileDNSEndpointsForConfig(ctx, config.controller, config.targetPostfix, clusterIdentity, dnsConfig); err != nil {
			logger.Error(err, "failed to reconcile DNS endpoints", "controller", config.controller, "postfix", config.targetPostfix)
			// Continue with other controllers, but return error at end?
			// For now we log and continue to try to reconcile as much as possible.
		}
	}

	return ctrl.Result{}, nil
}

// reconcileDNSEndpointsForConfig creates/updates DNSEndpoints for a specific controller configuration
func (r *IngressDNSReconciler) reconcileDNSEndpointsForConfig(
	ctx context.Context,
	controller string,
	targetPostfix string,
	clusterIdentity *clusteridentity.ClusterIdentity,
	dnsConfig *dnsconfiguration.DNSConfiguration,
) error {
	svc, err := r.getLoadBalancerService(ctx, controller)
	if err != nil {
		return err
	}
	if svc == nil {
		// Service not found, cannot create DNS records yet
		return nil
	}

	if len(svc.Status.LoadBalancer.Ingress) == 0 {
		// IP not assigned yet
		return nil
	}
	ip := svc.Status.LoadBalancer.Ingress[0].IP
	if ip == "" {
		return nil
	}

	targetHost := fmt.Sprintf("%s-%s-%s.%s",
		clusterIdentity.Cluster,
		clusterIdentity.Region,
		targetPostfix,
		clusterIdentity.Domain,
	)

	for _, extDNS := range dnsConfig.ExternalDNSControllers {
		name := fmt.Sprintf("gateway-controller-%s-%s-%s", controller, targetPostfix, extDNS.Name)

		desired := &externaldnsv1alpha1.DNSEndpoint{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: svc.Namespace,
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: "v1",
						Kind:       "Service",
						Name:       svc.Name,
						UID:        svc.UID,
						Controller: tryBool(true),
					},
				},
				Labels: map[string]string{
					"router.io/istio-controller":   controller,
					"router.io/target-postfix":     targetPostfix,
					"router.io/region":             extDNS.Region,
					"router.io/resource-type":      "gateway-service",
					"app.kubernetes.io/managed-by": "service-router-operator",
				},
			},
			Spec: externaldnsv1alpha1.DNSEndpointSpec{
				Endpoints: []*externaldnsendpoint.Endpoint{
					{
						DNSName:    targetHost,
						RecordType: "A",
						Targets:    externaldnsendpoint.Targets{ip},
						RecordTTL:  externaldnsendpoint.TTL(300),
					},
				},
			},
		}

		// Create or Update
		var existing externaldnsv1alpha1.DNSEndpoint
		if err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: svc.Namespace}, &existing); err != nil {
			if apierrors.IsNotFound(err) {
				if err := r.Create(ctx, desired); err != nil {
					return err
				}
			} else {
				return err
			}
		} else {
			// Update if needed
			// Check for equality safely to avoid panics on malformed resources
			needsUpdate := len(existing.Spec.Endpoints) == 0 ||
				len(existing.Spec.Endpoints[0].Targets) == 0 ||
				existing.Spec.Endpoints[0].Targets[0] != ip ||
				existing.Spec.Endpoints[0].DNSName != targetHost

			if needsUpdate {
				patch := client.MergeFrom(existing.DeepCopy())
				existing.Spec = desired.Spec
				existing.Labels = desired.Labels
				if err := r.Patch(ctx, &existing, patch); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

// cleanupOrphanedDNSEndpoints removes DNSEndpoints for controllers that are no longer active
func (r *IngressDNSReconciler) cleanupOrphanedDNSEndpoints(
	ctx context.Context,
	activeConfigs map[gatewayControllerConfig]bool,
) error {
	// Find all namespaces that have Istio services
	var services corev1.ServiceList
	if err := r.List(ctx, &services, client.HasLabels{"istio"}); err != nil {
		return err
	}

	namespaces := make(map[string]bool)
	for _, svc := range services.Items {
		namespaces[svc.Namespace] = true
	}

	for ns := range namespaces {
		var endpoints externaldnsv1alpha1.DNSEndpointList
		if err := r.List(ctx, &endpoints, client.InNamespace(ns), client.MatchingLabels{"router.io/resource-type": "gateway-service"}); err != nil {
			// Continue to next namespace if list fails
			continue
		}

		for _, ep := range endpoints.Items {
			controller := ep.Labels["router.io/istio-controller"]
			postfix := ep.Labels["router.io/target-postfix"]

			config := gatewayControllerConfig{
				controller:    controller,
				targetPostfix: postfix,
			}

			if !activeConfigs[config] {
				// This endpoint is no longer used by any gateway
				if err := r.Delete(ctx, &ep); err != nil {
					return client.IgnoreNotFound(err)
				}
			}
		}
	}

	return nil
}

// getLoadBalancerService finds the LoadBalancer Service for a given controller
func (r *IngressDNSReconciler) getLoadBalancerService(ctx context.Context, controller string) (*corev1.Service, error) {
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

// mapGlobalEventsToRequest maps any event to a single global request
func (r *IngressDNSReconciler) mapGlobalEventsToRequest(
	ctx context.Context,
	obj client.Object,
) []reconcile.Request {
	return []reconcile.Request{
		{NamespacedName: types.NamespacedName{Name: "global"}},
	}
}

// mapServiceToRequest maps Service events to a global request if it's an Istio service
func (r *IngressDNSReconciler) mapServiceToRequest(
	ctx context.Context,
	obj client.Object,
) []reconcile.Request {
	// Only interested in services that might be Istio controllers
	if _, ok := obj.GetLabels()["istio"]; ok {
		return r.mapGlobalEventsToRequest(ctx, obj)
	}
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *IngressDNSReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		Named("ingress-dns-controller").
		// Watch Gateways: any change might require DNS update/cleanup
		Watches(&routingv1alpha1.Gateway{}, handler.EnqueueRequestsFromMapFunc(r.mapGlobalEventsToRequest)).
		// Watch Services: if LoadBalancer IP changes, we must update DNS
		Watches(&corev1.Service{}, handler.EnqueueRequestsFromMapFunc(r.mapServiceToRequest)).
		// Watch ClusterIdentity: domain/region changes affect all DNS
		Watches(&clusterv1alpha1.ClusterIdentity{}, handler.EnqueueRequestsFromMapFunc(r.mapGlobalEventsToRequest)).
		// Watch DNSConfiguration: provider changes affect all DNS
		Watches(&clusterv1alpha1.DNSConfiguration{}, handler.EnqueueRequestsFromMapFunc(r.mapGlobalEventsToRequest)).
		Complete(r)
}

func tryBool(b bool) *bool {
	return &b
}
