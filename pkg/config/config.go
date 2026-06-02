package config

import (
	goflag "flag"
	"time"

	"github.com/go-logr/logr"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	ctrlzap "sigs.k8s.io/controller-runtime/pkg/log/zap"
)

// Config holds all configuration for the ack-event-publisher controller.
// Field names and flag names mirror ackcfg.Config in aws-controllers-k8s/runtime.
type Config struct {
	EnableDevelopmentLogging bool
	LogLevel                 string
	WatchNamespace           string
	ResyncPeriod             time.Duration
	MetricsBindAddress       string
	HealthProbeBindAddress   string
	LeaderElect              bool
	LeaderElectionNamespace  string
	Kubeconfig               string
}

// BindFlags registers all configuration flags onto the provided FlagSet.
func (c *Config) BindFlags(fs *goflag.FlagSet) {
	fs.BoolVar(&c.EnableDevelopmentLogging, "enable-development-logging", false,
		"Enable development logging (zap console encoder, debug threshold).")
	fs.StringVar(&c.LogLevel, "log-level", "info",
		"Log verbosity level. One of: debug, info, warn, error.")
	fs.StringVar(&c.WatchNamespace, "watch-namespace", "",
		"Namespace to watch for ACK resources. Empty string watches all namespaces.")
	fs.DurationVar(&c.ResyncPeriod, "resync-period", 10*time.Minute,
		"How often to re-discover ACK CRDs and register informers for newly installed services.")
	fs.StringVar(&c.MetricsBindAddress, "metrics-bind-address", ":8080",
		"Address for the Prometheus metrics endpoint.")
	fs.StringVar(&c.HealthProbeBindAddress, "health-probe-bind-address", ":8081",
		"Address for the /healthz and /readyz endpoints.")
	fs.BoolVar(&c.LeaderElect, "leader-elect", false,
		"Enable leader election for high-availability deployments.")
	fs.StringVar(&c.LeaderElectionNamespace, "leader-election-namespace", "",
		"Namespace for the leader election lease object. Defaults to the controller namespace.")
	fs.StringVar(&c.Kubeconfig, "kubeconfig", "",
		"Path to a kubeconfig file. For out-of-cluster development only.")
}

// SetupLogger initialises zap logging, registers it with controller-runtime and
// klog, and returns the root logger. Mirrors ackcfg.Config.SetupLogger().
func (c *Config) SetupLogger() logr.Logger {
	lvl, err := zapcore.ParseLevel(c.LogLevel)
	if err != nil {
		lvl = zapcore.InfoLevel
	}

	opts := ctrlzap.Options{
		Development: c.EnableDevelopmentLogging,
		Level:       zap.NewAtomicLevelAt(lvl),
		TimeEncoder: zapcore.ISO8601TimeEncoder,
	}
	logger := ctrlzap.New(ctrlzap.UseFlagOptions(&opts))
	ctrl.SetLogger(logger)
	klog.SetLogger(logger)
	return logger
}
