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

	clusterv1alpha1 "github.com/vecozo/service-router-operator/api/cluster/v1alpha1"
	routingv1alpha1 "github.com/vecozo/service-router-operator/api/routing/v1alpha1"
	"github.com/vecozo/service-router-operator/internal/clusteridentity"
	"github.com/vecozo/service-router-operator/internal/dnsconfiguration"
	"github.com/vecozo/service-router-operator/pkg/consts"
)

// DNSPolicyReconciler reconciles a DNSPolicy object
type DNSPolicyReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=routing.router.io,resources=dnspolicies,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=routing.router.io,resources=dnspolicies/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=routing.router.io,resources=dnspolicies/finalizers,verbs=update
//+kubebuilder:rbac:groups=cluster.router.io,resources=clusteridentities,verbs=get;list;watch
//+kubebuilder:rbac:groups=cluster.router.io,resources=dnsconfigurations,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.16.0/pkg/reconcile
func (r *DNSPolicyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Retrieve the desired state to determine if reconciliation is needed.
	var dnsPolicy routingv1alpha1.DNSPolicy
	if err := r.Get(ctx, req.NamespacedName, &dnsPolicy); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("DNSPolicy deleted", "name", req.Name, "namespace", req.Namespace)
			return ctrl.Result{}, nil
		}
		logger.Error(err, "unable to fetch DNSPolicy")
		return ctrl.Result{}, err
	}

	// ClusterIdentity is required to determine the current region and validate if the policy is active.
	// Cache-first with CRD fallback.
	clusterIdentity, err := clusteridentity.Fetch(ctx, r.Client)
	if err != nil {
		logger.Error(err, "failed to get ClusterIdentity")
		return r.updateStatusPending(ctx, &dnsPolicy, consts.ReasonClusterIdentityNotAvailable,
			"Waiting for ClusterIdentity to be configured")
	}
	if clusterIdentity == nil {
		logger.Info("ClusterIdentity not available, requeueing")
		return r.updateStatusPending(ctx, &dnsPolicy, consts.ReasonClusterIdentityNotAvailable,
			"Waiting for ClusterIdentity to be configured")
	}

	// DNSConfiguration provides the list of available ExternalDNS controllers and their regions.
	// Cache-first with CRD fallback.
	dnsConfig, err := dnsconfiguration.Fetch(ctx, r.Client)
	if err != nil {
		logger.Error(err, "failed to get DNSConfiguration")
		return r.updateStatusPending(ctx, &dnsPolicy, consts.ReasonDNSConfigurationNotAvailable,
			"Waiting for DNSConfiguration to be configured")
	}
	if dnsConfig == nil {
		logger.Info("DNSConfiguration not available, requeueing")
		return r.updateStatusPending(ctx, &dnsPolicy, consts.ReasonDNSConfigurationNotAvailable,
			"Waiting for DNSConfiguration to be configured")
	}

	// Validate the policy spec against the available configuration to prevent invalid states.
	if err := r.validateDNSPolicy(&dnsPolicy, dnsConfig); err != nil {
		logger.Error(err, "validation failed")
		return r.updateStatusFailed(ctx, &dnsPolicy, consts.ReasonValidationFailed, err.Error())
	}

	// Determine if this policy applies to the current cluster based on region and cluster name constraints.
	policyActive, inactiveReason := r.isPolicyActive(&dnsPolicy, clusterIdentity)
	if !policyActive {
		logger.Info("DNSPolicy is not active for this cluster", "name", dnsPolicy.Name, "namespace", dnsPolicy.Namespace, "reason", inactiveReason)
		return r.updateStatusInactive(ctx, &dnsPolicy, inactiveReason)
	}

	// Calculate which ExternalDNS controllers should process this policy based on the mode (Active/RegionBound).
	activeControllers := r.determineActiveControllers(&dnsPolicy, clusterIdentity, dnsConfig)

	// Synchronize the status with the determined active controllers to reflect the current state.
	return r.updateStatusActive(ctx, &dnsPolicy, activeControllers)
}

// validateDNSPolicy validates the DNSPolicy specification
func (r *DNSPolicyReconciler) validateDNSPolicy(dnsPolicy *routingv1alpha1.DNSPolicy, dnsConfig *dnsconfiguration.DNSConfiguration) error {
	// Validate mode
	if dnsPolicy.Spec.Mode != "Active" && dnsPolicy.Spec.Mode != "RegionBound" {
		return fmt.Errorf("invalid mode: %s, must be Active or RegionBound", dnsPolicy.Spec.Mode)
	}

	// Validate controllers are defined in DNSConfiguration
	if len(dnsConfig.ExternalDNSControllers) == 0 {
		return fmt.Errorf("at least one ExternalDNS controller must be defined in DNSConfiguration")
	}

	return nil
}

