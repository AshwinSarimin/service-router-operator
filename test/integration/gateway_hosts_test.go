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
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	clusterv1alpha1 "github.com/vecozo/service-router-operator/api/cluster/v1alpha1"
	routingv1alpha1 "github.com/vecozo/service-router-operator/api/routing/v1alpha1"
	istioclientv1beta1 "istio.io/client-go/pkg/apis/networking/v1beta1"
)

var _ = Describe("Gateway Hosts Management Integration", func() {
	var namespace string
	var clusterIdentity *clusterv1alpha1.ClusterIdentity
	var gateway *routingv1alpha1.Gateway
	var dnsPolicy *routingv1alpha1.DNSPolicy

	BeforeEach(func() {
		timestamp := time.Now().Unix()
		namespace = fmt.Sprintf("test-gw-hosts-%d", timestamp)
		CreateNamespace(namespace)

		// Create ClusterIdentity
		clusterIdentity = &clusterv1alpha1.ClusterIdentity{
			ObjectMeta: metav1.ObjectMeta{
				Name: fmt.Sprintf("test-identity-hosts-%d", timestamp),
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
				Name:      fmt.Sprintf("test-gateway-hosts-%d", timestamp),
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

		// Create DNSPolicy
		dnsPolicy = &routingv1alpha1.DNSPolicy{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-policy-hosts",
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

	Context("When ServiceRoutes are added", func() {
		It("Should add hosts to the Istio Gateway", func() {
			// Create first ServiceRoute
			serviceRoute1 := &routingv1alpha1.ServiceRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-route-1",
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
			WaitForCondition(gateway, "Ready", metav1.ConditionTrue)

			// Istio Gateway should now have the host for service-1
			expectedHost1 := "service-1-ns-d-dev-app1.example.com"
			istioGW := &istioclientv1beta1.Gateway{}
			Eventually(func() []string {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      gateway.Name,
					Namespace: gateway.Namespace,
				}, istioGW)
				if err != nil {
					return nil
				}
				if len(istioGW.Spec.Servers) == 0 {
					return nil
				}
				return istioGW.Spec.Servers[0].Hosts
			}, timeout, interval).Should(ContainElement(expectedHost1))

			// Create second ServiceRoute
			serviceRoute2 := &routingv1alpha1.ServiceRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-route-2",
					Namespace: namespace,
				},
				Spec: routingv1alpha1.ServiceRouteSpec{
					ServiceName: "service-2",
					GatewayName: gateway.Name,
					Environment: "prod",
					Application: "app2",
				},
			}
			Expect(k8sClient.Create(ctx, serviceRoute2)).To(Succeed())
			WaitForCondition(serviceRoute2, "Ready", metav1.ConditionTrue)

			// Istio Gateway should now have both hosts
			expectedHost2 := "service-2-ns-d-prod-app2.example.com"
			Eventually(func() []string {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      gateway.Name,
					Namespace: gateway.Namespace,
				}, istioGW)
				if err != nil {
					return nil
				}
				if len(istioGW.Spec.Servers) == 0 {
					return nil
				}
				return istioGW.Spec.Servers[0].Hosts
			}, timeout, interval).Should(ConsistOf(expectedHost1, expectedHost2))

			// Cleanup
			DeleteObject(serviceRoute1)
			DeleteObject(serviceRoute2)
		})
	})

	Context("When ServiceRoutes are removed", func() {
		It("Should remove hosts from the Istio Gateway", func() {
			// Create two ServiceRoutes
			serviceRoute1 := &routingv1alpha1.ServiceRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-route-remove-1",
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
					Name:      "test-route-remove-2",
					Namespace: namespace,
				},
				Spec: routingv1alpha1.ServiceRouteSpec{
					ServiceName: "service-2",
					GatewayName: gateway.Name,
					Environment: "prod",
					Application: "app2",
				},
			}
			Expect(k8sClient.Create(ctx, serviceRoute2)).To(Succeed())
			WaitForCondition(serviceRoute2, "Ready", metav1.ConditionTrue)

			expectedHost1 := "service-1-ns-d-dev-app1.example.com"
			expectedHost2 := "service-2-ns-d-prod-app2.example.com"

			// Verify both hosts are present
			istioGW := &istioclientv1beta1.Gateway{}
			Eventually(func() []string {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      gateway.Name,
					Namespace: gateway.Namespace,
				}, istioGW)
				if err != nil {
					return nil
				}
				if len(istioGW.Spec.Servers) == 0 {
					return nil
				}
				return istioGW.Spec.Servers[0].Hosts
			}, timeout, interval).Should(ConsistOf(expectedHost1, expectedHost2))

			// Delete first ServiceRoute
			DeleteObject(serviceRoute1)

			// Istio Gateway should now only have the second host
			Eventually(func() []string {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      gateway.Name,
					Namespace: gateway.Namespace,
				}, istioGW)
				if err != nil {
					return nil
				}
				if len(istioGW.Spec.Servers) == 0 {
					return nil
				}
				return istioGW.Spec.Servers[0].Hosts
			}, timeout, interval).Should(ConsistOf(expectedHost2))

			// Verify first host is no longer present
			Consistently(func() []string {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      gateway.Name,
					Namespace: gateway.Namespace,
				}, istioGW)
				if err != nil {
					return nil
				}
				if len(istioGW.Spec.Servers) == 0 {
					return nil
				}
				return istioGW.Spec.Servers[0].Hosts
			}, "2s", interval).ShouldNot(ContainElement(expectedHost1))

			// Cleanup
			DeleteObject(serviceRoute2)
		})
	})

	Context("When all ServiceRoutes are removed", func() {
		It("Should delete the Istio Gateway", func() {
			// Create a ServiceRoute
			serviceRoute := &routingv1alpha1.ServiceRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-route-wildcard",
					Namespace: namespace,
				},
				Spec: routingv1alpha1.ServiceRouteSpec{
					ServiceName: "service-1",
					GatewayName: gateway.Name,
					Environment: "dev",
					Application: "app1",
				},
			}
			Expect(k8sClient.Create(ctx, serviceRoute)).To(Succeed())
			WaitForCondition(serviceRoute, "Ready", metav1.ConditionTrue)

			expectedHost := "service-1-ns-d-dev-app1.example.com"

			// Verify specific host is present
			istioGW := &istioclientv1beta1.Gateway{}
			Eventually(func() []string {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      gateway.Name,
					Namespace: gateway.Namespace,
				}, istioGW)
				if err != nil {
					return nil
				}
				if len(istioGW.Spec.Servers) == 0 {
					return nil
				}
				return istioGW.Spec.Servers[0].Hosts
			}, timeout, interval).Should(ContainElement(expectedHost))

			// Delete the ServiceRoute
			DeleteObject(serviceRoute)

			// Istio Gateway should be deleted
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      gateway.Name,
					Namespace: gateway.Namespace,
				}, istioGW)
				return errors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())

			// Gateway should be in Pending state
			WaitForCondition(gateway, "Ready", metav1.ConditionFalse)
			Eventually(func() string {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      gateway.Name,
					Namespace: gateway.Namespace,
				}, gateway)
				if err != nil {
					return ""
				}
				return gateway.Status.Phase
			}, timeout, interval).Should(Equal("Pending"))
		})
	})
})
