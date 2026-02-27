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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	clusterv1alpha1 "github.com/vecozo/service-router-operator/api/cluster/v1alpha1"
	routingv1alpha1 "github.com/vecozo/service-router-operator/api/routing/v1alpha1"
)

var _ = Describe("Gateway Controller", func() {
	const (
		timeout  = time.Second * 10
		interval = time.Millisecond * 250
	)

	var clusterIdentity *clusterv1alpha1.ClusterIdentity

	BeforeEach(func() {
		ctx := context.Background()

		// Create ClusterIdentity so Gateway controller can proceed
		clusterIdentity = &clusterv1alpha1.ClusterIdentity{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-cluster-identity-gateway",
			},
			Spec: clusterv1alpha1.ClusterIdentitySpec{
				Region:            "neu",
				Cluster:           "aks01",
				Domain:            "example.com",
				EnvironmentLetter: "d",
			},
		}
		Expect(k8sClient.Create(ctx, clusterIdentity)).Should(Succeed())
	})

	AfterEach(func() {
		ctx := context.Background()

		// Clean up ClusterIdentity
		if clusterIdentity != nil {
			_ = k8sClient.Delete(ctx, clusterIdentity)
		}
	})

	Context("When reconciling a Gateway", func() {
		ctx := context.Background()

		It("should set status to Pending for valid Gateway without ServiceRoutes", func() {
			gateway := &routingv1alpha1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-gateway-valid",
					Namespace: "istio-system",
				},
				Spec: routingv1alpha1.GatewaySpec{
					Controller:     "aks-istio-ingressgateway-internal",
					CredentialName: "wildcard-cert",
					TargetPostfix:  "internal",
				},
			}

			Expect(k8sClient.Create(ctx, gateway)).Should(Succeed())

			gatewayLookupKey := types.NamespacedName{
				Name:      gateway.Name,
				Namespace: "istio-system",
			}
			createdGateway := &routingv1alpha1.Gateway{}

			Eventually(func() string {
				err := k8sClient.Get(ctx, gatewayLookupKey, createdGateway)
				if err != nil {
					return ""
				}
				return createdGateway.Status.Phase
			}, timeout, interval).Should(Equal("Pending"))

			Eventually(func() metav1.ConditionStatus {
				err := k8sClient.Get(ctx, gatewayLookupKey, createdGateway)
				if err != nil {
					return metav1.ConditionUnknown
				}
				for _, cond := range createdGateway.Status.Conditions {
					if cond.Type == "Ready" {
						return cond.Status
					}
				}
				return metav1.ConditionUnknown
			}, timeout, interval).Should(Equal(metav1.ConditionFalse))

			Expect(k8sClient.Delete(ctx, gateway)).Should(Succeed())
		})

		It("should set status to Failed for empty controller", func() {
			// Note: Due to OpenAPI validation (MinLength=1), we cannot create a Gateway
			// with an empty controller field. This test verifies the validation works.
			gateway := &routingv1alpha1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-gateway-empty-controller",
					Namespace: "istio-system",
				},
				Spec: routingv1alpha1.GatewaySpec{
					Controller:     "",
					CredentialName: "wildcard-cert",
					TargetPostfix:  "internal",
				},
			}

			err := k8sClient.Create(ctx, gateway)
			Expect(err).Should(HaveOccurred())
			Expect(err.Error()).Should(ContainSubstring("spec.controller"))
		})

		It("should set status to Failed for empty credentialName", func() {
			// Note: Due to OpenAPI validation (MinLength=1), we cannot create a Gateway
			// with an empty credentialName field. This test verifies the validation works.
			gateway := &routingv1alpha1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-gateway-empty-credential",
					Namespace: "istio-system",
				},
				Spec: routingv1alpha1.GatewaySpec{
					Controller:     "aks-istio-ingressgateway-internal",
					CredentialName: "",
					TargetPostfix:  "internal",
				},
			}

			err := k8sClient.Create(ctx, gateway)
			Expect(err).Should(HaveOccurred())
			Expect(err.Error()).Should(ContainSubstring("spec.credentialName"))
		})

		It("should set status to Failed for empty targetPostfix", func() {
			// Note: Due to OpenAPI validation (MinLength=1), we cannot create a Gateway
			// with an empty targetPostfix field. This test verifies the validation works.
			gateway := &routingv1alpha1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-gateway-empty-postfix",
					Namespace: "istio-system",
				},
				Spec: routingv1alpha1.GatewaySpec{
					Controller:     "aks-istio-ingressgateway-internal",
					CredentialName: "wildcard-cert",
					TargetPostfix:  "",
				},
			}

			err := k8sClient.Create(ctx, gateway)
			Expect(err).Should(HaveOccurred())
			Expect(err.Error()).Should(ContainSubstring("spec.targetPostfix"))
		})

		It("should set status to Failed for invalid targetPostfix format", func() {
			gateway := &routingv1alpha1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-gateway-invalid-postfix",
					Namespace: "istio-system",
				},
				Spec: routingv1alpha1.GatewaySpec{
					Controller:     "aks-istio-ingressgateway-internal",
					CredentialName: "wildcard-cert",
					TargetPostfix:  "INTERNAL_CAPS",
				},
			}

			Expect(k8sClient.Create(ctx, gateway)).Should(Succeed())

			gatewayLookupKey := types.NamespacedName{
				Name:      gateway.Name,
				Namespace: "istio-system",
			}
			createdGateway := &routingv1alpha1.Gateway{}

			Eventually(func() string {
				err := k8sClient.Get(ctx, gatewayLookupKey, createdGateway)
				if err != nil {
					return ""
				}
				return createdGateway.Status.Phase
			}, timeout, interval).Should(Equal("Failed"))

			Expect(k8sClient.Delete(ctx, gateway)).Should(Succeed())
		})

		It("should allow multiple Gateway resources", func() {
			gateway1 := &routingv1alpha1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-gateway-multi-1",
					Namespace: "istio-system",
				},
				Spec: routingv1alpha1.GatewaySpec{
					Controller:     "aks-istio-ingressgateway-internal",
					CredentialName: "wildcard-cert-internal",
					TargetPostfix:  "internal",
				},
			}

			gateway2 := &routingv1alpha1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-gateway-multi-2",
					Namespace: "istio-system",
				},
				Spec: routingv1alpha1.GatewaySpec{
					Controller:     "aks-istio-ingressgateway-external",
					CredentialName: "wildcard-cert-external",
					TargetPostfix:  "external",
				},
			}

			Expect(k8sClient.Create(ctx, gateway1)).Should(Succeed())
			Expect(k8sClient.Create(ctx, gateway2)).Should(Succeed())

			gateway1LookupKey := types.NamespacedName{
				Name:      gateway1.Name,
				Namespace: "istio-system",
			}
			gateway2LookupKey := types.NamespacedName{
				Name:      gateway2.Name,
				Namespace: "istio-system",
			}
			createdGateway1 := &routingv1alpha1.Gateway{}
			createdGateway2 := &routingv1alpha1.Gateway{}

			Eventually(func() string {
				err := k8sClient.Get(ctx, gateway1LookupKey, createdGateway1)
				if err != nil {
					return ""
				}
				return createdGateway1.Status.Phase
			}, timeout, interval).Should(Equal("Pending"))

			Eventually(func() string {
				err := k8sClient.Get(ctx, gateway2LookupKey, createdGateway2)
				if err != nil {
					return ""
				}
				return createdGateway2.Status.Phase
			}, timeout, interval).Should(Equal("Pending"))

			Expect(k8sClient.Delete(ctx, gateway1)).Should(Succeed())
			Expect(k8sClient.Delete(ctx, gateway2)).Should(Succeed())
		})

		It("should update status when spec is updated", func() {
			gateway := &routingv1alpha1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-gateway-update",
					Namespace: "istio-system",
				},
				Spec: routingv1alpha1.GatewaySpec{
					Controller:     "aks-istio-ingressgateway-internal",
					CredentialName: "wildcard-cert",
					TargetPostfix:  "internal",
				},
			}

			Expect(k8sClient.Create(ctx, gateway)).Should(Succeed())

			gatewayLookupKey := types.NamespacedName{
				Name:      gateway.Name,
				Namespace: "istio-system",
			}
			createdGateway := &routingv1alpha1.Gateway{}

			Eventually(func() string {
				err := k8sClient.Get(ctx, gatewayLookupKey, createdGateway)
				if err != nil {
					return ""
				}
				return createdGateway.Status.Phase
			}, timeout, interval).Should(Equal("Pending"))

			Eventually(func() error {
				err := k8sClient.Get(ctx, gatewayLookupKey, createdGateway)
				if err != nil {
					return err
				}
				createdGateway.Spec.TargetPostfix = "INVALID_FORMAT"
				return k8sClient.Update(ctx, createdGateway)
			}, timeout, interval).Should(Succeed())

			Eventually(func() string {
				err := k8sClient.Get(ctx, gatewayLookupKey, createdGateway)
				if err != nil {
					return ""
				}
				return createdGateway.Status.Phase
			}, timeout, interval).Should(Equal("Failed"))

			Expect(k8sClient.Delete(ctx, gateway)).Should(Succeed())
		})

		It("should set status to Pending when no ServiceRoutes reference the Gateway", func() {
			gateway := &routingv1alpha1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-gateway-no-routes",
					Namespace: "istio-system",
				},
				Spec: routingv1alpha1.GatewaySpec{
					Controller:     "aks-istio-ingressgateway-internal",
					CredentialName: "wildcard-cert",
					TargetPostfix:  "internal",
				},
			}

			Expect(k8sClient.Create(ctx, gateway)).Should(Succeed())

			gatewayLookupKey := types.NamespacedName{
				Name:      gateway.Name,
				Namespace: "istio-system",
			}
			createdGateway := &routingv1alpha1.Gateway{}

			Eventually(func() string {
				err := k8sClient.Get(ctx, gatewayLookupKey, createdGateway)
				if err != nil {
					return ""
				}
				return createdGateway.Status.Phase
			}, timeout, interval).Should(Equal("Pending"))

			Eventually(func() string {
				err := k8sClient.Get(ctx, gatewayLookupKey, createdGateway)
				if err != nil {
					return ""
				}
				for _, cond := range createdGateway.Status.Conditions {
					if cond.Type == "Ready" {
						return cond.Reason
					}
				}
				return ""
			}, timeout, interval).Should(Equal("NoServiceRoutes"))

			Eventually(func() string {
				err := k8sClient.Get(ctx, gatewayLookupKey, createdGateway)
				if err != nil {
					return ""
				}
				for _, cond := range createdGateway.Status.Conditions {
					if cond.Type == "Ready" {
						return cond.Message
					}
				}
				return ""
			}, timeout, interval).Should(Equal("Waiting for ServiceRoutes to reference this Gateway"))

			Expect(k8sClient.Delete(ctx, gateway)).Should(Succeed())
		})

		It("should transition from Pending to Active when ServiceRoute is added", func() {
			gateway := &routingv1alpha1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-gateway-transition",
					Namespace: "istio-system",
				},
				Spec: routingv1alpha1.GatewaySpec{
					Controller:     "aks-istio-ingressgateway-internal",
					CredentialName: "wildcard-cert",
					TargetPostfix:  "internal",
				},
			}

			Expect(k8sClient.Create(ctx, gateway)).Should(Succeed())

			gatewayLookupKey := types.NamespacedName{
				Name:      gateway.Name,
				Namespace: "istio-system",
			}
			createdGateway := &routingv1alpha1.Gateway{}

			// Initially should be Pending
			Eventually(func() string {
				err := k8sClient.Get(ctx, gatewayLookupKey, createdGateway)
				if err != nil {
					return ""
				}
				return createdGateway.Status.Phase
			}, timeout, interval).Should(Equal("Pending"))

			// Create a ServiceRoute that references this Gateway
			serviceRoute := &routingv1alpha1.ServiceRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-route-for-gateway",
					Namespace: "default",
				},
				Spec: routingv1alpha1.ServiceRouteSpec{
					GatewayName:      gateway.Name,
					GatewayNamespace: "istio-system",
					ServiceName:      "my-service",
					Environment:      "dev",
					Application:      "myapp",
				},
			}

			Expect(k8sClient.Create(ctx, serviceRoute)).Should(Succeed())

			// Should transition to Active
			Eventually(func() string {
				err := k8sClient.Get(ctx, gatewayLookupKey, createdGateway)
				if err != nil {
					return ""
				}
				return createdGateway.Status.Phase
			}, timeout, interval).Should(Equal("Active"))

			Expect(k8sClient.Delete(ctx, serviceRoute)).Should(Succeed())
			Expect(k8sClient.Delete(ctx, gateway)).Should(Succeed())
		})
	})
})
