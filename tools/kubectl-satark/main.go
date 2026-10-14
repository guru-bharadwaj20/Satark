// kubectl-satark is a kubectl plugin with operational helpers for the Satark
// platform. Install the binary on PATH and run `kubectl satark doctor`.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/guru-bharadwaj20/satark/tools/kubectl-satark/internal/doctor"
)

var version = "dev"

const usage = `kubectl satark - operational helpers for Satark

Usage:
  kubectl satark doctor [flags]   Run cluster health checks
  kubectl satark version          Print plugin version

Run 'kubectl satark doctor -h' for doctor flags.
`

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return 2
	}
	switch args[0] {
	case "doctor":
		return runDoctor(args[1:], stdout, stderr)
	case "version", "--version":
		fmt.Fprintln(stdout, version)
		return 0
	case "-h", "--help", "help":
		fmt.Fprint(stdout, usage)
		return 0
	default:
		fmt.Fprintf(stderr, "unknown command %q\n\n%s", args[0], usage)
		return 2
	}
}

type doctorOpts struct {
	kubeconfig   string
	context      string
	namespaces   string
	pendingGrace time.Duration
	restartWarn  int
	timeout      time.Duration
	verbose      bool
}

func parseDoctorFlags(args []string, stderr io.Writer) (*doctorOpts, error) {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	fs.SetOutput(stderr)
	o := &doctorOpts{}
	fs.StringVar(&o.kubeconfig, "kubeconfig", "", "path to kubeconfig (default: $KUBECONFIG or ~/.kube/config)")
	fs.StringVar(&o.context, "context", "", "kubeconfig context to use")
	fs.StringVar(&o.namespaces, "n", "", "comma-separated namespaces to check (default: all)")
	fs.DurationVar(&o.pendingGrace, "pending-grace", 5*time.Minute, "how long a pod or PVC may stay Pending before it is a failure")
	fs.IntVar(&o.restartWarn, "restart-warn", 5, "container restart count that triggers a warning (0 disables)")
	fs.DurationVar(&o.timeout, "timeout", 30*time.Second, "overall timeout")
	fs.BoolVar(&o.verbose, "v", false, "show OK findings too")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	return o, nil
}

func restConfig(o *doctorOpts) (*rest.Config, error) {
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	if o.kubeconfig != "" {
		rules.ExplicitPath = o.kubeconfig
	}
	overrides := &clientcmd.ConfigOverrides{CurrentContext: o.context}
	return clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, overrides).ClientConfig()
}

func runDoctor(args []string, stdout, stderr io.Writer) int {
	o, err := parseDoctorFlags(args, stderr)
	if err != nil {
		return 2
	}
	cfg, err := restConfig(o)
	if err != nil {
		fmt.Fprintf(stderr, "error: loading kubeconfig: %v\n", err)
		return 3
	}
	cfg.UserAgent = "kubectl-satark/" + version
	client, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 3
	}
	env := &doctor.Env{
		Client:       client,
		Namespaces:   splitList(o.namespaces),
		PendingGrace: o.pendingGrace,
		RestartWarn:  int32(o.restartWarn),
	}
	ctx, cancel := context.WithTimeout(context.Background(), o.timeout)
	defer cancel()

	results := doctor.Run(ctx, env, doctor.DefaultChecks())
	doctor.RenderText(stdout, results, o.verbose)
	return exitCode(doctor.Worst(results))
}

// exitCode: 0 healthy, 1 warnings only, 2 failures.
func exitCode(s doctor.Severity) int {
	switch s {
	case doctor.Fail:
		return 2
	case doctor.Warn:
		return 1
	default:
		return 0
	}
}

func splitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
