package main

import (
	goflag "flag"
	"fmt"
	"os"

	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"github.com/aws-controllers-k8s/ack-event-publisher/pkg/config"
	"github.com/aws-controllers-k8s/ack-event-publisher/pkg/discovery"
	"github.com/aws-controllers-k8s/ack-event-publisher/pkg/handler"
	"github.com/aws-controllers-k8s/ack-event-publisher/pkg/informer"
	"github.com/aws-controllers-k8s/ack-event-publisher/pkg/version"
)

func main() {
	cfg := &config.Config{}
	cfg.BindFlags(goflag.CommandLine)
	goflag.Parse()

	log := cfg.SetupLogger()
	log.Info("starting ack-event-publisher",
		"version", version.GitVersion,
		"commit", version.GitCommit,
		"buildDate", version.BuildDate,
		"watchNamespace", cfg.WatchNamespace,
		"logLevel", cfg.LogLevel,
	)

	restCfg, err := buildRestConfig(cfg.Kubeconfig)
	if err != nil {
		log.Error(err, "failed to build REST config")
		os.Exit(1)
	}

	dynClient, err := dynamic.NewForConfig(restCfg)
	if err != nil {
		log.Error(err, "failed to create dynamic client")
		os.Exit(1)
	}

	apiextClient, err := apiextensionsclient.NewForConfig(restCfg)
	if err != nil {
		log.Error(err, "failed to create apiextensions client")
		os.Exit(1)
	}

	kubeClient, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		log.Error(err, "failed to create kubernetes client")
		os.Exit(1)
	}

	mgr, err := ctrl.NewManager(restCfg, ctrl.Options{
		Metrics: metricsserver.Options{
			BindAddress: cfg.MetricsBindAddress,
		},
		HealthProbeBindAddress:  cfg.HealthProbeBindAddress,
		LeaderElection:          cfg.LeaderElect,
		LeaderElectionID:        "ack-event-publisher.services.k8s.aws",
		LeaderElectionNamespace: cfg.LeaderElectionNamespace,
		// No scheme registration needed: we use dynamic clients only.
	})
	if err != nil {
		log.Error(err, "failed to create controller-runtime manager")
		os.Exit(1)
	}

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		log.Error(err, "failed to register healthz check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		log.Error(err, "failed to register readyz check")
		os.Exit(1)
	}

	h := handler.New(log, kubeClient)
	disc := discovery.New(log, apiextClient)
	infMgr := informer.NewManager(log, dynClient, cfg.WatchNamespace, cfg.ResyncPeriod, h)

	if err := mgr.Add(infMgr.NewRunnable(disc, cfg.ResyncPeriod)); err != nil {
		log.Error(err, "failed to register informer runnable with manager")
		os.Exit(1)
	}

	log.Info("manager initialised, waiting for leader election and informer sync")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		log.Error(err, "manager exited with error")
		os.Exit(1)
	}
}

func buildRestConfig(kubeconfig string) (*rest.Config, error) {
	if kubeconfig != "" {
		return clientcmd.BuildConfigFromFlags("", kubeconfig)
	}
	cfg, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("in-cluster config unavailable (use --kubeconfig for local dev): %w", err)
	}
	return cfg, nil
}