// isPolicyActive checks if the DNSPolicy should be active based on cluster identity.
// Returns (active bool, reason string).
func (r *DNSPolicyReconciler) isPolicyActive(
	dnsPolicy *routingv1alpha1.DNSPolicy,
	clusterIdentity *clusteridentity.ClusterIdentity,
) (bool, string) {
	if dnsPolicy.Spec.SourceRegion != "" && dnsPolicy.Spec.SourceRegion != clusterIdentity.Region {
		return false, fmt.Sprintf("SourceRegion '%s' does not match cluster region '%s'",
			dnsPolicy.Spec.SourceRegion, clusterIdentity.Region)
	}

	if dnsPolicy.Spec.SourceCluster != "" && dnsPolicy.Spec.SourceCluster != clusterIdentity.Cluster {
		return false, fmt.Sprintf("SourceCluster '%s' does not match cluster name '%s'",
			dnsPolicy.Spec.SourceCluster, clusterIdentity.Cluster)
	}

	return true, ""
}

// determineActiveControllers determines which ExternalDNS controllers should be active
// based on the DNSPolicy mode and the cluster's region.
//
// Active Mode:
//   - Each cluster independently manages DNS records for its own region
//   - Selects ONLY controllers matching the cluster's region
//   - Traffic: Regional clients query regional DNS, get regional gateway
//
// RegionBound Mode:
//   - Enable one cluster to manage DNS records across multiple regions
//   - Selects ALL controllers (if policy is active for this cluster)
//   - Traffic: DNS records in ALL regions point to THIS cluster's gateway
func (r *DNSPolicyReconciler) determineActiveControllers(
	dnsPolicy *routingv1alpha1.DNSPolicy,
	clusterIdentity *clusteridentity.ClusterIdentity,
	dnsConfig *dnsconfiguration.DNSConfiguration,
) []string {
	if dnsPolicy.Spec.Mode == "Active" {
		return r.determineActiveControllersForActiveMode(clusterIdentity, dnsConfig)
	}
	return r.determineActiveControllersForRegionBoundMode(dnsConfig)
}

// determineActiveControllersForActiveMode selects controllers matching the cluster's region.
//
// In Active mode, each cluster provisions DNS records ONLY for its own region.
func (r *DNSPolicyReconciler) determineActiveControllersForActiveMode(
	clusterIdentity *clusteridentity.ClusterIdentity,
	dnsConfig *dnsconfiguration.DNSConfiguration,
) []string {
	var activeControllers []string

	// In Active mode, only controllers in the same region as the cluster
	for _, controller := range dnsConfig.ExternalDNSControllers {
		if controller.Region == clusterIdentity.Region {
			activeControllers = append(activeControllers, controller.Name)
		}
	}

	// Handle adopted regions
	if len(clusterIdentity.AdoptsRegions) > 0 && dnsConfig != nil {
		// Identify valid regions (those present in DNSConfiguration)
		existingRegions := make(map[string]bool)
		for _, controller := range dnsConfig.ExternalDNSControllers {
			existingRegions[controller.Region] = true
		}

		// Add controllers for valid adopted regions
		for _, region := range clusterIdentity.AdoptsRegions {
			if existingRegions[region] {
				for _, controller := range dnsConfig.ExternalDNSControllers {
					if controller.Region == region {
						activeControllers = append(activeControllers, controller.Name)
					}
				}
			}
		}
	}

	return activeControllers
}

// determineActiveControllersForRegionBoundMode selects all controllers.
//
// In RegionBound mode, a cluster provisions DNS records for ALL defined regions,
// with all DNS records pointing to THIS cluster's gateway.
func (r *DNSPolicyReconciler) determineActiveControllersForRegionBoundMode(
	dnsConfig *dnsconfiguration.DNSConfiguration,
) []string {
	var activeControllers []string

	// In RegionBound mode, activate ALL controllers
	for _, controller := range dnsConfig.ExternalDNSControllers {
		activeControllers = append(activeControllers, controller.Name)
	}

	return activeControllers
}

// mapGlobalConfigToDNSPolicies returns all DNSPolicies for reconciliation
func (r *DNSPolicyReconciler) mapGlobalConfigToDNSPolicies(
	ctx context.Context,
	obj client.Object,
) []reconcile.Request {
	// List all DNSPolicies and trigger reconcile
	var dnsPolicies routingv1alpha1.DNSPolicyList
	if err := r.List(ctx, &dnsPolicies); err != nil {
		return nil
	}

	var requests []reconcile.Request
	for _, policy := range dnsPolicies.Items {
		requests = append(requests, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      policy.Name,
				Namespace: policy.Namespace,
			},
		})
	}

	return requests
}

