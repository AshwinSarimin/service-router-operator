package main

import (
    "context"
    "flag"
    "os"

    "sigs.k8s.io/controller-runtime/pkg/client/config"
    "sigs.k8s.io/controller-runtime/pkg/manager"
    "sigs.k8s.io/controller-runtime/pkg/log/zap"
    "sigs.k8s.io/controller-runtime/pkg/manager/signals"

    "service-router-operator/internal/controller"
)

func main() {
    // Set up logging
    logger := zap.New(zap.UseDevMode(true))
    defer logger.Sync()

    // Parse command line flags
    var metricsAddr string
    flag.StringVar(&metricsAddr, "metrics-addr", ":8080", "The address the metric endpoint binds to.")
    flag.Parse()

    // Create a new manager
    cfg, err := config.GetConfig()
    if err != nil {
        logger.Error(err, "unable to get kubeconfig")
        os.Exit(1)
    }

    mgr, err := manager.New(cfg, manager.Options{
        MetricsBindAddress: metricsAddr,
        Logger:             logger,
    })
    if err != nil {
        logger.Error(err, "unable to set up overall controller manager")
        os.Exit(1)
    }

    // Setup the ServiceRouter controller
    if err = controller.SetupServiceRouterController(mgr); err != nil {
        logger.Error(err, "unable to create controller", "controller", "ServiceRouter")
        os.Exit(1)
    }

    // Start the manager
    logger.Info("starting the manager")
    if err := mgr.Start(signals.SetupSignalHandler()); err != nil {
        logger.Error(err, "problem running manager")
        os.Exit(1)
    }
}