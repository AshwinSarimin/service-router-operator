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
	"math/rand"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	chaosv1alpha1 "github.com/AshwinSarimin/service-router-operator/pod-restarter/api/v1alpha1"
)

const (
	ConditionTypeReady = "Ready"
)

// PodRestarterReconciler reconciles a PodRestarter object
type PodRestarterReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=chaos.platform.com,resources=podrestarters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=chaos.platform.com,resources=podrestarters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=chaos.platform.com,resources=podrestarters/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch;delete

// Reconcile is the main reconciliation loop
func (r *PodRestarterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// Fetch the PodRestarter instance
	podRestarter := &chaosv1alpha1.PodRestarter{}
	if err := r.Get(ctx, req.NamespacedName, podRestarter); err != nil {
		if errors.IsNotFound(err) {
			log.Info("PodRestarter resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to get PodRestarter")
		return ctrl.Result{}, err
	}

	// Check if suspended
	if podRestarter.Spec.Suspend {
		log.Info("PodRestarter is suspended, skipping reconciliation")
		r.setCondition(podRestarter, ConditionTypeReady, metav1.ConditionFalse, "Suspended", "Pod restarts are suspended")
		if err := r.Status().Update(ctx, podRestarter); err != nil {
			log.Error(err, "Failed to update status")
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: r.getInterval(podRestarter)}, nil
	}

	// Find matching pods
	podList := &corev1.PodList{}
	listOpts := []client.ListOption{
		client.InNamespace(podRestarter.Namespace),
	}

	if podRestarter.Spec.Selector.MatchLabels != nil {
		listOpts = append(listOpts, client.MatchingLabels(podRestarter.Spec.Selector.MatchLabels))
	}

	if err := r.List(ctx, podList, listOpts...); err != nil {
		log.Error(err, "Failed to list pods")
		r.setCondition(podRestarter, ConditionTypeReady, metav1.ConditionFalse, "ListFailed", fmt.Sprintf("Failed to list pods: %v", err))
		if statusErr := r.Status().Update(ctx, podRestarter); statusErr != nil {
			log.Error(statusErr, "Failed to update status")
		}
		return ctrl.Result{}, err
	}

	// Update matching pods count
	podRestarter.Status.MatchingPods = int32(len(podList.Items))
	podRestarter.Status.ObservedGeneration = podRestarter.Generation

	// Calculate if it's time to restart
	interval := r.getInterval(podRestarter)
	shouldRestart := false

	if podRestarter.Status.LastRestartTime == nil {
		shouldRestart = true
		log.Info("First restart - will restart pods immediately")
	} else {
		timeSinceLastRestart := time.Since(podRestarter.Status.LastRestartTime.Time)
		shouldRestart = timeSinceLastRestart >= interval
		log.V(1).Info("Checking restart interval",
			"timeSinceLastRestart", timeSinceLastRestart,
			"interval", interval,
			"shouldRestart", shouldRestart)
	}

	if shouldRestart {
		if len(podList.Items) == 0 {
			log.Info("No pods matching selector, skipping restart")
			r.setCondition(podRestarter, ConditionTypeReady, metav1.ConditionTrue, "NoPodsFound", "No pods match the selector")
		} else {
			// Restart pods based on strategy
			restarted, err := r.restartPods(ctx, podRestarter, podList.Items)
			if err != nil {
				log.Error(err, "Failed to restart pods")
				r.setCondition(podRestarter, ConditionTypeReady, metav1.ConditionFalse, "RestartFailed", fmt.Sprintf("Failed to restart pods: %v", err))
				if statusErr := r.Status().Update(ctx, podRestarter); statusErr != nil {
					log.Error(statusErr, "Failed to update status")
				}
				return ctrl.Result{}, err
			}

			log.Info("Successfully restarted pods", "count", restarted, "strategy", podRestarter.Spec.Strategy)

			// Update status
			podRestarter.Status.TotalRestarts += int32(restarted)
			now := metav1.Now()
			podRestarter.Status.LastRestartTime = &now
			nextRestart := metav1.NewTime(now.Add(interval))
			podRestarter.Status.NextRestartTime = &nextRestart

			r.setCondition(podRestarter, ConditionTypeReady, metav1.ConditionTrue, "Restarted", fmt.Sprintf("Restarted %d pod(s)", restarted))
		}
	} else {
		nextRestart := podRestarter.Status.LastRestartTime.Time.Add(interval)
		log.V(1).Info("Not time to restart yet", "nextRestartTime", nextRestart)
		podRestarter.Status.NextRestartTime = &metav1.Time{Time: nextRestart}
		r.setCondition(podRestarter, ConditionTypeReady, metav1.ConditionTrue, "Waiting", "Waiting for next restart interval")
	}

	// Update status
	if err := r.Status().Update(ctx, podRestarter); err != nil {
		log.Error(err, "Failed to update status")
		return ctrl.Result{}, err
	}

	// Requeue after interval
	return ctrl.Result{RequeueAfter: interval}, nil
}

// restartPods restarts pods based on the strategy
func (r *PodRestarterReconciler) restartPods(ctx context.Context, podRestarter *chaosv1alpha1.PodRestarter, pods []corev1.Pod) (int, error) {
	log := log.FromContext(ctx)
	restarted := 0
	strategy := podRestarter.Spec.Strategy
	if strategy == "" {
		strategy = "all"
	}

	maxConcurrent := int(podRestarter.Spec.MaxConcurrent)
	if maxConcurrent == 0 {
		maxConcurrent = len(pods)
	}

	switch strategy {
	case "random-one":
		if len(pods) > 0 {
			randomIndex := rand.Intn(len(pods))
			pod := pods[randomIndex]
			log.Info("Restarting pod (random-one strategy)", "pod", pod.Name)
			if err := r.Delete(ctx, &pod); err != nil {
				if !errors.IsNotFound(err) {
					return restarted, fmt.Errorf("failed to delete pod %s: %w", pod.Name, err)
				}
			}
			restarted++
		}

	case "rolling":
		for i, pod := range pods {
			if restarted >= maxConcurrent {
				log.Info("Reached maxConcurrent limit", "restarted", restarted, "maxConcurrent", maxConcurrent)
				break
			}
			log.Info("Restarting pod (rolling strategy)", "pod", pod.Name, "index", i+1, "total", len(pods))
			if err := r.Delete(ctx, &pod); err != nil {
				if !errors.IsNotFound(err) {
					log.Error(err, "Failed to delete pod", "pod", pod.Name)
					continue
				}
			}
			restarted++
		}

	case "all":
		fallthrough
	default:
		for _, pod := range pods {
			if restarted >= maxConcurrent && maxConcurrent != len(pods) {
				log.Info("Reached maxConcurrent limit", "restarted", restarted, "maxConcurrent", maxConcurrent)
				break
			}
			log.Info("Restarting pod (all strategy)", "pod", pod.Name)
			if err := r.Delete(ctx, &pod); err != nil {
				if !errors.IsNotFound(err) {
					log.Error(err, "Failed to delete pod", "pod", pod.Name)
					continue
				}
			}
			restarted++
		}
	}

	return restarted, nil
}

// getInterval returns the restart interval duration
func (r *PodRestarterReconciler) getInterval(podRestarter *chaosv1alpha1.PodRestarter) time.Duration {
	minutes := podRestarter.Spec.IntervalMinutes
	if minutes == 0 {
		minutes = 5
	}
	return time.Duration(minutes) * time.Minute
}

// setCondition sets a condition on the PodRestarter status
func (r *PodRestarterReconciler) setCondition(podRestarter *chaosv1alpha1.PodRestarter, conditionType string, status metav1.ConditionStatus, reason, message string) {
	condition := metav1.Condition{
		Type:               conditionType,
		Status:             status,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: metav1.Now(),
		ObservedGeneration: podRestarter.Generation,
	}

	found := false
	for i, existingCondition := range podRestarter.Status.Conditions {
		if existingCondition.Type == conditionType {
			if existingCondition.Status != status {
				podRestarter.Status.Conditions[i] = condition
			} else {
				podRestarter.Status.Conditions[i].Message = message
				podRestarter.Status.Conditions[i].Reason = reason
				podRestarter.Status.Conditions[i].LastTransitionTime = metav1.Now()
			}
			found = true
			break
		}
	}

	if !found {
		podRestarter.Status.Conditions = append(podRestarter.Status.Conditions, condition)
	}
}

// SetupWithManager sets up the controller with the Manager
func (r *PodRestarterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&chaosv1alpha1.PodRestarter{}).
		Complete(r)
}
