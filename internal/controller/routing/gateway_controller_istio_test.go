package routing

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	networkingv1beta1 "istio.io/api/networking/v1beta1"
	istioclientv1beta1 "istio.io/client-go/pkg/apis/networking/v1beta1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	clusterv1alpha1 "github.com/vecozo/service-router-operator/api/cluster/v1alpha1"
	routingv1alpha1 "github.com/vecozo/service-router-operator/api/routing/v1alpha1"
)

var _ = Describe("Gateway Controller - Istio Gateway Management", func() {
	const (
		timeout  = time.Second * 10
		interval = time.Millisecond * 250
	)

	var clusterIdentity *clusterv1alpha1.ClusterIdentity

	BeforeEach(func() {
		ctx := context.Background()

		// Create ClusterIdentity for domain (istio-system namespace created in suite_test.go BeforeSuite)
		clusterIdentity = &clusterv1alpha1.ClusterIdentity{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-cluster-identity-istio",
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

		if clusterIdentity != nil {
			_ = k8sClient.Delete(ctx, clusterIdentity)
		}

		// Don't delete istio-system namespace - it's shared across tests
		// and namespace deletion is async, causing "namespace is being terminated" errors
	})

	It("should create an Istio Gateway when ServiceRoutes exist", func() {
		ctx := context.Background()
		gateway := &routingv1alpha1.Gateway{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-istio-wildcard",
				Namespace: "istio-system",
			},
			Spec: routingv1alpha1.GatewaySpec{
				Controller:     "istio-ingressgateway",
				CredentialName: "test-tls",
				TargetPostfix:  "external",
			},
		}
		Expect(k8sClient.Create(ctx, gateway)).Should(Succeed())
		defer func() { err := k8sClient.Delete(ctx, gateway); Expect(err).To(Succeed()) }()

		// Create ServiceRoute to trigger Istio Gateway creation
		serviceRoute := &routingv1alpha1.ServiceRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-route-istio",
				Namespace: "default",
			},
			Spec: routingv1alpha1.ServiceRouteSpec{
				ServiceName: "test-svc",
				GatewayName: "test-istio-wildcard",
				Environment: "dev",
				Application: "testapp",
			},
		}
		Expect(k8sClient.Create(ctx, serviceRoute)).Should(Succeed())
		defer func() { err := k8sClient.Delete(ctx, serviceRoute); Expect(err).To(Succeed()) }()

		istioGateway := &istioclientv1beta1.Gateway{}
		Eventually(func() error {
			return k8sClient.Get(ctx, types.NamespacedName{
				Name:      "test-istio-wildcard",
				Namespace: "istio-system",
			}, istioGateway)
		}, timeout, interval).Should(Succeed())

		Expect(istioGateway.Spec.Servers).To(HaveLen(1))
		Expect(istioGateway.Spec.Servers[0].Hosts).To(ContainElement("test-svc-ns-d-dev-testapp.example.com"))
		Expect(istioGateway.Spec.Servers[0].Port.Protocol).To(Equal("HTTPS"))
		Expect(istioGateway.Spec.Servers[0].Port.Number).To(Equal(uint32(443)))
		Expect(istioGateway.Spec.Servers[0].Tls.Mode).To(Equal(networkingv1beta1.ServerTLSSettings_SIMPLE))
		Expect(istioGateway.Spec.Servers[0].Tls.CredentialName).To(Equal("test-tls"))
		Expect(istioGateway.Spec.Selector).To(HaveKeyWithValue("istio", "istio-ingressgateway"))
	})

	It("should create an Istio Gateway in a custom namespace", func() {
		ctx := context.Background()

		customNs := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "custom-istio-namespace",
			},
		}
		Expect(k8sClient.Create(ctx, customNs)).Should(Succeed())
		defer func() { err := k8sClient.Delete(ctx, customNs); Expect(err).To(Succeed()) }()

		gateway := &routingv1alpha1.Gateway{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-custom-ns",
				Namespace: "custom-istio-namespace",
			},
			Spec: routingv1alpha1.GatewaySpec{
				Controller:     "istio-ingressgateway",
				CredentialName: "custom-tls",
				TargetPostfix:  "internal",
			},
		}
		Expect(k8sClient.Create(ctx, gateway)).Should(Succeed())
		defer func() { err := k8sClient.Delete(ctx, gateway); Expect(err).To(Succeed()) }()

		// Create ServiceRoute to trigger Istio Gateway creation
		serviceRoute := &routingv1alpha1.ServiceRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-route-custom-ns",
				Namespace: "custom-istio-namespace",
			},
			Spec: routingv1alpha1.ServiceRouteSpec{
				ServiceName:      "test-svc",
				GatewayName:      "test-custom-ns",
				GatewayNamespace: "custom-istio-namespace",
				Environment:      "dev",
				Application:      "testapp",
			},
		}
		Expect(k8sClient.Create(ctx, serviceRoute)).Should(Succeed())
		defer func() { err := k8sClient.Delete(ctx, serviceRoute); Expect(err).To(Succeed()) }()

		istioGateway := &istioclientv1beta1.Gateway{}
		Eventually(func() error {
			return k8sClient.Get(ctx, types.NamespacedName{
				Name:      "test-custom-ns",
				Namespace: "custom-istio-namespace",
			}, istioGateway)
		}, timeout, interval).Should(Succeed())
	})

	It("should aggregate hosts from ServiceRoutes using the Gateway", func() {
		ctx := context.Background()
		gateway := &routingv1alpha1.Gateway{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-host-agg",
				Namespace: "istio-system",
			},
			Spec: routingv1alpha1.GatewaySpec{
				Controller:     "istio-ingressgateway",
				CredentialName: "test-tls",
				TargetPostfix:  "external",
			},
		}
		Expect(k8sClient.Create(ctx, gateway)).Should(Succeed())
		defer func() { err := k8sClient.Delete(ctx, gateway); Expect(err).To(Succeed()) }()

		serviceRoute1 := &routingv1alpha1.ServiceRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-route-agg-1",
				Namespace: "default",
			},
			Spec: routingv1alpha1.ServiceRouteSpec{
				ServiceName: "app1",
				GatewayName: "test-host-agg",
				Environment: "dev",
				Application: "testapp",
			},
		}
		Expect(k8sClient.Create(ctx, serviceRoute1)).Should(Succeed())
		defer func() { err := k8sClient.Delete(ctx, serviceRoute1); Expect(err).To(Succeed()) }()

		serviceRoute2 := &routingv1alpha1.ServiceRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-route-agg-2",
				Namespace: "default",
			},
			Spec: routingv1alpha1.ServiceRouteSpec{
				ServiceName: "app2",
				GatewayName: "test-host-agg",
				Environment: "dev",
				Application: "testapp",
			},
		}
		Expect(k8sClient.Create(ctx, serviceRoute2)).Should(Succeed())
		defer func() { err := k8sClient.Delete(ctx, serviceRoute2); Expect(err).To(Succeed()) }()

		istioGateway := &istioclientv1beta1.Gateway{}
		Eventually(func() []string {
			if err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      "test-host-agg",
				Namespace: "istio-system",
			}, istioGateway); err != nil {
				return nil
			}
			if len(istioGateway.Spec.Servers) == 0 {
				return nil
			}
			return istioGateway.Spec.Servers[0].Hosts
		}, timeout, interval).Should(ConsistOf(
			"app1-ns-d-dev-testapp.example.com",
			"app2-ns-d-dev-testapp.example.com",
		))
	})

	It("should create Istio Gateway when ServiceRoutes are added", func() {
		ctx := context.Background()
		gateway := &routingv1alpha1.Gateway{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-dynamic",
				Namespace: "istio-system",
			},
			Spec: routingv1alpha1.GatewaySpec{
				Controller:     "istio-ingressgateway",
				CredentialName: "test-tls",
				TargetPostfix:  "external",
			},
		}
		Expect(k8sClient.Create(ctx, gateway)).Should(Succeed())
		defer func() { err := k8sClient.Delete(ctx, gateway); Expect(err).To(Succeed()) }()

		// Verify Gateway starts in Pending state
		createdGateway := &routingv1alpha1.Gateway{}
		Eventually(func() string {
			if err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      "test-dynamic",
				Namespace: "istio-system",
			}, createdGateway); err != nil {
				return ""
			}
			return createdGateway.Status.Phase
		}, timeout, interval).Should(Equal("Pending"))

		// Create ServiceRoute to trigger Istio Gateway creation
		serviceRoute := &routingv1alpha1.ServiceRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-dynamic-route",
				Namespace: "default",
			},
			Spec: routingv1alpha1.ServiceRouteSpec{
				ServiceName: "dynamic",
				GatewayName: "test-dynamic",
				Environment: "prod",
				Application: "myapp",
			},
		}
		Expect(k8sClient.Create(ctx, serviceRoute)).Should(Succeed())
		defer func() { err := k8sClient.Delete(ctx, serviceRoute); Expect(err).To(Succeed()) }()

		// Verify Istio Gateway is created with the ServiceRoute host
		istioGateway := &istioclientv1beta1.Gateway{}
		Eventually(func() []string {
			if err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      "test-dynamic",
				Namespace: "istio-system",
			}, istioGateway); err != nil {
				return nil
			}
			if len(istioGateway.Spec.Servers) == 0 {
				return nil
			}
			return istioGateway.Spec.Servers[0].Hosts
		}, timeout, interval).Should(ConsistOf("dynamic-ns-d-prod-myapp.example.com"))
	})

	It("should delete Istio Gateway when all ServiceRoutes are deleted", func() {
		ctx := context.Background()
		gateway := &routingv1alpha1.Gateway{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-revert",
				Namespace: "istio-system",
			},
			Spec: routingv1alpha1.GatewaySpec{
				Controller:     "istio-ingressgateway",
				CredentialName: "test-tls",
				TargetPostfix:  "external",
			},
		}
		Expect(k8sClient.Create(ctx, gateway)).Should(Succeed())
		defer func() { err := k8sClient.Delete(ctx, gateway); Expect(err).To(Succeed()) }()

		serviceRoute := &routingv1alpha1.ServiceRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-temp",
				Namespace: "default",
			},
			Spec: routingv1alpha1.ServiceRouteSpec{
				ServiceName: "temp",
				GatewayName: "test-revert",
				Environment: "dev",
				Application: "test",
			},
		}
		Expect(k8sClient.Create(ctx, serviceRoute)).Should(Succeed())

		istioGateway := &istioclientv1beta1.Gateway{}
		Eventually(func() []string {
			if err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      "test-revert",
				Namespace: "istio-system",
			}, istioGateway); err != nil {
				return nil
			}
			if len(istioGateway.Spec.Servers) == 0 {
				return nil
			}
			return istioGateway.Spec.Servers[0].Hosts
		}, timeout, interval).Should(ConsistOf("temp-ns-d-dev-test.example.com"))

		Expect(k8sClient.Delete(ctx, serviceRoute)).Should(Succeed())

		// Verify Gateway status transitions to Pending
		createdGateway := &routingv1alpha1.Gateway{}
		Eventually(func() string {
			if err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      "test-revert",
				Namespace: "istio-system",
			}, createdGateway); err != nil {
				return ""
			}
			return createdGateway.Status.Phase
		}, timeout, interval).Should(Equal("Pending"))

		// Verify Istio Gateway is deleted
		Eventually(func() bool {
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      "test-revert",
				Namespace: "istio-system",
			}, istioGateway)
			return apierrors.IsNotFound(err)
		}, timeout, interval).Should(BeTrue())
	})

	It("should delete Istio Gateway when Gateway resource is deleted", func() {
		ctx := context.Background()
		gateway := &routingv1alpha1.Gateway{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-deletion",
				Namespace: "istio-system",
			},
			Spec: routingv1alpha1.GatewaySpec{
				Controller:     "istio-ingressgateway",
				CredentialName: "test-tls",
				TargetPostfix:  "external",
			},
		}
		Expect(k8sClient.Create(ctx, gateway)).Should(Succeed())

		// Create ServiceRoute to trigger Istio Gateway creation
		serviceRoute := &routingv1alpha1.ServiceRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-route-deletion",
				Namespace: "default",
			},
			Spec: routingv1alpha1.ServiceRouteSpec{
				ServiceName: "del-svc",
				GatewayName: "test-deletion",
				Environment: "dev",
				Application: "testapp",
			},
		}
		Expect(k8sClient.Create(ctx, serviceRoute)).Should(Succeed())
		defer func() { _ = k8sClient.Delete(ctx, serviceRoute) }()

		istioGateway := &istioclientv1beta1.Gateway{}
		Eventually(func() error {
			return k8sClient.Get(ctx, types.NamespacedName{
				Name:      "test-deletion",
				Namespace: "istio-system",
			}, istioGateway)
		}, timeout, interval).Should(Succeed())

		// Verify owner reference is set correctly
		Expect(istioGateway.OwnerReferences).To(HaveLen(1))
		Expect(istioGateway.OwnerReferences[0].Name).To(Equal("test-deletion"))
		Expect(istioGateway.OwnerReferences[0].Kind).To(Equal("Gateway"))
		Expect(*istioGateway.OwnerReferences[0].Controller).To(BeTrue())

		Expect(k8sClient.Delete(ctx, gateway)).Should(Succeed())

		// Note: Automatic garbage collection via owner references doesn't work in envtest
		// because envtest doesn't run the full Kubernetes garbage collector.
		// In a real cluster, the Istio Gateway would be automatically deleted.
		// We verify the owner reference above to ensure it's configured correctly.

		// Manual cleanup for test environment
		_ = k8sClient.Delete(ctx, istioGateway)
	})

	It("should update Istio Gateway TLS secret when Gateway spec changes", func() {
		ctx := context.Background()
		gateway := &routingv1alpha1.Gateway{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-tls-update",
				Namespace: "istio-system",
			},
			Spec: routingv1alpha1.GatewaySpec{
				Controller:     "istio-ingressgateway",
				CredentialName: "original-tls",
				TargetPostfix:  "external",
			},
		}
		Expect(k8sClient.Create(ctx, gateway)).Should(Succeed())
		defer func() { err := k8sClient.Delete(ctx, gateway); Expect(err).To(Succeed()) }()

		// Create ServiceRoute to trigger Istio Gateway creation
		serviceRoute := &routingv1alpha1.ServiceRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-route-tls",
				Namespace: "default",
			},
			Spec: routingv1alpha1.ServiceRouteSpec{
				ServiceName: "tls-svc",
				GatewayName: "test-tls-update",
				Environment: "dev",
				Application: "testapp",
			},
		}
		Expect(k8sClient.Create(ctx, serviceRoute)).Should(Succeed())
		defer func() { err := k8sClient.Delete(ctx, serviceRoute); Expect(err).To(Succeed()) }()

		istioGateway := &istioclientv1beta1.Gateway{}
		Eventually(func() string {
			if err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      "test-tls-update",
				Namespace: "istio-system",
			}, istioGateway); err != nil {
				return ""
			}
			if len(istioGateway.Spec.Servers) == 0 || istioGateway.Spec.Servers[0].Tls == nil {
				return ""
			}
			return istioGateway.Spec.Servers[0].Tls.CredentialName
		}, timeout, interval).Should(Equal("original-tls"))

		Eventually(func() error {
			if err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      "test-tls-update",
				Namespace: "istio-system",
			}, gateway); err != nil {
				return err
			}
			gateway.Spec.CredentialName = "updated-tls"
			return k8sClient.Update(ctx, gateway)
		}, timeout, interval).Should(Succeed())

		Eventually(func() string {
			if err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      "test-tls-update",
				Namespace: "istio-system",
			}, istioGateway); err != nil {
				return ""
			}
			if len(istioGateway.Spec.Servers) == 0 || istioGateway.Spec.Servers[0].Tls == nil {
				return ""
			}
			return istioGateway.Spec.Servers[0].Tls.CredentialName
		}, timeout, interval).Should(Equal("updated-tls"))
	})
})