// updateStatusActive updates the DNSPolicy status to Active
func (r *DNSPolicyReconciler) updateStatusActive(
	ctx context.Context,
	dnsPolicy *routingv1alpha1.DNSPolicy,
	activeControllers []string,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	dnsPolicy.Status.Phase = consts.PhaseActive
	dnsPolicy.Status.Active = true
	dnsPolicy.Status.ActiveControllers = activeControllers
	meta.SetStatusCondition(&dnsPolicy.Status.Conditions, metav1.Condition{
		Type:               consts.ConditionTypeReady,
		Status:             metav1.ConditionTrue,
		ObservedGeneration: dnsPolicy.Generation,
		Reason:             consts.ReasonReconciliationSucceeded,
		Message:            "DNSPolicy is active",
	})

	if err := r.Status().Update(ctx, dnsPolicy); err != nil {
		if apierrors.IsConflict(err) {
			logger.Info("DNSPolicy status update conflict (Active), will retry")
			return ctrl.Result{Requeue: true}, nil
		}
		logger.Error(err, "failed to update DNSPolicy status to Active")
		return ctrl.Result{}, err
	}

	logger.Info("DNSPolicy reconciled successfully", "activeControllers", len(activeControllers))
	return ctrl.Result{}, nil
}

// updateStatusInactive updates the DNSPolicy status when policy doesn't match cluster identity
func (r *DNSPolicyReconciler) updateStatusInactive(
	ctx context.Context,
	dnsPolicy *routingv1alpha1.DNSPolicy,
	reason string,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	dnsPolicy.Status.Active = false
	dnsPolicy.Status.ActiveControllers = []string{}
	meta.SetStatusCondition(&dnsPolicy.Status.Conditions, metav1.Condition{
		Type:               consts.ConditionTypeReady,
		Status:             metav1.ConditionFalse,
		ObservedGeneration: dnsPolicy.Generation,
		Reason:             consts.ReasonPolicyInactive,
		Message:            fmt.Sprintf("Policy not active for this cluster: %s", reason),
	})

	if err := r.Status().Update(ctx, dnsPolicy); err != nil {
		if apierrors.IsConflict(err) {
			logger.Info("DNSPolicy status update conflict (Inactive), will retry")
			return ctrl.Result{Requeue: true}, nil
		}
		logger.Error(err, "failed to update DNSPolicy status to Inactive")
		return ctrl.Result{}, err
	}

	logger.Info("DNSPolicy is inactive", "reason", reason)
	return ctrl.Result{}, nil
}

// updateStatusPending updates the DNSPolicy status to Pending
func (r *DNSPolicyReconciler) updateStatusPending(
	ctx context.Context,
	dnsPolicy *routingv1alpha1.DNSPolicy,
	reason, message string,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	meta.SetStatusCondition(&dnsPolicy.Status.Conditions, metav1.Condition{
		Type:               consts.ConditionTypeReady,
		Status:             metav1.ConditionFalse,
		ObservedGeneration: dnsPolicy.Generation,
		Reason:             reason,
		Message:            message,
	})

	if err := r.Status().Update(ctx, dnsPolicy); err != nil {
		if apierrors.IsConflict(err) {
			logger.Info("DNSPolicy status update conflict (Pending), will retry")
			return ctrl.Result{Requeue: true}, nil
		}
		logger.Error(err, "failed to update DNSPolicy status to Pending")
		return ctrl.Result{}, err
	}

	logger.Info("DNSPolicy marked as Pending", "reason", reason, "message", message)
	return ctrl.Result{}, nil
}

// updateStatusFailed updates the DNSPolicy status to Failed
func (r *DNSPolicyReconciler) updateStatusFailed(
	ctx context.Context,
	dnsPolicy *routingv1alpha1.DNSPolicy,
	reason, message string,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	meta.SetStatusCondition(&dnsPolicy.Status.Conditions, metav1.Condition{
		Type:               consts.ConditionTypeReady,
		Status:             metav1.ConditionFalse,
		ObservedGeneration: dnsPolicy.Generation,
		Reason:             reason,
		Message:            message,
	})

	if err := r.Status().Update(ctx, dnsPolicy); err != nil {
		if apierrors.IsConflict(err) {
			logger.Info("DNSPolicy status update conflict (Failed), will retry")
			return ctrl.Result{Requeue: true}, nil
		}
		logger.Error(err, "failed to update DNSPolicy status to Failed")
		return ctrl.Result{}, err
	}

	logger.Info("DNSPolicy marked as Failed", "reason", reason, "message", message)
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *DNSPolicyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&routingv1alpha1.DNSPolicy{}).
		Watches(
			&clusterv1alpha1.ClusterIdentity{},
			handler.EnqueueRequestsFromMapFunc(r.mapGlobalConfigToDNSPolicies),
		).
		Watches(
			&clusterv1alpha1.DNSConfiguration{},
			handler.EnqueueRequestsFromMapFunc(r.mapGlobalConfigToDNSPolicies),
		).
		Complete(r)
}
