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
	"context"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	clusterv1alpha1 "github.com/vecozo/service-router-operator/api/cluster/v1alpha1"
	routingv1alpha1 "github.com/vecozo/service-router-operator/api/routing/v1alpha1"
	clustercontroller "github.com/vecozo/service-router-operator/internal/controller/cluster"
	routingcontroller "github.com/vecozo/service-router-operator/internal/controller/routing"
	istioclientv1beta1 "istio.io/client-go/pkg/apis/networking/v1beta1"
	externaldnsv1alpha1 "sigs.k8s.io/external-dns/apis/v1alpha1"
)

var (
	cfg        *rest.Config
	k8sClient  client.Client
	k8sManager ctrl.Manager
	testEnv    *envtest.Environment
	ctx        context.Context
)

func TestIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Integration Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	ctx = context.Background()

	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "config", "crd", "bases"),
			filepath.Join("..", "..", "config", "crd", "dependencies"),
		},
		ErrorIfCRDPathMissing: true,
	}

	var err error
	cfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	// Register schemes
	if err := clusterv1alpha1.AddToScheme(scheme.Scheme); err != nil {
		Expect(err).NotTo(HaveOccurred())
	}
	if err := routingv1alpha1.AddToScheme(scheme.Scheme); err != nil {
		Expect(err).NotTo(HaveOccurred())
	}
	if err := istioclientv1beta1.AddToScheme(scheme.Scheme); err != nil {
		Expect(err).NotTo(HaveOccurred())
	}
	if err := externaldnsv1alpha1.AddToScheme(scheme.Scheme); err != nil {
		Expect(err).NotTo(HaveOccurred())
	}

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	// Start controllers
	k8sManager, err = ctrl.NewManager(cfg, ctrl.Options{
		Scheme:  scheme.Scheme,
		Metrics: metricsserver.Options{BindAddress: "0"}, // Disable metrics server to avoid port conflicts in tests
	})
	Expect(err).ToNot(HaveOccurred())

	err = (&clustercontroller.DNSConfigurationReconciler{
		Client: k8sManager.GetClient(),
		Scheme: k8sManager.GetScheme(),
	}).SetupWithManager(k8sManager)
	Expect(err).ToNot(HaveOccurred())

	err = (&clustercontroller.ClusterIdentityReconciler{
		Client: k8sManager.GetClient(),
		Scheme: k8sManager.GetScheme(),
	}).SetupWithManager(k8sManager)
	Expect(err).ToNot(HaveOccurred())

	err = (&routingcontroller.GatewayReconciler{
		Client:                        k8sManager.GetClient(),
		Scheme:                        k8sManager.GetScheme(),
		DefaultRouterGatewayNamespace: "istio-system",
	}).SetupWithManager(k8sManager)
	Expect(err).ToNot(HaveOccurred())

	err = (&routingcontroller.DNSPolicyReconciler{
		Client: k8sManager.GetClient(),
		Scheme: k8sManager.GetScheme(),
	}).SetupWithManager(k8sManager)
	Expect(err).ToNot(HaveOccurred())

	err = (&routingcontroller.ServiceRouteReconciler{
		Client:                        k8sManager.GetClient(),
		Scheme:                        k8sManager.GetScheme(),
		DefaultRouterGatewayNamespace: "istio-system",
	}).SetupWithManager(k8sManager)
	Expect(err).ToNot(HaveOccurred())

	go func() {
		defer GinkgoRecover()
		err = k8sManager.Start(ctrl.SetupSignalHandler())
		Expect(err).ToNot(HaveOccurred(), "failed to run manager")
	}()

	// Create istio-system namespace for Gateway resources
	istioNs := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "istio-system",
		},
	}
	Expect(k8sClient.Create(ctx, istioNs)).To(Succeed())
})

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	err := testEnv.Stop()
	// Ignore timeout errors during shutdown as they are common with envtest
	if err != nil && !strings.Contains(err.Error(), "timeout waiting for process") {
		Expect(err).NotTo(HaveOccurred())
	}
})
