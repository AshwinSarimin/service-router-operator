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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	clusterv1alpha1 "github.com/vecozo/service-router-operator/api/cluster/v1alpha1"
	"github.com/vecozo/service-router-operator/internal/dnsconfiguration"
)

var _ = Describe("DNSConfiguration Controller", func() {
	const (
		DNSConfigName = "default-dns-config"
		timeout       = time.Second * 10
		interval      = time.Millisecond * 250
	)

	Context("When reconciling a DNSConfiguration", func() {
		BeforeEach(func() {
			dnsconfiguration.Clear()
		})

		AfterEach(func() {
			dnsConfig := &clusterv1alpha1.DNSConfiguration{}
			if err := k8sClient.Get(context.Background(), types.NamespacedName{Name: DNSConfigName}, dnsConfig); err == nil {
				Expect(k8sClient.Delete(context.Background(), dnsConfig)).To(Succeed())
			}
			dnsconfiguration.Clear()
		})

		It("Should update the cache and status when valid", func() {
			ctx := context.Background()

			dnsConfig := &clusterv1alpha1.DNSConfiguration{
				ObjectMeta: metav1.ObjectMeta{
					Name: DNSConfigName,
				},
				Spec: clusterv1alpha1.DNSConfigurationSpec{
					ExternalDNSControllers: []clusterv1alpha1.ExternalDNSController{
						{
							Name:   "external-dns-weu",
							Region: "weu",
						},
					},
				},
			}

			Expect(k8sClient.Create(ctx, dnsConfig)).To(Succeed())

			// Verify Cache
			Eventually(func() *dnsconfiguration.DNSConfiguration {
				return dnsconfiguration.Get()
			}, timeout, interval).ShouldNot(BeNil())

			cached := dnsconfiguration.Get()
			Expect(len(cached.ExternalDNSControllers)).To(Equal(1))
			Expect(cached.ExternalDNSControllers[0].Name).To(Equal("external-dns-weu"))
			Expect(cached.ExternalDNSControllers[0].Region).To(Equal("weu"))

			Eventually(func() string {
				var current clusterv1alpha1.DNSConfiguration
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: DNSConfigName}, &current); err != nil {
					return ""
				}
				if len(current.Status.Conditions) == 0 {
					return ""
				}
				return current.Status.Conditions[0].Reason
			}, timeout, interval).Should(Equal("ReconciliationSucceeded"))
		})

		It("Should clear cache when deleted", func() {
			ctx := context.Background()

			dnsConfig := &clusterv1alpha1.DNSConfiguration{
				ObjectMeta: metav1.ObjectMeta{
					Name: DNSConfigName,
				},
				Spec: clusterv1alpha1.DNSConfigurationSpec{
					ExternalDNSControllers: []clusterv1alpha1.ExternalDNSController{
						{Name: "test", Region: "test"},
					},
				},
			}

			Expect(k8sClient.Create(ctx, dnsConfig)).To(Succeed())
			Eventually(func() *dnsconfiguration.DNSConfiguration {
				return dnsconfiguration.Get()
			}, timeout, interval).ShouldNot(BeNil())

			Expect(k8sClient.Delete(ctx, dnsConfig)).To(Succeed())

			Eventually(func() *dnsconfiguration.DNSConfiguration {
				return dnsconfiguration.Get()
			}, timeout, interval).Should(BeNil())
		})
	})
})
