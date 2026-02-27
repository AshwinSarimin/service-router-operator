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

package cluster

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
	"github.com/vecozo/service-router-operator/internal/clusteridentity"
	"github.com/vecozo/service-router-operator/internal/dnsconfiguration"
	"github.com/vecozo/service-router-operator/pkg/consts"
)

// ClusterIdentityReconciler reconciles a ClusterIdentity object
type ClusterIdentityReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=cluster.router.io,resources=clusteridentities,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=cluster.router.io,resources=clusteridentities/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=cluster.router.io,resources=clusteridentities/finalizers,verbs=update
//+kubebuilder:rbac:groups=cluster.router.io,resources=dnsconfigurations,verbs=get;list;watch
//+kubebuilder:rbac:groups=authentication.k8s.io,resources=tokenreviews,verbs=create
//+kubebuilder:rbac:groups=authorization.k8s.io,resources=subjectaccessreviews,verbs=create

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.16.0/pkg/reconcile
func (r *ClusterIdentityReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Retrieve the cluster identity to determine the current configuration and update the cache.
	var clusterIdentity clusterv1alpha1.ClusterIdentity
	if err := r.Get(ctx, req.NamespacedName, &clusterIdentity); err != nil {
		if apierrors.IsNotFound(err) {
			// Clear the cache when the resource is deleted to prevent other controllers from using stale data.
			clusteridentity.Clear()
			logger.Info("ClusterIdentity deleted", "name", req.Name, "namespace", req.Namespace, "action", "cleared cache")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "unable to fetch ClusterIdentity")
		return ctrl.Result{}, err
	}

	// Ensure singleton to maintain a single source of truth for cluster identity.
	if err := r.validateSingleton(ctx, &clusterIdentity); err != nil {
		logger.Error(err, "singleton validation failed")
		return r.updateStatusFailed(ctx, &clusterIdentity, consts.ReasonSingletonViolation, err.Error())
	}

	if err := r.validateSpec(&clusterIdentity); err != nil {
		logger.Error(err, "spec validation failed")
		return r.updateStatusFailed(ctx, &clusterIdentity, consts.ReasonInvalidSpec, err.Error())
	}

	// Validate adopted regions against DNSConfiguration (Soft dependency)
	// We validate even if empty to ensure any stale error conditions are cleared.
	if err := r.validateAdoptedRegions(ctx, &clusterIdentity); err != nil {
		// Log error but do not fail reconciliation if validation fails due to missing dependencies
		// This keeps the dependency "soft"
		logger.Info("Optional adopted regions validation skipped or failed", "error", err)
	}

	// Update the in-memory cache so other controllers (e.g. ServiceRoute) can access
	// the cluster's identity (region, environment, etc.) synchronously without API calls.
	identity := &clusteridentity.ClusterIdentity{
		Region:            clusterIdentity.Spec.Region,
		Cluster:           clusterIdentity.Spec.Cluster,
		Domain:            clusterIdentity.Spec.Domain,
		EnvironmentLetter: clusterIdentity.Spec.EnvironmentLetter,
		AdoptsRegions:     clusterIdentity.Spec.AdoptsRegions,
	}
	clusteridentity.Set(identity)

	return r.updateStatusActive(ctx, &clusterIdentity)
}

// validateAdoptedRegions checks if adopted regions exist in DNSConfiguration
// This is a soft dependency: if DNSConfiguration is missing, we don't block.
func (r *ClusterIdentityReconciler) validateAdoptedRegions(ctx context.Context, cr *clusterv1alpha1.ClusterIdentity) error {
	if len(cr.Spec.AdoptsRegions) == 0 {
		meta.SetStatusCondition(&cr.Status.Conditions, metav1.Condition{
			Type:               consts.ConditionTypeAdoptedRegionsValid,
			Status:             metav1.ConditionTrue,
			ObservedGeneration: cr.Generation,
			Reason:             consts.ReasonNoAdoptedRegions,
			Message:            "No adopted regions configured",
		})
		return nil
	}

	config, err := dnsconfiguration.Fetch(ctx, r.Client)
	if err != nil {
		// If listing fails (e.g., CRD not monitored or permission issue), return error to be logged but ignored
		return fmt.Errorf("failed to fetch DNSConfiguration: %w", err)
	}

	existingRegions := make(map[string]bool)
	if config != nil {
		for _, controller := range config.ExternalDNSControllers {
			existingRegions[controller.Region] = true
		}
	}

	var invalid []string
	for _, region := range cr.Spec.AdoptsRegions {
		if !existingRegions[region] {
			invalid = append(invalid, region)
		}
	}

	var condition metav1.Condition
	if len(invalid) > 0 {
		condition = metav1.Condition{
			Type:               consts.ConditionTypeAdoptedRegionsValid,
			Status:             metav1.ConditionFalse,
			ObservedGeneration: cr.Generation,
			Reason:             consts.ReasonAdoptedRegionNotFound,
			Message:            fmt.Sprintf("Adopted regions %v have no matching controller in DNSConfiguration", invalid),
		}
	} else {
		condition = metav1.Condition{
			Type:               consts.ConditionTypeAdoptedRegionsValid,
			Status:             metav1.ConditionTrue,
			ObservedGeneration: cr.Generation,
			Reason:             consts.ReasonAllAdoptedRegionsValid,
			Message:            "All adopted regions have matching controllers",
		}
	}

	meta.SetStatusCondition(&cr.Status.Conditions, condition)
	return nil
}

