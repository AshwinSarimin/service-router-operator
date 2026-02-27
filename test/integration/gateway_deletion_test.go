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
	istioclientv1beta1 "istio.io/client-go/pkg/apis/networking/v1beta1"
)

var _ = Describe("Gateway Deletion Integration", func() {
	var namespace string
	var clusterIdentity *clusterv1alpha1.ClusterIdentity
	var dnsPolicy *routingv1alpha1.DNSPolicy

	BeforeEach(func() {
		timestamp := time.Now().Unix()
		namespace = fmt.Sprintf("test-gw-del-%d", timestamp)
		CreateNamespace(namespace)

		// Create ClusterIdentity
		clusterIdentity = &clusterv1alpha1.ClusterIdentity{
			ObjectMeta: metav1.ObjectMeta{
				Name: fmt.Sprintf("test-identity-gw-%d", timestamp),
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
				Name:      "test-policy",
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
	})

	Context("When Gateway is deleted while ServiceRoute references it", func() {
		It("Should cause ServiceRoute to fail reconciliation", func() {
			// Create Gateway in istio-system namespace
			gateway := &routingv1alpha1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-gateway-deletion",
					Namespace: "istio-system",
				},
				Spec: routingv1alpha1.GatewaySpec{
					Controller:     "aks-istio-ingressgateway-internal",
					CredentialName: "cert-aks-ingress",
					TargetPostfix:  "external",
				},
			}
			Expect(k8sClient.Create(ctx, gateway)).To(Succeed())

			// Create ServiceRoute that references the gateway
			serviceRoute := &routingv1alpha1.ServiceRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-route-gw-deletion",
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

			// Wait for Gateway to become Ready now that ServiceRoute exists
			WaitForCondition(gateway, "Ready", metav1.ConditionTrue)
			WaitForCondition(serviceRoute, "Ready", metav1.ConditionTrue)

			// Verify Istio Gateway exists (named after the Gateway CR, in istio-system)
			istioGW := &istioclientv1beta1.Gateway{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      gateway.Name,
					Namespace: "istio-system",
				}, istioGW)
			}, timeout, interval).Should(Succeed())

			// Delete the Gateway resource
			DeleteObject(gateway)

			// Trigger ServiceRoute reconciliation by updating it
			Eventually(func() error {
				var sr routingv1alpha1.ServiceRoute
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(serviceRoute), &sr)
				if err != nil {
					return err
				}
				if sr.Labels == nil {
					sr.Labels = make(map[string]string)
				}
				sr.Labels["trigger"] = "reconcile"
				return k8sClient.Update(ctx, &sr)
			}, timeout, interval).Should(Succeed())

			// ServiceRoute should transition to Failed condition
			Eventually(func() bool {
				var sr routingv1alpha1.ServiceRoute
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(serviceRoute), &sr)
				if err != nil {
					return false
				}
				for _, cond := range sr.Status.Conditions {
					if cond.Type == "Ready" && cond.Status == metav1.ConditionFalse {
						return true
					}
				}
				return false
			}, timeout, interval).Should(BeTrue())

			// Verify Ready condition is False with GatewayNotFound reason
			Eventually(func() bool {
				var sr routingv1alpha1.ServiceRoute
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(serviceRoute), &sr)
				if err != nil {
					return false
				}
				for _, cond := range sr.Status.Conditions {
					if cond.Type == "Ready" && cond.Status == metav1.ConditionFalse &&
						cond.Reason == "GatewayNotFound" {
						return true
					}
				}
				return false
			}, timeout, interval).Should(BeTrue())

			// Cleanup
			DeleteObject(serviceRoute)
		})
	})
})
