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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	clusterv1alpha1 "github.com/vecozo/service-router-operator/api/cluster/v1alpha1"
	"github.com/vecozo/service-router-operator/internal/clusteridentity"
)

var _ = Describe("ClusterIdentity Controller", func() {
	const (
		timeout  = time.Second * 10
		interval = time.Millisecond * 250
	)

	Context("When reconciling a ClusterIdentity", func() {
		ctx := context.Background()

		It("Should create ClusterIdentity successfully and update cache", func() {
			clusterIdentity := &clusterv1alpha1.ClusterIdentity{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-cluster-identity",
				},
				Spec: clusterv1alpha1.ClusterIdentitySpec{
					Region:            "neu",
					Cluster:           "aks01",
					Domain:            "example.com",
					EnvironmentLetter: "d",
				},
			}

			Expect(k8sClient.Create(ctx, clusterIdentity)).Should(Succeed())

			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: clusterIdentity.Name}, clusterIdentity)
				return err == nil && clusterIdentity.Status.Phase == "Active"
			}, timeout, interval).Should(BeTrue())

			identity := clusteridentity.Get()
			Expect(identity).NotTo(BeNil())
			Expect(identity.Region).To(Equal("neu"))
			Expect(identity.Cluster).To(Equal("aks01"))
			Expect(identity.Domain).To(Equal("example.com"))
			Expect(identity.EnvironmentLetter).To(Equal("d"))

			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: clusterIdentity.Name}, clusterIdentity)
				if err != nil {
					return false
				}
				for _, cond := range clusterIdentity.Status.Conditions {
					if cond.Type == "Ready" && cond.Status == metav1.ConditionTrue {
						return true
					}
				}
				return false
			}, timeout, interval).Should(BeTrue())

			Expect(k8sClient.Delete(ctx, clusterIdentity)).Should(Succeed())

			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: clusterIdentity.Name}, clusterIdentity)
				return errors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())

			Eventually(func() *clusteridentity.ClusterIdentity {
				return clusteridentity.Get()
			}, timeout, interval).Should(BeNil())
		})

		It("Should reject multiple ClusterIdentity resources", func() {
			clusterRegion1 := &clusterv1alpha1.ClusterIdentity{
				ObjectMeta: metav1.ObjectMeta{
					Name: "cluster-identity-1",
				},
				Spec: clusterv1alpha1.ClusterIdentitySpec{
					Region:            "neu",
					Cluster:           "aks01",
					Domain:            "example.com",
					EnvironmentLetter: "d",
				},
			}

			clusterRegion2 := &clusterv1alpha1.ClusterIdentity{
				ObjectMeta: metav1.ObjectMeta{
					Name: "cluster-identity-2",
				},
				Spec: clusterv1alpha1.ClusterIdentitySpec{
					Region:            "weu",
					Cluster:           "aks02",
					Domain:            "example.com",
					EnvironmentLetter: "t",
				},
			}

			Expect(k8sClient.Create(ctx, clusterRegion1)).Should(Succeed())

			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: clusterRegion1.Name}, clusterRegion1)
				return err == nil && clusterRegion1.Status.Phase == "Active"
			}, timeout, interval).Should(BeTrue())

			Expect(k8sClient.Create(ctx, clusterRegion2)).Should(Succeed())

			// Both should fail singleton validation
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: clusterRegion2.Name}, clusterRegion2)
				return err == nil && clusterRegion2.Status.Phase == "Failed"
			}, timeout, interval).Should(BeTrue())

			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: clusterRegion2.Name}, clusterRegion2)
				if err != nil {
					return false
				}
				for _, cond := range clusterRegion2.Status.Conditions {
					if cond.Type == "Ready" && cond.Status == metav1.ConditionFalse && cond.Reason == "SingletonViolation" {
						return true
					}
				}
				return false
			}, timeout, interval).Should(BeTrue())

			Expect(k8sClient.Delete(ctx, clusterRegion1)).Should(Succeed())
			Expect(k8sClient.Delete(ctx, clusterRegion2)).Should(Succeed())

			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: clusterRegion1.Name}, clusterRegion1)
				return errors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())

			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: clusterRegion2.Name}, clusterRegion2)
				return errors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())
		})

		It("Should update cache when ClusterIdentity spec changes", func() {
			clusterIdentity := &clusterv1alpha1.ClusterIdentity{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-cluster-identity-update",
				},
				Spec: clusterv1alpha1.ClusterIdentitySpec{
					Region:            "neu",
					Cluster:           "aks01",
					Domain:            "example.com",
					EnvironmentLetter: "d",
				},
			}

			Expect(k8sClient.Create(ctx, clusterIdentity)).Should(Succeed())

			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: clusterIdentity.Name}, clusterIdentity)
				return err == nil && clusterIdentity.Status.Phase == "Active"
			}, timeout, interval).Should(BeTrue())

			clusterIdentity.Spec.Region = "weu"
			Expect(k8sClient.Update(ctx, clusterIdentity)).Should(Succeed())

			Eventually(func() string {
				identity := clusteridentity.Get()
				if identity == nil {
					return ""
				}
				return identity.Region
			}, timeout, interval).Should(Equal("weu"))

			Expect(k8sClient.Delete(ctx, clusterIdentity)).Should(Succeed())

			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: clusterIdentity.Name}, clusterIdentity)
				return errors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())
		})

		It("Should report invalid adopted regions in status", func() {
			// Setup: Create DNSConfiguration
			dnsConfig := &clusterv1alpha1.DNSConfiguration{
				ObjectMeta: metav1.ObjectMeta{
					Name: "default",
				},
				Spec: clusterv1alpha1.DNSConfigurationSpec{
					ExternalDNSControllers: []clusterv1alpha1.ExternalDNSController{
						{Name: "external-dns-neu", Region: "neu"},
					},
				},
			}
			Expect(k8sClient.Create(ctx, dnsConfig)).Should(Succeed())

			// Setup: Create ClusterIdentity
			clusterIdentity := &clusterv1alpha1.ClusterIdentity{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-cluster-identity-adopted",
				},
				Spec: clusterv1alpha1.ClusterIdentitySpec{
					Region:            "neu",
					Cluster:           "aks01",
					Domain:            "example.com",
					EnvironmentLetter: "d",
				},
			}
			Expect(k8sClient.Create(ctx, clusterIdentity)).Should(Succeed())

			// Wait for Active
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: clusterIdentity.Name}, clusterIdentity)
				return err == nil && clusterIdentity.Status.Phase == "Active"
			}, timeout, interval).Should(BeTrue())

			// Update to adopt invalid region
			Eventually(func() error {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: clusterIdentity.Name}, clusterIdentity)
				if err != nil {
					return err
				}
				clusterIdentity.Spec.AdoptsRegions = []string{"invalid-region"}
				return k8sClient.Update(ctx, clusterIdentity)
			}, timeout, interval).Should(Succeed())

			Eventually(func() bool {
				ci := &clusterv1alpha1.ClusterIdentity{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: clusterIdentity.Name}, ci); err != nil {
					return false
				}
				cond := meta.FindStatusCondition(ci.Status.Conditions, "AdoptedRegionsValid")
				return cond != nil && cond.Status == metav1.ConditionFalse && cond.Reason == "AdoptedRegionNotFound"
			}, timeout, interval).Should(BeTrue())

			// Cleanup
			Expect(k8sClient.Delete(ctx, clusterIdentity)).Should(Succeed())
			Expect(k8sClient.Delete(ctx, dnsConfig)).Should(Succeed())

			Eventually(func() bool {
				return errors.IsNotFound(k8sClient.Get(ctx, types.NamespacedName{Name: clusterIdentity.Name}, clusterIdentity))
			}, timeout, interval).Should(BeTrue())
			Eventually(func() bool {
				return errors.IsNotFound(k8sClient.Get(ctx, types.NamespacedName{Name: dnsConfig.Name}, dnsConfig))
			}, timeout, interval).Should(BeTrue())
		})
	})
})
