package controller

import (
    "context"
    "fmt"
    "github.com/go-logr/logr"
    "k8s.io/apimachinery/pkg/api/errors"
    "k8s.io/apimachinery/pkg/runtime"
    "sigs.k8s.io/controller-runtime/pkg/client"
    "sigs.k8s.io/controller-runtime/pkg/controller"
    "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
    "sigs.k8s.io/controller-runtime/pkg/reconcile"
    "sigs.k8s.io/controller-runtime/pkg/source"

    v1 "service-router-operator/api/v1"
)

// ServiceRouterReconciler manages the lifecycle of ServiceRouter resources
type ServiceRouterReconciler struct {
    Client client.Client
    Log    logr.Logger
    Scheme *runtime.Scheme
}

// Reconcile handles the reconciliation logic for ServiceRouter resources
func (r *ServiceRouterReconciler) Reconcile(req reconcile.Request) (reconcile.Result, error) {
    ctx := context.Background()
    log := r.Log.WithValues("servicerouter", req.NamespacedName)

    // Fetch the ServiceRouter instance
    serviceRouter := &v1.ServiceRouter{}
    err := r.Client.Get(ctx, req.NamespacedName, serviceRouter)
    if err != nil {
        if errors.IsNotFound(err) {
            log.Info("ServiceRouter resource not found. Ignoring since it must be deleted.")
            return reconcile.Result{}, nil
        }
        log.Error(err, "Failed to get ServiceRouter.")
        return reconcile.Result{}, err
    }

    // Implement your reconciliation logic here
    // For example, ensure that the desired state matches the actual state

    // Update the status of the ServiceRouter resource
    if err := r.updateStatus(serviceRouter); err != nil {
        log.Error(err, "Failed to update ServiceRouter status.")
        return reconcile.Result{}, err
    }

    return reconcile.Result{}, nil
}

// updateStatus updates the status of the ServiceRouter resource
func (r *ServiceRouterReconciler) updateStatus(serviceRouter *v1.ServiceRouter) error {
    // Example status update logic
    serviceRouter.Status.LastUpdated = time.Now().Format(time.RFC3339)
    return r.Client.Status().Update(context.Background(), serviceRouter)
}

// SetupWithManager sets up the controller with the Manager
func (r *ServiceRouterReconciler) SetupWithManager(mgr controller.Manager) error {
    return ctrl.NewControllerManagedBy(mgr).
        For(&v1.ServiceRouter{}).
        Owns(&corev1.Pod{}). // Example of owned resources
        Complete(r)
}