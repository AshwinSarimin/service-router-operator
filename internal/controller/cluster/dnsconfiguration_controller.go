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
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	clusterv1alpha1 "github.com/vecozo/service-router-operator/api/cluster/v1alpha1"
	"github.com/vecozo/service-router-operator/internal/dnsconfiguration"
)

// DNSConfigurationReconciler reconciles a DNSConfiguration object
type DNSConfigurationReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=cluster.router.io,resources=dnsconfigurations,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=cluster.router.io,resources=dnsconfigurations/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=cluster.router.io,resources=dnsconfigurations/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *DNSConfigurationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Retrieve the desired DNS configuration to update the cache.
	var dnsConfig clusterv1alpha1.DNSConfiguration
	if err := r.Get(ctx, req.NamespacedName, &dnsConfig); err != nil {
		if apierrors.IsNotFound(err) {
			// Clear the cache when the resource is deleted to prevent other controllers from using stale data.
			dnsconfiguration.Clear()
			logger.Info("DNSConfiguration deleted", "name", req.Name, "namespace", req.Namespace, "action", "cleared cache")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "unable to fetch DNSConfiguration")
		return ctrl.Result{}, err
	}

	// Ensure singleton to maintain a single source of truth for DNS configuration.
	if err := r.validateSingleton(ctx, &dnsConfig); err != nil {
		logger.Error(err, "singleton validation failed")
		return r.updateStatusFailed(ctx, &dnsConfig, "SingletonViolation", err.Error())
	}

	if err := r.validateSpec(&dnsConfig); err != nil {
		logger.Error(err, "spec validation failed")
		return r.updateStatusFailed(ctx, &dnsConfig, "InvalidSpec", err.Error())
	}

	// Update the in-memory cache so other controllers (DNSPolicy, ServiceRoute) can access
	// this configuration synchronously without API calls.
	config := &dnsconfiguration.DNSConfiguration{
		ExternalDNSControllers: make([]dnsconfiguration.ExternalDNSController, len(dnsConfig.Spec.ExternalDNSControllers)),
	}

	for i, c := range dnsConfig.Spec.ExternalDNSControllers {
		config.ExternalDNSControllers[i] = dnsconfiguration.ExternalDNSController{
			Name:   c.Name,
			Region: c.Region,
		}
	}

	dnsconfiguration.Set(config)

	// Update status to Ready
	return r.updateStatusReady(ctx, &dnsConfig)
}

// validateSingleton ensures only one DNSConfiguration resource exists per cluster
func (r *DNSConfigurationReconciler) validateSingleton(ctx context.Context, current *clusterv1alpha1.DNSConfiguration) error {
	var dnsConfigs clusterv1alpha1.DNSConfigurationList
	if err := r.List(ctx, &dnsConfigs); err != nil {
		return fmt.Errorf("failed to list DNSConfigurations: %w", err)
	}

	if len(dnsConfigs.Items) > 1 {
		return fmt.Errorf("only one DNSConfiguration resource is allowed per cluster, found %d", len(dnsConfigs.Items))
	}

	return nil
}

// validateSpec validates the DNSConfiguration spec fields
func (r *DNSConfigurationReconciler) validateSpec(cr *clusterv1alpha1.DNSConfiguration) error {
	if len(cr.Spec.ExternalDNSControllers) == 0 {
		return fmt.Errorf("externalDNSControllers cannot be empty")
	}
	for i, c := range cr.Spec.ExternalDNSControllers {
		if c.Name == "" {
			return fmt.Errorf("externalDNSControllers[%d].name cannot be empty", i)
		}
		if c.Region == "" {
			return fmt.Errorf("externalDNSControllers[%d].region cannot be empty", i)
		}
	}
	return nil
}

// updateStatusReady updates the DNSConfiguration status to Ready
func (r *DNSConfigurationReconciler) updateStatusReady(ctx context.Context, cr *clusterv1alpha1.DNSConfiguration) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	meta.SetStatusCondition(&cr.Status.Conditions, metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionTrue,
		ObservedGeneration: cr.Generation,
		Reason:             "ReconciliationSucceeded",
		Message:            "DNSConfiguration is valid and cached",
	})

	if err := r.Status().Update(ctx, cr); err != nil {
		if apierrors.IsConflict(err) {
			logger.Info("DNSConfiguration status update conflict (Ready), will retry")
			return ctrl.Result{Requeue: true}, nil
		}
		logger.Error(err, "failed to update DNSConfiguration status to Ready")
		return ctrl.Result{}, err
	}

	logger.Info("DNSConfiguration reconciled successfully")
	return ctrl.Result{}, nil
}

// updateStatusFailed updates the DNSConfiguration status to Failed
func (r *DNSConfigurationReconciler) updateStatusFailed(ctx context.Context, cr *clusterv1alpha1.DNSConfiguration, reason, message string) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	meta.SetStatusCondition(&cr.Status.Conditions, metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionFalse,
		ObservedGeneration: cr.Generation,
		Reason:             reason,
		Message:            message,
	})

	if err := r.Status().Update(ctx, cr); err != nil {
		if apierrors.IsConflict(err) {
			logger.Info("DNSConfiguration status update conflict (Failed), will retry")
			return ctrl.Result{Requeue: true}, nil
		}
		logger.Error(err, "failed to update DNSConfiguration status to Failed")
		return ctrl.Result{}, err
	}

	logger.Info("DNSConfiguration marked as Failed", "reason", reason, "message", message)
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *DNSConfigurationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&clusterv1alpha1.DNSConfiguration{}).
		Complete(r)
}
