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
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	externaldnsv1alpha1 "sigs.k8s.io/external-dns/apis/v1alpha1"

	clusterv1alpha1 "github.com/vecozo/service-router-operator/api/cluster/v1alpha1"
	routingv1alpha1 "github.com/vecozo/service-router-operator/api/routing/v1alpha1"
)

var _ = Describe("ServiceRoute Controller", func() {
	const (
		timeout  = time.Second * 10
		interval = time.Millisecond * 250
	)

	var (
		testNamespace   = "test-serviceroute"
		clusterIdentity *clusterv1alpha1.ClusterIdentity
		gateway         *routingv1alpha1.Gateway
		dnsPolicy       *routingv1alpha1.DNSPolicy
		dnsConfig       *clusterv1alpha1.DNSConfiguration
		ctx             context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()

		// Use unique names to avoid conflicts
		timestamp := time.Now().UnixNano()
		testNamespace = fmt.Sprintf("test-sr-%d", timestamp)

		// Create namespace
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: testNamespace,
			},
		}
		Expect(k8sClient.Create(ctx, ns)).Should(Succeed())

		// Create ClusterIdentity
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
		Expect(k8sClient.Create(ctx, clusterIdentity)).Should(Succeed())

		// Wait for ClusterIdentity to be Active
		Eventually(func() string {
			var cr clusterv1alpha1.ClusterIdentity
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: clusterIdentity.Name}, &cr); err != nil {
				return ""
			}
			return cr.Status.Phase
		}, timeout, interval).Should(Equal("Active"))

		// Create DNSConfiguration
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
		// Create or Update DNSConfiguration (it is cluster scoped singleton)
		// Since other tests might have created it, we handle AlreadyExists
		if err := k8sClient.Create(ctx, dnsConfig); err != nil {
			if apierrors.IsAlreadyExists(err) {
				existingConfig := &clusterv1alpha1.DNSConfiguration{}
				Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "default"}, existingConfig)).Should(Succeed())
				existingConfig.Spec = dnsConfig.Spec
				Expect(k8sClient.Update(ctx, existingConfig)).Should(Succeed())
			} else {
				Expect(err).Should(Succeed())
			}
		}

		// Create Gateway
		gateway = &routingv1alpha1.Gateway{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("test-gateway-%d", timestamp),
				Namespace: "istio-system",
			},
			Spec: routingv1alpha1.GatewaySpec{
				Controller:     "aks-istio-ingressgateway-internal",
				CredentialName: "cert-aks-ingress",
				TargetPostfix:  "external",
			},
		}
		Expect(k8sClient.Create(ctx, gateway)).Should(Succeed())

		// Wait for Gateway to be Pending (no ServiceRoutes yet)
		Eventually(func() string {
			var gw routingv1alpha1.Gateway
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: gateway.Name, Namespace: gateway.Namespace}, &gw); err != nil {
				return ""
			}
			return gw.Status.Phase
		}, timeout, interval).Should(Equal("Pending"))

		// Create DNSPolicy
		dnsPolicy = &routingv1alpha1.DNSPolicy{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("test-dnspolicy-%d", timestamp),
				Namespace: testNamespace,
			},
			Spec: routingv1alpha1.DNSPolicySpec{
				Mode: "Active",
			},
		}
		Expect(k8sClient.Create(ctx, dnsPolicy)).Should(Succeed())

		// Wait for DNSPolicy to have active controllers
		Eventually(func() []string {
			var dp routingv1alpha1.DNSPolicy
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: dnsPolicy.Name, Namespace: dnsPolicy.Namespace}, &dp); err != nil {
				return nil
			}
			return dp.Status.ActiveControllers
		}, timeout, interval).Should(HaveLen(1))
	})

	AfterEach(func() {
		// Cleanup resources
		if dnsPolicy != nil {
			err := k8sClient.Delete(ctx, dnsPolicy)
			Expect(err).To(Succeed())
		}
		if gateway != nil {
			err := k8sClient.Delete(ctx, gateway)
			Expect(err).To(Succeed())
		}
		if clusterIdentity != nil {
			err := k8sClient.Delete(ctx, clusterIdentity)
			Expect(err).To(Succeed())
		}
		if dnsConfig != nil {
			_ = k8sClient.Delete(ctx, dnsConfig)
		}
	})

	Context("When creating a ServiceRoute", func() {
		It("should generate DNSEndpoint resources", func() {
			serviceRoute := &routingv1alpha1.ServiceRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-serviceroute-dns",
					Namespace: testNamespace,
				},
				Spec: routingv1alpha1.ServiceRouteSpec{
					ServiceName:      "my-service",
					GatewayName:      gateway.Name,
					GatewayNamespace: gateway.Namespace,
					Environment:      "dev",
					Application:      "myapp",
				},
			}

			Expect(k8sClient.Create(ctx, serviceRoute)).Should(Succeed())

			var dnsEndpoints externaldnsv1alpha1.DNSEndpointList
			Eventually(func() int {
				if err := k8sClient.List(ctx, &dnsEndpoints, client.InNamespace(testNamespace)); err != nil {
					return 0
				}
				return len(dnsEndpoints.Items)
			}, timeout, interval).Should(BeNumerically(">", 0))

			Expect(dnsEndpoints.Items[0].OwnerReferences).Should(HaveLen(1))
			Expect(dnsEndpoints.Items[0].OwnerReferences[0].Name).Should(Equal(serviceRoute.Name))

			Expect(k8sClient.Delete(ctx, serviceRoute)).Should(Succeed())
		})

		It("should use CNAME record pointing to target host", func() {
			serviceRoute := &routingv1alpha1.ServiceRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-serviceroute-cname",
					Namespace: testNamespace,
				},
				Spec: routingv1alpha1.ServiceRouteSpec{
					ServiceName:      "my-service",
					GatewayName:      gateway.Name,
					GatewayNamespace: gateway.Namespace,
					Environment:      "dev",
					Application:      "myapp",
				},
			}

			Expect(k8sClient.Create(ctx, serviceRoute)).Should(Succeed())

			var dnsEndpoints externaldnsv1alpha1.DNSEndpointList
			Eventually(func() bool {
				if err := k8sClient.List(ctx, &dnsEndpoints, client.InNamespace(testNamespace)); err != nil {
					return false
				}
				if len(dnsEndpoints.Items) == 0 {
					return false
				}
				for _, ep := range dnsEndpoints.Items[0].Spec.Endpoints {
					if ep.RecordType == "CNAME" && len(ep.Targets) > 0 {
						// Verify target host format: {cluster}-{region}-{targetPostfix}.{domain}
						expectedTarget := "aks01-neu-external.example.com"
						if ep.Targets[0] == expectedTarget {
							return true
						}
					}
				}
				return false
			}, timeout, interval).Should(BeTrue())

			Expect(k8sClient.Delete(ctx, serviceRoute)).Should(Succeed())
		})

		It("should construct target host correctly from Gateway and ClusterIdentity", func() {
			serviceRoute := &routingv1alpha1.ServiceRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-serviceroute-target",
					Namespace: testNamespace,
				},
				Spec: routingv1alpha1.ServiceRouteSpec{
					ServiceName:      "my-service",
					GatewayName:      gateway.Name,
					GatewayNamespace: gateway.Namespace,
					Environment:      "dev",
					Application:      "myapp",
				},
			}

			Expect(k8sClient.Create(ctx, serviceRoute)).Should(Succeed())

			var dnsEndpoints externaldnsv1alpha1.DNSEndpointList
			Eventually(func() string {
				if err := k8sClient.List(ctx, &dnsEndpoints, client.InNamespace(testNamespace)); err != nil {
					return ""
				}
				if len(dnsEndpoints.Items) == 0 {
					return ""
				}
				for _, ep := range dnsEndpoints.Items[0].Spec.Endpoints {
					if ep.RecordType == "CNAME" && len(ep.Targets) > 0 {
						return ep.Targets[0]
					}
				}
				return ""
			}, timeout, interval).Should(Equal("aks01-neu-external.example.com"))

			Expect(k8sClient.Delete(ctx, serviceRoute)).Should(Succeed())
		})

		It("should construct source host correctly from ServiceRoute fields", func() {
			serviceRoute := &routingv1alpha1.ServiceRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-serviceroute-source",
					Namespace: testNamespace,
				},
				Spec: routingv1alpha1.ServiceRouteSpec{
					ServiceName:      "my-service",
					GatewayName:      gateway.Name,
					GatewayNamespace: gateway.Namespace,
					Environment:      "dev",
					Application:      "myapp",
				},
			}

			Expect(k8sClient.Create(ctx, serviceRoute)).Should(Succeed())

			// Verify source host construction: {serviceName}-ns-{envLetter}-{environment}-{application}.{domain}
			var dnsEndpoints externaldnsv1alpha1.DNSEndpointList
			Eventually(func() string {
				if err := k8sClient.List(ctx, &dnsEndpoints, client.InNamespace(testNamespace)); err != nil {
					return ""
				}
				if len(dnsEndpoints.Items) == 0 {
					return ""
				}
				for _, ep := range dnsEndpoints.Items[0].Spec.Endpoints {
					if ep.RecordType == "CNAME" {
						return ep.DNSName
					}
				}
				return ""
			}, timeout, interval).Should(Equal("my-service-ns-d-dev-myapp.example.com"))

			Expect(k8sClient.Delete(ctx, serviceRoute)).Should(Succeed())
		})

		It("should correctly map source host to target host in CNAME record", func() {
			serviceRoute := &routingv1alpha1.ServiceRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-serviceroute-mapping",
					Namespace: testNamespace,
				},
				Spec: routingv1alpha1.ServiceRouteSpec{
					ServiceName:      "api-service",
					GatewayName:      gateway.Name,
					GatewayNamespace: gateway.Namespace,
					Environment:      "prod",
					Application:      "backend",
				},
			}

			Expect(k8sClient.Create(ctx, serviceRoute)).Should(Succeed())

			var dnsEndpoints externaldnsv1alpha1.DNSEndpointList
			Eventually(func() bool {
				if err := k8sClient.List(ctx, &dnsEndpoints, client.InNamespace(testNamespace)); err != nil {
					return false
				}
				if len(dnsEndpoints.Items) == 0 {
					return false
				}
				for _, ep := range dnsEndpoints.Items[0].Spec.Endpoints {
					if ep.RecordType == "CNAME" {
						expectedSource := "api-service-ns-d-prod-backend.example.com"
						expectedTarget := "aks01-neu-external.example.com"
						return ep.DNSName == expectedSource && len(ep.Targets) > 0 && ep.Targets[0] == expectedTarget
					}
				}
				return false
			}, timeout, interval).Should(BeTrue())

			Expect(k8sClient.Delete(ctx, serviceRoute)).Should(Succeed())
		})

		It("should update status correctly", func() {
			serviceRoute := &routingv1alpha1.ServiceRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-serviceroute-status",
					Namespace: testNamespace,
				},
				Spec: routingv1alpha1.ServiceRouteSpec{
					ServiceName:      "my-service",
					GatewayName:      gateway.Name,
					GatewayNamespace: gateway.Namespace,
					Environment:      "dev",
					Application:      "myapp",
				},
			}

			Expect(k8sClient.Create(ctx, serviceRoute)).Should(Succeed())

			Eventually(func() metav1.ConditionStatus {
				var sr routingv1alpha1.ServiceRoute
				if err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      serviceRoute.Name,
					Namespace: serviceRoute.Namespace,
				}, &sr); err != nil {
					return metav1.ConditionUnknown
				}
				for _, cond := range sr.Status.Conditions {
					if cond.Type == "Ready" {
						return cond.Status
					}
				}
				return metav1.ConditionUnknown
			}, timeout, interval).Should(Equal(metav1.ConditionTrue))

			var sr routingv1alpha1.ServiceRoute
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      serviceRoute.Name,
				Namespace: serviceRoute.Namespace,
			}, &sr)).Should(Succeed())
			Expect(sr.Status.DNSEndpoint).ShouldNot(BeEmpty())

			Expect(k8sClient.Delete(ctx, serviceRoute)).Should(Succeed())
		})
	})

	Context("When validating ServiceRoute", func() {
		It("should catch missing gatewayName", func() {
			serviceRoute := &routingv1alpha1.ServiceRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-serviceroute-no-gateway",
					Namespace: testNamespace,
				},
				Spec: routingv1alpha1.ServiceRouteSpec{
					ServiceName: "my-service",
					GatewayName: "", // Missing
					Environment: "dev",
					Application: "myapp",
				},
			}

			// Creation is rejected by CRD validation (spec.gatewayName minLength 1)
			err := k8sClient.Create(ctx, serviceRoute)
			Expect(err).Should(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("spec.gatewayName"))
		})

		It("should catch missing serviceName", func() {
			serviceRoute := &routingv1alpha1.ServiceRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-serviceroute-no-service",
					Namespace: testNamespace,
				},
				Spec: routingv1alpha1.ServiceRouteSpec{
					ServiceName:      "", // Missing
					GatewayName:      gateway.Name,
					GatewayNamespace: gateway.Namespace,
					Environment:      "dev",
					Application:      "myapp",
				},
			}

			// Creation is rejected by CRD validation (spec.serviceName minLength 1)
			err := k8sClient.Create(ctx, serviceRoute)
			Expect(err).Should(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("spec.serviceName"))
		})
	})

	Context("When Gateway is not found", func() {
		It("should fail gracefully", func() {
			serviceRoute := &routingv1alpha1.ServiceRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-serviceroute-no-gw",
					Namespace: testNamespace,
				},
				Spec: routingv1alpha1.ServiceRouteSpec{
					ServiceName: "my-service",
					GatewayName: "nonexistent-gateway",
					Environment: "dev",
					Application: "myapp",
				},
			}

			Expect(k8sClient.Create(ctx, serviceRoute)).Should(Succeed())

			// Verify status shows Gateway not found
			Eventually(func() bool {
				var sr routingv1alpha1.ServiceRoute
				if err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      serviceRoute.Name,
					Namespace: serviceRoute.Namespace,
				}, &sr); err != nil {
					return false
				}
				for _, cond := range sr.Status.Conditions {
					if cond.Type == "Ready" && cond.Status == metav1.ConditionFalse && cond.Reason == "GatewayNotFound" {
						return true
					}
				}
				return false
			}, timeout, interval).Should(BeTrue())

			Expect(k8sClient.Delete(ctx, serviceRoute)).Should(Succeed())
		})
	})

	Context("When DNSPolicy is not found", func() {
		It("should set status to pending", func() {
			otherNs := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "other-namespace",
				},
			}
			Expect(k8sClient.Create(ctx, otherNs)).Should(Succeed())

			serviceRoute := &routingv1alpha1.ServiceRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-serviceroute-no-policy",
					Namespace: "other-namespace",
				},
				Spec: routingv1alpha1.ServiceRouteSpec{
					ServiceName:      "my-service",
					GatewayName:      gateway.Name,
					GatewayNamespace: gateway.Namespace,
					Environment:      "dev",
					Application:      "myapp",
				},
			}

			Expect(k8sClient.Create(ctx, serviceRoute)).Should(Succeed())

			// Verify status shows DNSPolicy not found
			Eventually(func() bool {
				var sr routingv1alpha1.ServiceRoute
				if err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      serviceRoute.Name,
					Namespace: serviceRoute.Namespace,
				}, &sr); err != nil {
					return false
				}
				for _, cond := range sr.Status.Conditions {
					if cond.Type == "Ready" && cond.Status == metav1.ConditionFalse && cond.Reason == "DNSPolicyNotFound" {
						return true
					}
				}
				return false
			}, timeout, interval).Should(BeTrue())

			Expect(k8sClient.Delete(ctx, serviceRoute)).Should(Succeed())
			Expect(k8sClient.Delete(ctx, otherNs)).Should(Succeed())
		})
	})

	Context("When updating ServiceRoute", func() {
		It("should update existing resources when spec changes", func() {
			serviceRoute := &routingv1alpha1.ServiceRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-serviceroute-update",
					Namespace: testNamespace,
				},
				Spec: routingv1alpha1.ServiceRouteSpec{
					ServiceName:      "my-service",
					GatewayName:      gateway.Name,
					GatewayNamespace: gateway.Namespace,
					Environment:      "dev",
					Application:      "myapp",
				},
			}

			Expect(k8sClient.Create(ctx, serviceRoute)).Should(Succeed())

			Eventually(func() string {
				var sr routingv1alpha1.ServiceRoute
				if err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      serviceRoute.Name,
					Namespace: serviceRoute.Namespace,
				}, &sr); err != nil {
					return ""
				}
				return sr.Status.DNSEndpoint
			}, timeout, interval).ShouldNot(BeEmpty())

			var sr routingv1alpha1.ServiceRoute
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      serviceRoute.Name,
				Namespace: serviceRoute.Namespace,
			}, &sr)).Should(Succeed())

			var initialDNSName string
			{
				var dnsEndpoints externaldnsv1alpha1.DNSEndpointList
				Expect(k8sClient.List(ctx, &dnsEndpoints, client.InNamespace(serviceRoute.Namespace))).Should(Succeed())
				for _, ep := range dnsEndpoints.Items {
					for _, owner := range ep.OwnerReferences {
						if owner.Name == serviceRoute.Name {
							for _, e := range ep.Spec.Endpoints {
								if e.RecordType == "CNAME" {
									initialDNSName = e.DNSName
									break
								}
							}
						}
					}
				}
				Expect(initialDNSName).ShouldNot(BeEmpty())
			}

			sr.Spec.ServiceName = "updated-service"
			Expect(k8sClient.Update(ctx, &sr)).Should(Succeed())

			Eventually(func() string {
				var dnsEndpoints externaldnsv1alpha1.DNSEndpointList
				if err := k8sClient.List(ctx, &dnsEndpoints, client.InNamespace(serviceRoute.Namespace)); err != nil {
					return ""
				}
				for _, ep := range dnsEndpoints.Items {
					for _, owner := range ep.OwnerReferences {
						if owner.Name == serviceRoute.Name {
							for _, e := range ep.Spec.Endpoints {
								if e.RecordType == "CNAME" {
									return e.DNSName
								}
							}
						}
					}
				}
				return ""
			}, timeout, interval).Should(ContainSubstring("updated-service"))

			Expect(k8sClient.Delete(ctx, serviceRoute)).Should(Succeed())
		})

		It("should handle DNSPolicy with multiple controllers in RegionBound mode", func() {
			multiNs := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-multi-namespace",
				},
			}
			Expect(k8sClient.Create(ctx, multiNs)).Should(Succeed())

			multiDNSPolicy := &routingv1alpha1.DNSPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "multi-controller-policy",
					Namespace: "test-multi-namespace",
				},
				Spec: routingv1alpha1.DNSPolicySpec{
					Mode:         "RegionBound",
					SourceRegion: "neu",
				},
			}
			Expect(k8sClient.Create(ctx, multiDNSPolicy)).Should(Succeed())

			serviceRoute := &routingv1alpha1.ServiceRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-serviceroute-multicontroller",
					Namespace: "test-multi-namespace",
				},
				Spec: routingv1alpha1.ServiceRouteSpec{
					ServiceName:      "my-service",
					GatewayName:      gateway.Name,
					GatewayNamespace: gateway.Namespace,
					Environment:      "dev",
					Application:      "myapp",
				},
			}

			Expect(k8sClient.Create(ctx, serviceRoute)).Should(Succeed())

			// RegionBound mode activates ALL controllers, creating DNSEndpoints for each
			var dnsEndpoints externaldnsv1alpha1.DNSEndpointList
			Eventually(func() int {
				if err := k8sClient.List(ctx, &dnsEndpoints, client.InNamespace("test-multi-namespace")); err != nil {
					return 0
				}
				return len(dnsEndpoints.Items)
			}, timeout, interval).Should(Equal(2))

			Expect(k8sClient.Delete(ctx, serviceRoute)).Should(Succeed())
			Expect(k8sClient.Delete(ctx, multiDNSPolicy)).Should(Succeed())
			Expect(k8sClient.Delete(ctx, multiNs)).Should(Succeed())
		})
	})

	Context("When deleting ServiceRoute", func() {
		It("should delete owned resources automatically", func() {
			serviceRoute := &routingv1alpha1.ServiceRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-serviceroute-delete",
					Namespace: testNamespace,
				},
				Spec: routingv1alpha1.ServiceRouteSpec{
					ServiceName:      "my-service",
					GatewayName:      gateway.Name,
					GatewayNamespace: gateway.Namespace,
					Environment:      "dev",
					Application:      "myapp",
				},
			}

			Expect(k8sClient.Create(ctx, serviceRoute)).Should(Succeed())

			Eventually(func() string {
				var sr routingv1alpha1.ServiceRoute
				if err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      serviceRoute.Name,
					Namespace: serviceRoute.Namespace,
				}, &sr); err != nil {
					return ""
				}
				return sr.Status.DNSEndpoint
			}, timeout, interval).ShouldNot(BeEmpty())

			Expect(k8sClient.Delete(ctx, serviceRoute)).Should(Succeed())

			var dnsEndpoints externaldnsv1alpha1.DNSEndpointList
			Eventually(func() int {
				if err := k8sClient.List(ctx, &dnsEndpoints, client.InNamespace(testNamespace)); err != nil {
					return -1
				}
				// Filter to only owned DNSEndpoints
				count := 0
				for _, ep := range dnsEndpoints.Items {
					for _, owner := range ep.OwnerReferences {
						if owner.Name == serviceRoute.Name {
							count++
						}
					}
				}
				return count
			}, timeout, interval).Should(Equal(0))
		})
	})
})