// validateSingleton ensures only one ClusterIdentity resource exists
func (r *ClusterIdentityReconciler) validateSingleton(ctx context.Context, current *clusterv1alpha1.ClusterIdentity) error {
	var clusterIdentities clusterv1alpha1.ClusterIdentityList
	if err := r.List(ctx, &clusterIdentities); err != nil {
		return fmt.Errorf("failed to list ClusterIdentities: %w", err)
	}

	if len(clusterIdentities.Items) > 1 {
		return fmt.Errorf("only one ClusterIdentity resource is allowed per cluster, found %d", len(clusterIdentities.Items))
	}

	return nil
}

// validateSpec validates the ClusterIdentity spec fields
func (r *ClusterIdentityReconciler) validateSpec(cr *clusterv1alpha1.ClusterIdentity) error {
	if cr.Spec.Region == "" {
		return fmt.Errorf("region cannot be empty")
	}
	if cr.Spec.Cluster == "" {
		return fmt.Errorf("cluster cannot be empty")
	}
	if cr.Spec.Domain == "" {
		return fmt.Errorf("domain cannot be empty")
	}
	if cr.Spec.EnvironmentLetter == "" {
		return fmt.Errorf("environmentLetter cannot be empty")
	}
	return nil
}

// updateStatusActive updates the ClusterIdentity status to Active
func (r *ClusterIdentityReconciler) updateStatusActive(ctx context.Context, cr *clusterv1alpha1.ClusterIdentity) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	cr.Status.Phase = consts.PhaseActive
	meta.SetStatusCondition(&cr.Status.Conditions, metav1.Condition{
		Type:               consts.ConditionTypeReady,
		Status:             metav1.ConditionTrue,
		ObservedGeneration: cr.Generation,
		Reason:             consts.ReasonReconciliationSucceeded,
		Message:            "ClusterIdentity is active and cluster identity is cached",
	})

	if err := r.Status().Update(ctx, cr); err != nil {
		if apierrors.IsConflict(err) {
			logger.Info("ClusterIdentity status update conflict (Active), will retry")
			return ctrl.Result{Requeue: true}, nil
		}
		logger.Error(err, "failed to update ClusterIdentity status to Active")
		return ctrl.Result{}, err
	}

	logger.Info("ClusterIdentity reconciled successfully", "phase", cr.Status.Phase)
	return ctrl.Result{}, nil
}

// updateStatusFailed updates the ClusterIdentity status to Failed
func (r *ClusterIdentityReconciler) updateStatusFailed(ctx context.Context, cr *clusterv1alpha1.ClusterIdentity, reason, message string) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	cr.Status.Phase = consts.PhaseFailed
	meta.SetStatusCondition(&cr.Status.Conditions, metav1.Condition{
		Type:               consts.ConditionTypeReady,
		Status:             metav1.ConditionFalse,
		ObservedGeneration: cr.Generation,
		Reason:             reason,
		Message:            message,
	})

	if err := r.Status().Update(ctx, cr); err != nil {
		if apierrors.IsConflict(err) {
			logger.Info("ClusterIdentity status update conflict (Failed), will retry")
			return ctrl.Result{Requeue: true}, nil
		}
		logger.Error(err, "failed to update ClusterIdentity status to Failed")
		return ctrl.Result{}, err
	}

	logger.Info("ClusterIdentity marked as Failed", "reason", reason, "message", message)
	return ctrl.Result{}, nil
}

// mapDNSConfigToClusterIdentities triggers reconciliation for all ClusterIdentity resources
func (r *ClusterIdentityReconciler) mapDNSConfigToClusterIdentities(ctx context.Context, obj client.Object) []reconcile.Request {
	var identities clusterv1alpha1.ClusterIdentityList
	if err := r.List(ctx, &identities); err != nil {
		return nil
	}

	var requests []reconcile.Request
	for _, identity := range identities.Items {
		requests = append(requests, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name: identity.Name,
			},
		})
	}
	return requests
}

// SetupWithManager sets up the controller with the Manager.
func (r *ClusterIdentityReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&clusterv1alpha1.ClusterIdentity{}).
		Watches(
			&clusterv1alpha1.DNSConfiguration{},
			handler.EnqueueRequestsFromMapFunc(r.mapDNSConfigToClusterIdentities),
		).
		Complete(r)
}
