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
	"github.com/vecozo/service-router-operator/pkg/consts"
	externaldnsv1alpha1 "sigs.k8s.io/external-dns/apis/v1alpha1"
)

var _ = Describe("DNSPolicy Inactive Cleanup Integration", func() {
	var namespace string
	var clusterIdentity *clusterv1alpha1.ClusterIdentity
	var gateway *routingv1alpha1.Gateway
	var dnsPolicy *routingv1alpha1.DNSPolicy

	BeforeEach(func() {
		timestamp := time.Now().Unix()
		namespace = fmt.Sprintf("test-inactive-%d", timestamp)
		CreateNamespace(namespace)

		// Create ClusterIdentity
		clusterIdentity = &clusterv1alpha1.ClusterIdentity{
			ObjectMeta: metav1.ObjectMeta{
				Name: fmt.Sprintf("test-identity-inactive-%d", timestamp),
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
				Name:      fmt.Sprintf("test-gateway-inactive-%d", timestamp),
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
	})

	AfterEach(func() {
		if clusterIdentity != nil {
			DeleteObject(clusterIdentity)
		}
		if gateway != nil {
			DeleteObject(gateway)
		}
	})

	Context("When DNSPolicy becomes inactive after being active", func() {
		It("Should delete DNSEndpoints to prevent race conditions", func() {
			// Create DNSPolicy in Active mode
			dnsPolicy = &routingv1alpha1.DNSPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-policy-cleanup",
					Namespace: namespace,
				},
				Spec: routingv1alpha1.DNSPolicySpec{
					Mode: "Active",
				},
			}
			Expect(k8sClient.Create(ctx, dnsPolicy)).To(Succeed())
			WaitForCondition(dnsPolicy, "Ready", metav1.ConditionTrue)

			// Create ServiceRoute
			serviceRoute := &routingv1alpha1.ServiceRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-route-cleanup",
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

			// Verify DNSEndpoint was created
			var dnsEndpoint externaldnsv1alpha1.DNSEndpoint
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      "test-route-cleanup-external-dns-neu",
					Namespace: namespace,
				}, &dnsEndpoint)
			}, timeout, interval).Should(Succeed())

			// Change DNSPolicy to inactive (RegionBound with non-matching sourceRegion)
			Eventually(func() error {
				var policy routingv1alpha1.DNSPolicy
				if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(dnsPolicy), &policy); err != nil {
					return err
				}
				policy.Spec.Mode = "RegionBound"
				policy.Spec.SourceRegion = "weu" // Does NOT match cluster region "neu"
				return k8sClient.Update(ctx, &policy)
			}, timeout, interval).Should(Succeed())

			// Wait for DNSPolicy to become inactive
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
					Name:      "test-route-cleanup-external-dns-neu",
					Namespace: namespace,
				}, &de)
				return err != nil
			}, timeout, interval).Should(BeTrue())

			// Verify ServiceRoute status shows Pending with correct reason
			Eventually(func() string {
				var route routingv1alpha1.ServiceRoute
				if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(serviceRoute), &route); err != nil {
					return ""
				}
				return route.Status.Phase
			}, timeout, interval).Should(Equal(consts.PhasePending))

			Eventually(func() string {
				var route routingv1alpha1.ServiceRoute
				if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(serviceRoute), &route); err != nil {
					return ""
				}
				if len(route.Status.Conditions) == 0 {
					return ""
				}
				return route.Status.Conditions[0].Reason
			}, timeout, interval).Should(Equal(consts.ReasonDNSPolicyInactive))

			// Cleanup
			DeleteObject(serviceRoute)
			DeleteObject(dnsPolicy)
		})
	})

	Context("When DNSPolicy becomes active again", func() {
		It("Should recreate DNSEndpoints", func() {
			// Create DNSPolicy in inactive state
			dnsPolicy = &routingv1alpha1.DNSPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-policy-reactivate",
					Namespace: namespace,
				},
				Spec: routingv1alpha1.DNSPolicySpec{
					Mode:         "RegionBound",
					SourceRegion: "weu", // Does NOT match cluster region "neu"
				},
			}
			Expect(k8sClient.Create(ctx, dnsPolicy)).To(Succeed())

			// Wait for DNSPolicy to be inactive
			Eventually(func() bool {
				var policy routingv1alpha1.DNSPolicy
				if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(dnsPolicy), &policy); err != nil {
					return true
				}
				return policy.Status.Active
			}, timeout, interval).Should(BeFalse())

			// Create ServiceRoute while policy is inactive
			serviceRoute := &routingv1alpha1.ServiceRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-route-reactivate",
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

			// Verify ServiceRoute is Pending
			Eventually(func() string {
				var route routingv1alpha1.ServiceRoute
				if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(serviceRoute), &route); err != nil {
					return ""
				}
				return route.Status.Phase
			}, timeout, interval).Should(Equal(consts.PhasePending))

			// Verify no DNSEndpoints created
			Consistently(func() bool {
				var list externaldnsv1alpha1.DNSEndpointList
				if err := k8sClient.List(ctx, &list, client.InNamespace(namespace)); err != nil {
					return false
				}
				return len(list.Items) == 0
			}, "3s", interval).Should(BeTrue())

			// Reactivate DNSPolicy by changing sourceRegion to match cluster
			Eventually(func() error {
				var policy routingv1alpha1.DNSPolicy
				if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(dnsPolicy), &policy); err != nil {
					return err
				}
				policy.Spec.SourceRegion = "neu" // Matches cluster region
				return k8sClient.Update(ctx, &policy)
			}, timeout, interval).Should(Succeed())

			// Wait for DNSPolicy to become active
			Eventually(func() bool {
				var policy routingv1alpha1.DNSPolicy
				if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(dnsPolicy), &policy); err != nil {
					return false
				}
				return policy.Status.Active
			}, timeout, interval).Should(BeTrue())

			// Verify DNSEndpoints are now created
			Eventually(func() bool {
				var list externaldnsv1alpha1.DNSEndpointList
				if err := k8sClient.List(ctx, &list,
					client.InNamespace(namespace),
					client.MatchingLabels{
						"app.kubernetes.io/managed-by": "service-router-operator",
						"router.io/serviceroute":       "test-route-reactivate",
					}); err != nil {
					return false
				}
				return len(list.Items) > 0
			}, timeout, interval).Should(BeTrue())

			// Verify ServiceRoute status is Active
			Eventually(func() string {
				var route routingv1alpha1.ServiceRoute
				if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(serviceRoute), &route); err != nil {
					return ""
				}
				return route.Status.Phase
			}, timeout, interval).Should(Equal(consts.PhaseActive))

			// Cleanup
			DeleteObject(serviceRoute)
			DeleteObject(dnsPolicy)
		})
	})

	Context("When multiple ServiceRoutes exist in namespace", func() {
		It("Should delete DNSEndpoints for all ServiceRoutes when DNSPolicy becomes inactive", func() {
			// Create DNSPolicy in Active mode
			dnsPolicy = &routingv1alpha1.DNSPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-policy-multiple",
					Namespace: namespace,
				},
				Spec: routingv1alpha1.DNSPolicySpec{
					Mode: "Active",
				},
			}
			Expect(k8sClient.Create(ctx, dnsPolicy)).To(Succeed())
			WaitForCondition(dnsPolicy, "Ready", metav1.ConditionTrue)

			// Create multiple ServiceRoutes
			serviceRoute1 := &routingv1alpha1.ServiceRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-route-multi-1",
					Namespace: namespace,
				},
				Spec: routingv1alpha1.ServiceRouteSpec{
					ServiceName: "service-1",
					GatewayName: gateway.Name,
					Environment: "dev",
					Application: "app1",
				},
			}
			Expect(k8sClient.Create(ctx, serviceRoute1)).To(Succeed())
			WaitForCondition(serviceRoute1, "Ready", metav1.ConditionTrue)

			serviceRoute2 := &routingv1alpha1.ServiceRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-route-multi-2",
					Namespace: namespace,
				},
				Spec: routingv1alpha1.ServiceRouteSpec{
					ServiceName: "service-2",
					GatewayName: gateway.Name,
					Environment: "dev",
					Application: "app2",
				},
			}
			Expect(k8sClient.Create(ctx, serviceRoute2)).To(Succeed())
			WaitForCondition(serviceRoute2, "Ready", metav1.ConditionTrue)

			// Verify both have DNSEndpoints
			Eventually(func() bool {
				var list externaldnsv1alpha1.DNSEndpointList
				if err := k8sClient.List(ctx, &list, client.InNamespace(namespace)); err != nil {
					return false
				}
				return len(list.Items) >= 2
			}, timeout, interval).Should(BeTrue())

			// Make DNSPolicy inactive
			Eventually(func() error {
				var policy routingv1alpha1.DNSPolicy
				if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(dnsPolicy), &policy); err != nil {
					return err
				}
				policy.Spec.Mode = "RegionBound"
				policy.Spec.SourceRegion = "weu"
				return k8sClient.Update(ctx, &policy)
			}, timeout, interval).Should(Succeed())

			// Verify all DNSEndpoints are deleted
			Eventually(func() int {
				var list externaldnsv1alpha1.DNSEndpointList
				if err := k8sClient.List(ctx, &list, client.InNamespace(namespace)); err != nil {
					return -1
				}
				return len(list.Items)
			}, timeout, interval).Should(Equal(0))

			// Verify both ServiceRoutes are Pending
			Eventually(func() bool {
				var route1 routingv1alpha1.ServiceRoute
				var route2 routingv1alpha1.ServiceRoute
				if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(serviceRoute1), &route1); err != nil {
					return false
				}
				if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(serviceRoute2), &route2); err != nil {
					return false
				}
				return route1.Status.Phase == consts.PhasePending && route2.Status.Phase == consts.PhasePending
			}, timeout, interval).Should(BeTrue())

			// Cleanup
			DeleteObject(serviceRoute1)
			DeleteObject(serviceRoute2)
			DeleteObject(dnsPolicy)
		})
	})

	Context("When DNSPolicy is already inactive (idempotency test)", func() {
		It("Should handle multiple reconciliations gracefully", func() {
			// Create DNSPolicy in inactive state from the start
			dnsPolicy = &routingv1alpha1.DNSPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-policy-idempotent",
					Namespace: namespace,
				},
				Spec: routingv1alpha1.DNSPolicySpec{
					Mode:         "RegionBound",
					SourceRegion: "weu", // Does NOT match cluster region "neu"
				},
			}
			Expect(k8sClient.Create(ctx, dnsPolicy)).To(Succeed())

			// Create ServiceRoute with DNSPolicy already inactive
			serviceRoute := &routingv1alpha1.ServiceRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-route-idempotent",
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

			// Verify ServiceRoute reaches Pending state (no errors)
			Eventually(func() string {
				var route routingv1alpha1.ServiceRoute
				if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(serviceRoute), &route); err != nil {
					return ""
				}
				return route.Status.Phase
			}, timeout, interval).Should(Equal(consts.PhasePending))

			// Verify no DNSEndpoints are created
			Consistently(func() int {
				var list externaldnsv1alpha1.DNSEndpointList
				if err := k8sClient.List(ctx, &list, client.InNamespace(namespace)); err != nil {
					return -1
				}
				return len(list.Items)
			}, "3s", interval).Should(Equal(0))

			// Trigger another reconciliation by updating ServiceRoute
			Eventually(func() error {
				var route routingv1alpha1.ServiceRoute
				if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(serviceRoute), &route); err != nil {
					return err
				}
				if route.Annotations == nil {
					route.Annotations = make(map[string]string)
				}
				route.Annotations["test"] = "trigger-reconcile"
				return k8sClient.Update(ctx, &route)
			}, timeout, interval).Should(Succeed())

			// Verify ServiceRoute stays Pending (no errors even though DNSEndpoints already don't exist)
			Consistently(func() string {
				var route routingv1alpha1.ServiceRoute
				if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(serviceRoute), &route); err != nil {
					return ""
				}
				return route.Status.Phase
			}, "3s", interval).Should(Equal(consts.PhasePending))

			// Cleanup
			DeleteObject(serviceRoute)
			DeleteObject(dnsPolicy)
		})
	})
})
