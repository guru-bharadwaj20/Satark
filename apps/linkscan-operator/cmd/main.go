/*
Copyright 2026 The Satark Authors.

Licensed under the MIT License. See LICENSE in the repository root.
*/

package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	scanv1alpha1 "github.com/guru-bharadwaj20/satark/apps/linkscan-operator/api/v1alpha1"
	"github.com/guru-bharadwaj20/satark/apps/linkscan-operator/internal/controller"
	"github.com/guru-bharadwaj20/satark/apps/linkscan-operator/internal/natsbridge"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(scanv1alpha1.AddToScheme(scheme))
}

func main() {
	var metricsAddr, probeAddr string
	var enableLeaderElection bool
	var sandbox controller.SandboxConfig
	var dnsServers string
	var natsURL, scanNamespace string
	var maxActive int
	var ensureStreams bool
	flag.StringVar(&natsURL, "nats-url", os.Getenv("NATS_URL"), "NATS URL for scan jobs and results (empty disables the NATS bridge).")
	flag.StringVar(&scanNamespace, "scan-namespace", envOr("SCAN_NAMESPACE", "satark-sandbox"), "Namespace for LinkScans created from NATS jobs.")
	flag.IntVar(&maxActive, "max-active-scans", 5, "Unfinished scans one replica may run at a time.")
	flag.BoolVar(&ensureStreams, "nats-ensure-streams", false, "Create JetStream streams and the consumer if missing (local development).")
	flag.StringVar(&sandbox.Image, "sandbox-image", envOr("SANDBOX_IMAGE", "ghcr.io/guru-bharadwaj20/satark/sandbox-browser:dev"), "Default sandbox-browser image.")
	flag.StringVar(&sandbox.RuntimeClassName, "sandbox-runtime-class", envOr("SANDBOX_RUNTIME_CLASS", "gvisor"), "RuntimeClass for sandbox pods (empty for the cluster default).")
	flag.StringVar(&sandbox.ScreenshotBucket, "screenshot-bucket", os.Getenv("SCREENSHOT_BUCKET"), "S3 bucket for sandbox screenshots (empty disables upload).")
	flag.StringVar(&sandbox.S3SecretName, "screenshot-secret", envOr("SCREENSHOT_SECRET", "sandbox-s3"), "Secret with AWS credentials for screenshot upload.")
	flag.StringVar(&dnsServers, "sandbox-dns", envOr("SANDBOX_DNS", "1.1.1.1,9.9.9.9"), "Comma-separated public resolvers for sandbox pods.")
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metrics endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false, "Enable leader election for the controller manager.")
	opts := zap.Options{Development: false}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))
	for _, d := range strings.Split(dnsServers, ",") {
		if d = strings.TrimSpace(d); d != "" {
			sandbox.DNSServers = append(sandbox.DNSServers, d)
		}
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		Metrics:                metricsserver.Options{BindAddress: metricsAddr},
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "linkscan-operator.satark.io",
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	reconciler := &controller.LinkScanReconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		Sandbox:  sandbox,
		Recorder: mgr.GetEventRecorderFor("linkscan-operator"),
	}
	if natsURL != "" {
		nc, err := nats.Connect(natsURL, nats.Name("linkscan-operator"), nats.MaxReconnects(-1), nats.ReconnectWait(2*time.Second))
		if err != nil {
			setupLog.Error(err, "unable to connect to NATS")
			os.Exit(1)
		}
		defer nc.Drain() //nolint:errcheck
		js, err := jetstream.New(nc)
		if err != nil {
			setupLog.Error(err, "unable to open JetStream")
			os.Exit(1)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		if ensureStreams {
			if err := natsbridge.EnsureStreams(ctx, js); err != nil {
				setupLog.Error(err, "unable to declare streams")
				os.Exit(1)
			}
		}
		consumer, err := js.Consumer(ctx, natsbridge.JobsStream, natsbridge.ConsumerName)
		cancel()
		if err != nil {
			setupLog.Error(err, "unable to find JetStream consumer", "stream", natsbridge.JobsStream, "consumer", natsbridge.ConsumerName)
			os.Exit(1)
		}
		reconciler.Results = &natsbridge.ResultPublisher{JS: js}
		if err := mgr.Add(&natsbridge.Intake{
			Client:    mgr.GetClient(),
			Consumer:  consumer,
			Namespace: scanNamespace,
			Replica:   envOr("POD_NAME", "local"),
			MaxActive: maxActive,
			Log:       ctrl.Log.WithName("intake"),
		}); err != nil {
			setupLog.Error(err, "unable to add intake")
			os.Exit(1)
		}
		if err := mgr.AddReadyzCheck("nats", func(_ *http.Request) error {
			if !nc.IsConnected() {
				return fmt.Errorf("nats disconnected: %s", nc.Status())
			}
			return nil
		}); err != nil {
			setupLog.Error(err, "unable to set up nats check")
			os.Exit(1)
		}
	}
	if err := reconciler.SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "LinkScan")
		os.Exit(1)
	}
	// +kubebuilder:scaffold:builder

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

func envOr(key, def string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return def
}
