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

package integration

import (
	"time"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1alpha1 "github.com/vecozo/service-router-operator/api/cluster/v1alpha1"
	routingv1alpha1 "github.com/vecozo/service-router-operator/api/routing/v1alpha1"
)

// Test timing constants
const (
	timeout  = time.Second * 60
	interval = time.Millisecond * 500
)

// CreateNamespace creates a new namespace
func CreateNamespace(name string) {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
	Expect(k8sClient.Create(ctx, ns)).To(Succeed())
}

// GetObject returns a function that gets an object from the cluster
func GetObject(key types.NamespacedName, obj client.Object) func() error {
	return func() error {
		return k8sClient.Get(ctx, key, obj)
	}
}

// DeleteObject deletes an object from the cluster and waits for it to be deleted
func DeleteObject(obj client.Object) {
	if obj == nil {
		return
	}
	// Check if it exists before trying to delete
	if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(obj), obj); err != nil {
		if errors.IsNotFound(err) {
			return
		}
		// If other error, let the delete try and fail or succeed
	}

	Expect(k8sClient.Delete(ctx, obj)).To(Succeed())

	Eventually(func() bool {
		// Enable this check to ensure the object is gone from the API server
		if !errors.IsNotFound(k8sClient.Get(ctx, client.ObjectKeyFromObject(obj), obj)) {
			return false
		}
		// Also ensure the object is gone from the controller's cache
		if k8sManager != nil {
			// We need a deep copy or a new object to avoid race conditions or dirtying the object
			// But for IsNotFound check, using the same obj is usually acceptable if we ignore the content
			return errors.IsNotFound(k8sManager.GetClient().Get(ctx, client.ObjectKeyFromObject(obj), obj))
		}
		return true
	}, timeout, interval).Should(BeTrue())
}

// WaitForCondition waits for a condition to become true on an object
func WaitForCondition(obj client.Object, conditionType string, status metav1.ConditionStatus) {
	Eventually(func() bool {
		if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(obj), obj); err != nil {
			return false
		}

		// Extract conditions based on object type
		var conditions []metav1.Condition
		switch typed := obj.(type) {
		case *clusterv1alpha1.ClusterIdentity:
			conditions = typed.Status.Conditions
		case *routingv1alpha1.Gateway:
			conditions = typed.Status.Conditions
		case *routingv1alpha1.DNSPolicy:
			conditions = typed.Status.Conditions
		case *routingv1alpha1.ServiceRoute:
			conditions = typed.Status.Conditions
		default:
			return false
		}

		for _, cond := range conditions {
			if cond.Type == conditionType && cond.Status == status {
				return true
			}
		}
		return false
	}, timeout, interval).Should(BeTrue())
}
