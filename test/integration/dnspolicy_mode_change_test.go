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
	externaldnsv1alpha1 "sigs.k8s.io/external-dns/apis/v1alpha1"
)

var _ = Describe("DNSPolicy Mode Change Integration", func() {
	var namespace string
	var clusterIdentity *clusterv1alpha1.ClusterIdentity
	var gateway *routingv1alpha1.Gateway
	var dnsPolicy *routingv1alpha1.DNSPolicy

	BeforeEach(func() {
		timestamp := time.Now().Unix()
		namespace = fmt.Sprintf("test-dns-mode-%d", timestamp)
		CreateNamespace(namespace)

		// Create ClusterIdentity
		clusterIdentity = &clusterv1alpha1.ClusterIdentity{
			ObjectMeta: metav1.ObjectMeta{
				Name: fmt.Sprintf("test-identity-dns-%d", timestamp),
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

		// Create Gateway in istio-system namespace
		gateway = &routingv1alpha1.Gateway{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("test-gateway-dns-%d", timestamp),
				Namespace: "istio-system",
			},
			Spec: routingv1alpha1.GatewaySpec{
				Controller:     "aks-istio-ingressgateway-internal",
				CredentialName: "cert-aks-ingress",
				TargetPostfix:  "external",
			},
		}
		Expect(k8sClient.Create(ctx, gateway)).To(Succeed())

		// Create or Update DNSConfiguration
		dnsConfig := &clusterv1alpha1.DNSConfiguration{
			ObjectMeta: metav1.ObjectMeta{
				Name: "default",
			},
			Spec: clusterv1alpha1.DNSConfigurationSpec{
				ExternalDNSControllers: []clusterv1alpha1.ExternalDNSController{
					{Name: "external-dns-neu", Region: "neu"},
					{Name: "external-dns-weu", Region: "weu"},
					{Name: "external-dns-frc", Region: "frc"},
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
				Name:      "test-policy-mode",
				Namespace: namespace,
			},
			Spec: routingv1alpha1.DNSPolicySpec{
				Mode: "Active",
			},
		}
		Expect(k8sClient.Create(ctx, dnsPolicy)).To(Succeed())
		WaitForCondition(dnsPolicy, "Ready", metav1.ConditionTrue)
	})

	AfterEach(func() {
		if clusterIdentity != nil {
			DeleteObject(clusterIdentity)
		}
		if gateway != nil {
			DeleteObject(gateway)
		}
	})

	Context("When DNSPolicy mode changes from Active to RegionBound", func() {
		It("Should update DNSEndpoint controller annotations accordingly", func() {
			// Create ServiceRoute
			serviceRoute := &routingv1alpha1.ServiceRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-route-dns-mode",
					Namespace: namespace,
				},
				Spec: routingv1alpha1.ServiceRouteSpec{
					ServiceName: "my-service",
					GatewayName: gateway.Name,
					Environment: "dev",
					Application: "myapp",
				},
			}
			Expect(k8sClient.Create(ctx, serviceRoute)).To(Succeed())
			WaitForCondition(serviceRoute, "Ready", metav1.ConditionTrue)

			// In Active mode, should have only neu controller
			Eventually(func() []string {
				var policy routingv1alpha1.DNSPolicy
				if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(dnsPolicy), &policy); err != nil {
					return nil
				}
				return policy.Status.ActiveControllers
			}, timeout, interval).Should(Equal([]string{"external-dns-neu"}))

			// Verify DNSEndpoint created with neu controller
			var dnsEndpoint externaldnsv1alpha1.DNSEndpoint
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      "test-route-dns-mode-external-dns-neu",
					Namespace: namespace,
				}, &dnsEndpoint)
			}, timeout, interval).Should(Succeed())

			// Change DNSPolicy mode to RegionBound with sourceRegion matching cluster
			Eventually(func() error {
				var policy routingv1alpha1.DNSPolicy
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(dnsPolicy), &policy)
				if err != nil {
					return err
				}
				policy.Spec.Mode = "RegionBound"
				policy.Spec.SourceRegion = "neu" // Matches cluster region
				return k8sClient.Update(ctx, &policy)
			}, timeout, interval).Should(Succeed())

			// Wait for DNSPolicy to update
			WaitForCondition(dnsPolicy, "Ready", metav1.ConditionTrue)

			// Should now have ALL controllers active (RegionBound activates all controllers)
			Eventually(func() []string {
				var policy routingv1alpha1.DNSPolicy
				if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(dnsPolicy), &policy); err != nil {
					return nil
				}
				return policy.Status.ActiveControllers
			}, timeout, interval).Should(ConsistOf("external-dns-neu", "external-dns-weu", "external-dns-frc"))

			// Verify the policy is active
			Eventually(func() bool {
				var policy routingv1alpha1.DNSPolicy
				if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(dnsPolicy), &policy); err != nil {
					return false
				}
				return policy.Status.Active
			}, timeout, interval).Should(BeTrue())

			// Verify DNSEndpoint still has correct controller annotation
			Eventually(func() string {
				var de externaldnsv1alpha1.DNSEndpoint
				if err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      "test-route-dns-mode-external-dns-neu",
					Namespace: namespace,
				}, &de); err != nil {
					return ""
				}
				return de.Annotations["external-dns.alpha.kubernetes.io/controller"]
			}, timeout, interval).Should(Equal("external-dns-neu"))

			// Cleanup
			DeleteObject(serviceRoute)
		})
	})

	Context("When DNSPolicy becomes inactive (non-matching sourceRegion)", func() {
		It("Should delete DNSEndpoints to prevent race conditions", func() {
			// Create ServiceRoute with Active DNSPolicy
			serviceRoute := &routingv1alpha1.ServiceRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-route-inactive",
					Namespace: namespace,
				},
				Spec: routingv1alpha1.ServiceRouteSpec{
					ServiceName: "my-service",
					GatewayName: gateway.Name,
					Environment: "dev",
					Application: "myapp",
				},
			}
			Expect(k8sClient.Create(ctx, serviceRoute)).To(Succeed())
			WaitForCondition(serviceRoute, "Ready", metav1.ConditionTrue)

			// Verify DNSEndpoints created
			var dnsEndpoint externaldnsv1alpha1.DNSEndpoint
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      "test-route-inactive-external-dns-neu",
					Namespace: namespace,
				}, &dnsEndpoint)
			}, timeout, interval).Should(Succeed())

			// Change to RegionBound with NON-MATCHING sourceRegion
			Eventually(func() error {
				var policy routingv1alpha1.DNSPolicy
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(dnsPolicy), &policy)
				if err != nil {
					return err
				}
				policy.Spec.Mode = "RegionBound"
				policy.Spec.SourceRegion = "weu" // Cluster is in "neu"
				return k8sClient.Update(ctx, &policy)
			}, timeout, interval).Should(Succeed())

			// Verify DNSPolicy becomes inactive
			Eventually(func() bool {
				var policy routingv1alpha1.DNSPolicy
				if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(dnsPolicy), &policy); err != nil {
					return true
				}
				return policy.Status.Active
			}, timeout, interval).Should(BeFalse())

			// Verify DNSEndpoints are deleted
			Eventually(func() bool {
				var de externaldnsv1alpha1.DNSEndpoint
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      "test-route-inactive-external-dns-neu",
					Namespace: namespace,
				}, &de)
				return err != nil
			}, timeout, interval).Should(BeTrue())

			// Verify ServiceRoute status
			Eventually(func() string {
				var route routingv1alpha1.ServiceRoute
				if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(serviceRoute), &route); err != nil {
					return ""
				}
				return route.Status.Phase
			}, timeout, interval).Should(Equal("Pending"))

			// Cleanup
			DeleteObject(serviceRoute)
		})
	})
})
