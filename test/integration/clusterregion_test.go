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
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1alpha1 "github.com/vecozo/service-router-operator/api/cluster/v1alpha1"
	routingv1alpha1 "github.com/vecozo/service-router-operator/api/routing/v1alpha1"
	"github.com/vecozo/service-router-operator/internal/clusteridentity"
)

var _ = Describe("ClusterIdentity Integration", func() {
	var clusterIdentity *clusterv1alpha1.ClusterIdentity
	var dnsPolicy *routingv1alpha1.DNSPolicy
	var dnsConfig *clusterv1alpha1.DNSConfiguration

	BeforeEach(func() {
		timestamp := time.Now().UnixNano()
		// Create initial ClusterIdentity
		clusterIdentity = &clusterv1alpha1.ClusterIdentity{
			ObjectMeta: metav1.ObjectMeta{
				Name: fmt.Sprintf("test-cluster-identity-%d", timestamp),
			},
			Spec: clusterv1alpha1.ClusterIdentitySpec{
				Region:            "neu",
				Cluster:           "aks01",
				Domain:            "example.com",
				EnvironmentLetter: "d",
			},
		}
		Expect(k8sClient.Create(ctx, clusterIdentity)).To(Succeed())
		WaitForCondition(clusterIdentity, "Ready", metav1.ConditionTrue)

		// Create or Update DNSConfiguration
		dnsConfig = &clusterv1alpha1.DNSConfiguration{
			ObjectMeta: metav1.ObjectMeta{
				Name: "default",
			},
			Spec: clusterv1alpha1.DNSConfigurationSpec{
				ExternalDNSControllers: []clusterv1alpha1.ExternalDNSController{
					{Name: "external-dns-neu", Region: "neu"},
					{Name: "external-dns-weu", Region: "weu"},
				},
			},
		}

		existingConfig := &clusterv1alpha1.DNSConfiguration{}
		if err := k8sClient.Get(ctx, types.NamespacedName{Name: "default"}, existingConfig); err != nil {
			Expect(k8sClient.Create(ctx, dnsConfig)).To(Succeed())
		} else {
			existingConfig.Spec = dnsConfig.Spec
			Expect(k8sClient.Update(ctx, existingConfig)).To(Succeed())
		}

		// Create DNSPolicy in Active mode
		dnsPolicy = &routingv1alpha1.DNSPolicy{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("test-policy-integration-%d", timestamp),
				Namespace: "default",
			},
			Spec: routingv1alpha1.DNSPolicySpec{
				Mode: "Active",
			},
		}
		Expect(k8sClient.Create(ctx, dnsPolicy)).To(Succeed())
		WaitForCondition(dnsPolicy, "Ready", metav1.ConditionTrue)
	})

	AfterEach(func() {
		if dnsPolicy != nil {
			DeleteObject(dnsPolicy)
		}
		if clusterIdentity != nil {
			DeleteObject(clusterIdentity)
		}
		// We don't delete DNSConfiguration as it might be shared/global or other tests rely on it existing/default
	})

	Context("When ClusterIdentity changes affect DNSPolicy", func() {
		It("Should update DNSPolicy active controllers when region changes", func() {
			// Verify initial active controllers (should be neu)
			Eventually(func() []string {
				var policy routingv1alpha1.DNSPolicy
				if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(dnsPolicy), &policy); err != nil {
					return nil
				}
				return policy.Status.ActiveControllers
			}, timeout, interval).Should(Equal([]string{"external-dns-neu"}))

			// Update ClusterIdentity to change region
			Eventually(func() error {
				var cr clusterv1alpha1.ClusterIdentity
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(clusterIdentity), &cr)
				if err != nil {
					return err
				}
				cr.Spec.Region = "weu"
				return k8sClient.Update(ctx, &cr)
			}, timeout, interval).Should(Succeed())

			// Verify cluster identity cache is updated
			Eventually(func() string {
				identity := clusteridentity.Get()
				if identity == nil {
					return ""
				}
				return identity.Region
			}, timeout, interval).Should(Equal("weu"))

			// Trigger DNSPolicy reconciliation by adding a label
			Eventually(func() error {
				var policy routingv1alpha1.DNSPolicy
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(dnsPolicy), &policy)
				if err != nil {
					return err
				}
				if policy.Labels == nil {
					policy.Labels = make(map[string]string)
				}
				policy.Labels["trigger"] = "reconcile"
				return k8sClient.Update(ctx, &policy)
			}, timeout, interval).Should(Succeed())

			// Verify active controllers updated to weu
			Eventually(func() []string {
				var policy routingv1alpha1.DNSPolicy
				if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(dnsPolicy), &policy); err != nil {
					return nil
				}
				return policy.Status.ActiveControllers
			}, timeout, interval).Should(Equal([]string{"external-dns-weu"}))
		})
	})
})
