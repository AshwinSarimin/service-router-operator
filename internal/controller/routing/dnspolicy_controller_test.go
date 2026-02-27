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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1alpha1 "github.com/vecozo/service-router-operator/api/cluster/v1alpha1"
	routingv1alpha1 "github.com/vecozo/service-router-operator/api/routing/v1alpha1"
)

var _ = Describe("DNSPolicy Controller", func() {
	const (
		timeout  = time.Second * 10
		interval = time.Millisecond * 250
	)

	var clusterIdentity *clusterv1alpha1.ClusterIdentity
	var dnsConfig *clusterv1alpha1.DNSConfiguration

	Context("When reconciling a DNSPolicy", func() {
		ctx := context.Background()

		BeforeEach(func() {
			// Create ClusterIdentity so controllers have cluster identity
			clusterIdentity = &clusterv1alpha1.ClusterIdentity{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-cluster-identity-dnspolicy",
				},
				Spec: clusterv1alpha1.ClusterIdentitySpec{
					Region:            "neu",
					Cluster:           "aks01",
					Domain:            "example.com",
					EnvironmentLetter: "d",
				},
			}
			if err := k8sClient.Create(ctx, clusterIdentity); err != nil {
				if apierrors.IsAlreadyExists(err) {
					// Update existing to ensure correct spec
					existing := &clusterv1alpha1.ClusterIdentity{}
					Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(clusterIdentity), existing)).To(Succeed())
					existing.Spec = clusterIdentity.Spec
					Expect(k8sClient.Update(ctx, existing)).To(Succeed())
				} else {
					Expect(err).To(Succeed())
				}
			}

			// Create DNSConfiguration
			dnsConfig = &clusterv1alpha1.DNSConfiguration{
				ObjectMeta: metav1.ObjectMeta{
					Name: "default",
				},
				Spec: clusterv1alpha1.DNSConfigurationSpec{
					ExternalDNSControllers: []clusterv1alpha1.ExternalDNSController{
						{Name: "external-dns-neu", Region: "neu"},
						{Name: "external-dns-weu", Region: "weu"},
						{Name: "external-dns-frc", Region: "frc"},
						{Name: "external-dns-neu-1", Region: "neu"},
						{Name: "external-dns-neu-2", Region: "neu"},
					},
				},
			}
			if err := k8sClient.Create(ctx, dnsConfig); err != nil {
				if apierrors.IsAlreadyExists(err) {
					// Update existing to ensure correct spec
					existing := &clusterv1alpha1.DNSConfiguration{}
					Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(dnsConfig), existing)).To(Succeed())
					existing.Spec = dnsConfig.Spec
					Expect(k8sClient.Update(ctx, existing)).To(Succeed())
				} else {
					Expect(err).To(Succeed())
				}
			}
		})

		AfterEach(func() {
			// Clean up ClusterIdentity
			if clusterIdentity != nil {
				_ = k8sClient.Delete(ctx, clusterIdentity)
				Eventually(func() bool {
					return apierrors.IsNotFound(k8sClient.Get(ctx, client.ObjectKeyFromObject(clusterIdentity), clusterIdentity))
				}, timeout, interval).Should(BeTrue())
			}
			if dnsConfig != nil {
				_ = k8sClient.Delete(ctx, dnsConfig)
				Eventually(func() bool {
					return apierrors.IsNotFound(k8sClient.Get(ctx, client.ObjectKeyFromObject(dnsConfig), dnsConfig))
				}, timeout, interval).Should(BeTrue())
			}
		})

		It("should reconcile DNSPolicy with Active mode", func() {
			dnsPolicy := &routingv1alpha1.DNSPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-dnspolicy-active",
					Namespace: "default",
				},
				Spec: routingv1alpha1.DNSPolicySpec{
					Mode: "Active",
				},
			}

			Expect(k8sClient.Create(ctx, dnsPolicy)).Should(Succeed())

			dnsPolicyLookupKey := types.NamespacedName{
				Name:      dnsPolicy.Name,
				Namespace: dnsPolicy.Namespace,
			}
			createdDNSPolicy := &routingv1alpha1.DNSPolicy{}

			// Verify active controllers contain only controllers in same region
			// In DNSConfiguration we have external-dns-neu, external-dns-neu-1, external-dns-neu-2 for region neu
			Eventually(func() []string {
				err := k8sClient.Get(ctx, dnsPolicyLookupKey, createdDNSPolicy)
				if err != nil {
					return nil
				}
				return createdDNSPolicy.Status.ActiveControllers
			}, timeout, interval).Should(ConsistOf("external-dns-neu", "external-dns-neu-1", "external-dns-neu-2"))

			Eventually(func() metav1.ConditionStatus {
				err := k8sClient.Get(ctx, dnsPolicyLookupKey, createdDNSPolicy)
				if err != nil {
					return metav1.ConditionUnknown
				}
				for _, cond := range createdDNSPolicy.Status.Conditions {
					if cond.Type == "Ready" {
						return cond.Status
					}
				}
				return metav1.ConditionUnknown
			}, timeout, interval).Should(Equal(metav1.ConditionTrue))

			Expect(k8sClient.Delete(ctx, dnsPolicy)).Should(Succeed())
		})

		It("should reconcile DNSPolicy with RegionBound mode matching cluster region", func() {
			dnsPolicy := &routingv1alpha1.DNSPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-dnspolicy-regionbound",
					Namespace: "default",
				},
				Spec: routingv1alpha1.DNSPolicySpec{
					Mode:         "RegionBound",
					SourceRegion: "neu",
				},
			}

			Expect(k8sClient.Create(ctx, dnsPolicy)).Should(Succeed())

			dnsPolicyLookupKey := types.NamespacedName{
				Name:      dnsPolicy.Name,
				Namespace: dnsPolicy.Namespace,
			}
			createdDNSPolicy := &routingv1alpha1.DNSPolicy{}

			Eventually(func() bool {
				err := k8sClient.Get(ctx, dnsPolicyLookupKey, createdDNSPolicy)
				if err != nil {
					return false
				}
				return createdDNSPolicy.Status.Active
			}, timeout, interval).Should(BeTrue())

			// Verify active controllers contain ALL controllers (RegionBound activates all)
			Eventually(func() []string {
				err := k8sClient.Get(ctx, dnsPolicyLookupKey, createdDNSPolicy)
				if err != nil {
					return nil
				}
				return createdDNSPolicy.Status.ActiveControllers
			}, timeout, interval).Should(ConsistOf("external-dns-neu", "external-dns-weu", "external-dns-frc", "external-dns-neu-1", "external-dns-neu-2"))

			Eventually(func() metav1.ConditionStatus {
				err := k8sClient.Get(ctx, dnsPolicyLookupKey, createdDNSPolicy)
				if err != nil {
					return metav1.ConditionUnknown
				}
				for _, cond := range createdDNSPolicy.Status.Conditions {
					if cond.Type == "Ready" {
						return cond.Status
					}
				}
				return metav1.ConditionUnknown
			}, timeout, interval).Should(Equal(metav1.ConditionTrue))

			Expect(k8sClient.Delete(ctx, dnsPolicy)).Should(Succeed())
		})

		It("should fail validation for invalid mode", func() {
			dnsPolicy := &routingv1alpha1.DNSPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-dnspolicy-invalid-mode",
					Namespace: "default",
				},
				Spec: routingv1alpha1.DNSPolicySpec{
					Mode: "InvalidMode",
				},
			}

			err := k8sClient.Create(ctx, dnsPolicy)
			Expect(err).Should(HaveOccurred())
			Expect(err.Error()).Should(ContainSubstring("spec.mode"))
		})

		It("should mark policy as inactive when sourceRegion doesn't match", func() {
			dnsPolicy := &routingv1alpha1.DNSPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-dnspolicy-wrong-region",
					Namespace: "default",
				},
				Spec: routingv1alpha1.DNSPolicySpec{
					Mode:         "RegionBound",
					SourceRegion: "weu", // Doesn't match cluster region (neu)
				},
			}

			Expect(k8sClient.Create(ctx, dnsPolicy)).Should(Succeed())

			dnsPolicyLookupKey := types.NamespacedName{
				Name:      dnsPolicy.Name,
				Namespace: dnsPolicy.Namespace,
			}
			createdDNSPolicy := &routingv1alpha1.DNSPolicy{}

			Eventually(func() bool {
				err := k8sClient.Get(ctx, dnsPolicyLookupKey, createdDNSPolicy)
				if err != nil {
					return true // Return true to fail the test
				}
				return !createdDNSPolicy.Status.Active
			}, timeout, interval).Should(BeTrue())

			Eventually(func() int {
				err := k8sClient.Get(ctx, dnsPolicyLookupKey, createdDNSPolicy)
				if err != nil {
					return -1
				}
				return len(createdDNSPolicy.Status.ActiveControllers)
			}, timeout, interval).Should(Equal(0))

			Eventually(func() bool {
				err := k8sClient.Get(ctx, dnsPolicyLookupKey, createdDNSPolicy)
				if err != nil {
					return false
				}
				for _, cond := range createdDNSPolicy.Status.Conditions {
					if cond.Type == "Ready" && cond.Status == metav1.ConditionFalse &&
						cond.Reason == "PolicyInactive" {
						return true
					}
				}
				return false
			}, timeout, interval).Should(BeTrue())

			Expect(k8sClient.Delete(ctx, dnsPolicy)).Should(Succeed())
		})

		It("should requeue when ClusterIdentity is not available", func() {
			// Delete ClusterIdentity to simulate cluster identity not being available
			Expect(k8sClient.Delete(ctx, clusterIdentity)).Should(Succeed())

			dnsPolicy := &routingv1alpha1.DNSPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-dnspolicy-no-identity",
					Namespace: "default",
				},
				Spec: routingv1alpha1.DNSPolicySpec{
					Mode: "Active",
				},
			}

			Expect(k8sClient.Create(ctx, dnsPolicy)).Should(Succeed())

			dnsPolicyLookupKey := types.NamespacedName{
				Name:      dnsPolicy.Name,
				Namespace: dnsPolicy.Namespace,
			}
			createdDNSPolicy := &routingv1alpha1.DNSPolicy{}

			Eventually(func() bool {
				err := k8sClient.Get(ctx, dnsPolicyLookupKey, createdDNSPolicy)
				if err != nil {
					return false
				}
				for _, cond := range createdDNSPolicy.Status.Conditions {
					if cond.Type == "Ready" && cond.Status == metav1.ConditionFalse &&
						cond.Reason == "ClusterIdentityNotAvailable" {
						return true
					}
				}
				return false
			}, timeout, interval).Should(BeTrue())

			Expect(k8sClient.Delete(ctx, dnsPolicy)).Should(Succeed())
		})

		It("should update active controllers when cluster identity changes", func() {
			dnsPolicy := &routingv1alpha1.DNSPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-dnspolicy-identity-change",
					Namespace: "default",
				},
				Spec: routingv1alpha1.DNSPolicySpec{
					Mode: "Active",
				},
			}

			Expect(k8sClient.Create(ctx, dnsPolicy)).Should(Succeed())

			dnsPolicyLookupKey := types.NamespacedName{
				Name:      dnsPolicy.Name,
				Namespace: dnsPolicy.Namespace,
			}
			createdDNSPolicy := &routingv1alpha1.DNSPolicy{}

			// Initially should have neu controller active
			Eventually(func() []string {
				err := k8sClient.Get(ctx, dnsPolicyLookupKey, createdDNSPolicy)
				if err != nil {
					return nil
				}
				return createdDNSPolicy.Status.ActiveControllers
			}, timeout, interval).Should(ConsistOf("external-dns-neu", "external-dns-neu-1", "external-dns-neu-2"))

			// Change cluster identity region to weu by updating ClusterIdentity
			Eventually(func() error {
				updatedRegion := &clusterv1alpha1.ClusterIdentity{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: "test-cluster-identity-dnspolicy"}, updatedRegion)
				if err != nil {
					return err
				}
				updatedRegion.Spec.Region = "weu"
				updatedRegion.Spec.Cluster = "aks02"
				return k8sClient.Update(ctx, updatedRegion)
			}, timeout, interval).Should(Succeed())

			// Trigger reconciliation by updating the DNSPolicy
			Eventually(func() error {
				err := k8sClient.Get(ctx, dnsPolicyLookupKey, createdDNSPolicy)
				if err != nil {
					return err
				}
				// Add a label to trigger reconciliation
				if createdDNSPolicy.Labels == nil {
					createdDNSPolicy.Labels = make(map[string]string)
				}
				createdDNSPolicy.Labels["test"] = "trigger"
				return k8sClient.Update(ctx, createdDNSPolicy)
			}, timeout, interval).Should(Succeed())

			Eventually(func() []string {
				err := k8sClient.Get(ctx, dnsPolicyLookupKey, createdDNSPolicy)
				if err != nil {
					return nil
				}
				return createdDNSPolicy.Status.ActiveControllers
			}, timeout, interval).Should(Equal([]string{"external-dns-weu"}))

			Expect(k8sClient.Delete(ctx, dnsPolicy)).Should(Succeed())
		})

		It("should mark policy as inactive when sourceCluster doesn't match", func() {
			dnsPolicy := &routingv1alpha1.DNSPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-dnspolicy-wrong-cluster",
					Namespace: "default",
				},
				Spec: routingv1alpha1.DNSPolicySpec{
					Mode:          "RegionBound",
					SourceRegion:  "neu",   // Matches cluster region
					SourceCluster: "aks02", // Doesn't match cluster name (aks01)
				},
			}

			Expect(k8sClient.Create(ctx, dnsPolicy)).Should(Succeed())

			dnsPolicyLookupKey := types.NamespacedName{
				Name:      dnsPolicy.Name,
				Namespace: dnsPolicy.Namespace,
			}
			createdDNSPolicy := &routingv1alpha1.DNSPolicy{}

			Eventually(func() bool {
				err := k8sClient.Get(ctx, dnsPolicyLookupKey, createdDNSPolicy)
				if err != nil {
					return true
				}
				return !createdDNSPolicy.Status.Active
			}, timeout, interval).Should(BeTrue())

			Expect(k8sClient.Delete(ctx, dnsPolicy)).Should(Succeed())
		})

		It("should handle no matching controllers", func() {
			// Update DNSConfiguration to have no matching controllers for active mode
			// We can create a new DNSConfiguration or update existing one.
			// Since tests are parallel or might interfere, we should use a specific config if possible or update global.
			// But here we rely on the global one created in BeforeEach which has controllers.

			// To test no matching controllers, we can change the ClusterIdentity region to a region with no controllers
			Eventually(func() error {
				updatedRegion := &clusterv1alpha1.ClusterIdentity{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: "test-cluster-identity-dnspolicy"}, updatedRegion)
				if err != nil {
					return err
				}
				updatedRegion.Spec.Region = "unknown-region"
				return k8sClient.Update(ctx, updatedRegion)
			}, timeout, interval).Should(Succeed())

			dnsPolicy := &routingv1alpha1.DNSPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-dnspolicy-no-match",
					Namespace: "default",
				},
				Spec: routingv1alpha1.DNSPolicySpec{
					Mode: "Active",
				},
			}

			Expect(k8sClient.Create(ctx, dnsPolicy)).Should(Succeed())

			dnsPolicyLookupKey := types.NamespacedName{
				Name:      dnsPolicy.Name,
				Namespace: dnsPolicy.Namespace,
			}
			createdDNSPolicy := &routingv1alpha1.DNSPolicy{}

			Eventually(func() int {
				err := k8sClient.Get(ctx, dnsPolicyLookupKey, createdDNSPolicy)
				if err != nil {
					return -1
				}
				return len(createdDNSPolicy.Status.ActiveControllers)
			}, timeout, interval).Should(Equal(0))

			Expect(k8sClient.Delete(ctx, dnsPolicy)).Should(Succeed())
		})

		It("should activate adopted regions in Active mode", func() {
			// Update ClusterIdentity to adopt region "frc"
			Eventually(func() error {
				updatedIdentity := &clusterv1alpha1.ClusterIdentity{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: "test-cluster-identity-dnspolicy"}, updatedIdentity)
				if err != nil {
					return err
				}
				updatedIdentity.Spec.AdoptsRegions = []string{"frc"}
				return k8sClient.Update(ctx, updatedIdentity)
			}, timeout, interval).Should(Succeed())

			dnsPolicy := &routingv1alpha1.DNSPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-dnspolicy-adopted",
					Namespace: "default",
				},
				Spec: routingv1alpha1.DNSPolicySpec{
					Mode: "Active",
				},
			}

			Expect(k8sClient.Create(ctx, dnsPolicy)).Should(Succeed())

			dnsPolicyLookupKey := types.NamespacedName{
				Name:      dnsPolicy.Name,
				Namespace: dnsPolicy.Namespace,
			}
			createdDNSPolicy := &routingv1alpha1.DNSPolicy{}

			// Verify active controllers include local region (neu) AND adopted region (frc)
			// Local: external-dns-neu, external-dns-neu-1, external-dns-neu-2
			// Adopted: external-dns-frc
			Eventually(func() []string {
				err := k8sClient.Get(ctx, dnsPolicyLookupKey, createdDNSPolicy)
				if err != nil {
					return nil
				}
				return createdDNSPolicy.Status.ActiveControllers
			}, timeout, interval).Should(ConsistOf(
				"external-dns-neu",
				"external-dns-neu-1",
				"external-dns-neu-2",
				"external-dns-frc",
			))

			Expect(k8sClient.Delete(ctx, dnsPolicy)).Should(Succeed())
		})
	})
})
