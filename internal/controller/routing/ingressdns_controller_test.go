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
	externaldnsv1alpha1 "sigs.k8s.io/external-dns/apis/v1alpha1"

	clusterv1alpha1 "github.com/vecozo/service-router-operator/api/cluster/v1alpha1"
	routingv1alpha1 "github.com/vecozo/service-router-operator/api/routing/v1alpha1"
)

var _ = Describe("IngressDNS Controller", func() {
	const (
		timeout  = time.Second * 10
		interval = time.Millisecond * 250
	)

	var (
		ctx             context.Context
		clusterIdentity *clusterv1alpha1.ClusterIdentity
		dnsConfig       *clusterv1alpha1.DNSConfiguration
	)

	BeforeEach(func() {
		ctx = context.Background()

		// Cleanup all existing ClusterIdentities to avoid pollution
		var identities clusterv1alpha1.ClusterIdentityList
		_ = k8sClient.List(ctx, &identities)
		for _, id := range identities.Items {
			_ = k8sClient.Delete(ctx, &id)
		}

		// Cleanup all existing DNSConfigurations
		var configs clusterv1alpha1.DNSConfigurationList
		_ = k8sClient.List(ctx, &configs)
		for _, cfg := range configs.Items {
			_ = k8sClient.Delete(ctx, &cfg)
		}

		// Create ClusterIdentity
		clusterIdentity = &clusterv1alpha1.ClusterIdentity{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-cluster-identity-dns-gw",
			},
			Spec: clusterv1alpha1.ClusterIdentitySpec{
				Region:            "neu",
				Cluster:           "aks01",
				Domain:            "example.com",
				EnvironmentLetter: "d",
			},
		}
		// Ignore error if it already exists (from other tests)
		_ = k8sClient.Create(ctx, clusterIdentity)

		// Create DNSConfiguration
		dnsConfig = &clusterv1alpha1.DNSConfiguration{
			ObjectMeta: metav1.ObjectMeta{
				Name: "default-dns-config-gw",
			},
			Spec: clusterv1alpha1.DNSConfigurationSpec{
				// GatewayEndpointNamespace is no longer used/required
				ExternalDNSControllers: []clusterv1alpha1.ExternalDNSController{
					{
						Name:   "external-dns-private",
						Region: "neu",
					},
				},
			},
		}
		_ = k8sClient.Create(ctx, dnsConfig)
	})

	AfterEach(func() {
		if clusterIdentity != nil {
			_ = k8sClient.Delete(ctx, clusterIdentity)
		}
		if dnsConfig != nil {
			_ = k8sClient.Delete(ctx, dnsConfig)
		}
	})

	Context("When reconciling Gateway DNS", func() {
		It("should create DNSEndpoints in the same namespace as the Service", func() {
			controllerName := "test-controller-dns"
			serviceNamespace := "custom-gateway-ns"

			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: serviceNamespace}}
			_ = k8sClient.Create(ctx, ns)

			service := &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "istio-ingressgateway-dns",
					Namespace: serviceNamespace,
					Labels: map[string]string{
						"istio": controllerName,
					},
				},
				Spec: corev1.ServiceSpec{
					Type: corev1.ServiceTypeLoadBalancer,
					Ports: []corev1.ServicePort{
						{
							Port: 80,
							Name: "http2",
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, service)).Should(Succeed())

			service.Status.LoadBalancer.Ingress = []corev1.LoadBalancerIngress{
				{IP: "20.20.20.20"},
			}
			Expect(k8sClient.Status().Update(ctx, service)).Should(Succeed())

			gateway := &routingv1alpha1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-gateway-dns",
					Namespace: "default",
				},
				Spec: routingv1alpha1.GatewaySpec{
					Controller:     controllerName,
					CredentialName: "wildcard-cert",
					TargetPostfix:  "internal",
				},
			}
			Expect(k8sClient.Create(ctx, gateway)).Should(Succeed())

			dnsEndpointName := fmt.Sprintf("gateway-controller-%s-%s-%s", controllerName, "internal", "external-dns-private")
			dnsEndpointLookupKey := types.NamespacedName{
				Name:      dnsEndpointName,
				Namespace: serviceNamespace, // Expecting it in the service namespace
			}
			createdDNSEndpoint := &externaldnsv1alpha1.DNSEndpoint{}

			Eventually(func() string {
				err := k8sClient.Get(ctx, dnsEndpointLookupKey, createdDNSEndpoint)
				if err != nil {
					return ""
				}
				if len(createdDNSEndpoint.Spec.Endpoints) == 0 {
					return ""
				}
				if len(createdDNSEndpoint.Spec.Endpoints[0].Targets) > 0 {
					return createdDNSEndpoint.Spec.Endpoints[0].Targets[0]
				}
				return ""
			}, timeout, interval).Should(Equal("20.20.20.20"))

			expectedHost := "aks01-neu-internal.example.com"
			Expect(createdDNSEndpoint.Spec.Endpoints[0].DNSName).To(Equal(expectedHost))

			Expect(createdDNSEndpoint.OwnerReferences).To(HaveLen(1))
			Expect(createdDNSEndpoint.OwnerReferences[0].Kind).To(Equal("Service"))
			Expect(createdDNSEndpoint.OwnerReferences[0].Name).To(Equal(service.Name))

			gatewayLookupKey := types.NamespacedName{
				Name:      gateway.Name,
				Namespace: gateway.Namespace,
			}
			updatedGateway := &routingv1alpha1.Gateway{}
			Eventually(func() string {
				err := k8sClient.Get(ctx, gatewayLookupKey, updatedGateway)
				if err != nil {
					return ""
				}
				for _, cond := range updatedGateway.Status.Conditions {
					if cond.Type == "DNSReady" {
						return string(cond.Status)
					}
				}
				return ""
			}, timeout, interval).Should(Equal("True"))

			Expect(k8sClient.Delete(ctx, gateway)).Should(Succeed())
			Expect(k8sClient.Delete(ctx, service)).Should(Succeed())
			_ = k8sClient.Delete(ctx, ns)

			// Verify DNSEndpoint deletion (cleanup)
			// Since we added OwnerReference, K8s GC would handle it in real cluster.
			// But here we rely on the controller's cleanup logic or envtest GC if enabled.
			// The controller cleanup logic (orphaned) only deletes if the controller is no longer active.
			// However, since we deleted the Gateway, the controller config is no longer active, so the operator should clean it up.
			Eventually(func() bool {
				err := k8sClient.Get(ctx, dnsEndpointLookupKey, createdDNSEndpoint)
				return err != nil
			}, timeout, interval).Should(BeTrue())
		})

		It("should cleanup DNSEndpoints when Gateway is deleted but Service remains", func() {
			controllerName := "test-controller-orphan"
			serviceNamespace := "orphan-ns"

			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: serviceNamespace}}
			_ = k8sClient.Create(ctx, ns)
			defer func() { _ = k8sClient.Delete(ctx, ns) }()

			service := &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "istio-ingressgateway-orphan",
					Namespace: serviceNamespace,
					Labels: map[string]string{
						"istio": controllerName,
					},
				},
				Spec: corev1.ServiceSpec{
					Type:  corev1.ServiceTypeLoadBalancer,
					Ports: []corev1.ServicePort{{Port: 80}},
				},
			}
			Expect(k8sClient.Create(ctx, service)).Should(Succeed())

			svcToUpdate := &corev1.Service{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{Name: service.Name, Namespace: service.Namespace}, svcToUpdate)
			}, timeout, interval).Should(Succeed())
			svcToUpdate.Status.LoadBalancer.Ingress = []corev1.LoadBalancerIngress{
				{IP: "30.30.30.30"},
			}
			Expect(k8sClient.Status().Update(ctx, svcToUpdate)).Should(Succeed())

			gateway := &routingv1alpha1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-gateway-orphan",
					Namespace: "default",
				},
				Spec: routingv1alpha1.GatewaySpec{
					Controller:     controllerName,
					CredentialName: "wildcard-cert",
					TargetPostfix:  "orphan",
				},
			}
			Expect(k8sClient.Create(ctx, gateway)).Should(Succeed())

			dnsEndpointName := fmt.Sprintf("gateway-controller-%s-%s-%s", controllerName, "orphan", "external-dns-private")
			dnsEndpointLookupKey := types.NamespacedName{Name: dnsEndpointName, Namespace: serviceNamespace}

			Eventually(func() error {
				return k8sClient.Get(ctx, dnsEndpointLookupKey, &externaldnsv1alpha1.DNSEndpoint{})
			}, timeout, interval).Should(Succeed())

			Expect(k8sClient.Delete(ctx, gateway)).Should(Succeed())

			Eventually(func() bool {
				err := k8sClient.Get(ctx, dnsEndpointLookupKey, &externaldnsv1alpha1.DNSEndpoint{})
				return apierrors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue(), "DNSEndpoint should be deleted when Gateway is removed")
		})
	})
})
