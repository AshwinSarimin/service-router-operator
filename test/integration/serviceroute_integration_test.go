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
	externaldnsv1alpha1 "sigs.k8s.io/external-dns/apis/v1alpha1"
)

var _ = Describe("ServiceRoute Full Stack Integration", func() {
	var namespace string
	var clusterIdentity *clusterv1alpha1.ClusterIdentity
	var gateway *routingv1alpha1.Gateway
	var dnsPolicy *routingv1alpha1.DNSPolicy

	BeforeEach(func() {
		timestamp := time.Now().Unix()
		namespace = fmt.Sprintf("test-sr-full-%d", timestamp)
		CreateNamespace(namespace)

		// Create ClusterIdentity
		clusterIdentity = &clusterv1alpha1.ClusterIdentity{
			ObjectMeta: metav1.ObjectMeta{
				Name: fmt.Sprintf("test-identity-sr-%d", timestamp),
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
				Name:      fmt.Sprintf("test-gateway-sr-%d", timestamp),
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
				Name:      "test-policy-sr",
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

	Context("When creating a complete ServiceRoute stack", func() {
		It("Should create DNSEndpoints with correct configuration", func() {
			serviceRoute := &routingv1alpha1.ServiceRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-route-full",
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

			// Verify DNSEndpoint is created for active controller
			Eventually(func() error {
				var dnsEndpoint externaldnsv1alpha1.DNSEndpoint
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      "test-route-full-external-dns-neu",
					Namespace: namespace,
				}, &dnsEndpoint)
			}, timeout, interval).Should(Succeed())

			// Verify DNSEndpoint has correct records
			var dnsEndpoint externaldnsv1alpha1.DNSEndpoint
			if err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      "test-route-full-external-dns-neu",
				Namespace: namespace,
			}, &dnsEndpoint); err != nil {
				Fail(err.Error())
			}

			Expect(dnsEndpoint.Spec.Endpoints).To(HaveLen(1))
			expectedSourceHost := "my-service-ns-d-dev-myapp.example.com"
			expectedTargetHost := "aks01-neu-external.example.com"

			Expect(dnsEndpoint.Spec.Endpoints[0].DNSName).To(Equal(expectedSourceHost))
			Expect(dnsEndpoint.Spec.Endpoints[0].RecordType).To(Equal("CNAME"))
			Expect(dnsEndpoint.Spec.Endpoints[0].Targets).To(HaveLen(1))
			Expect(dnsEndpoint.Spec.Endpoints[0].Targets[0]).To(Equal(expectedTargetHost))

			// Cleanup
			DeleteObject(serviceRoute)
		})
	})

	Context("When deleting a ServiceRoute", func() {
		It("Should delete owned DNSEndpoints", func() {
			serviceRoute := &routingv1alpha1.ServiceRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-route-delete",
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

			// Verify DNSEndpoint is created
			Eventually(func() error {
				var dnsEndpoint externaldnsv1alpha1.DNSEndpoint
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      "test-route-delete-external-dns-neu",
					Namespace: namespace,
				}, &dnsEndpoint)
			}, timeout, interval).Should(Succeed())

			// Delete ServiceRoute
			DeleteObject(serviceRoute)

			// Verify DNSEndpoint is deleted (owned resource with cascade deletion via OwnerReference)
			Eventually(func() bool {
				var dnsEndpoint externaldnsv1alpha1.DNSEndpoint
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      "test-route-delete-external-dns-neu",
					Namespace: namespace,
				}, &dnsEndpoint)
				return errors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())
		})
	})
})
