// Package doctor runs read-only health checks against a Kubernetes cluster
// and reports findings grouped by check.
package doctor

import (
	"context"
	"fmt"
	"sort"
	"time"

	"k8s.io/client-go/kubernetes"
)

// Severity of a finding.
type Severity string

const (
	OK   Severity = "ok"
	Warn Severity = "warn"
	Fail Severity = "fail"
)

func (s Severity) rank() int {
	switch s {
	case Fail:
		return 2
	case Warn:
		return 1
	default:
		return 0
	}
}

// Finding is one problem (or confirmation) reported by a check.
type Finding struct {
	Severity  Severity `json:"severity"`
	Namespace string   `json:"namespace,omitempty"`
	Object    string   `json:"object,omitempty"`
	Message   string   `json:"message"`
}

// Result is the outcome of one check.
type Result struct {
	Check    string    `json:"check"`
	Status   Severity  `json:"status"`
	Summary  string    `json:"summary"`
	Findings []Finding `json:"findings,omitempty"`
	Error    string    `json:"error,omitempty"`
}

// Env carries the clients and options shared by all checks.
type Env struct {
	Client     kubernetes.Interface
	Namespaces []string // empty means all namespaces
	Now        func() time.Time
	// PendingGrace is how long a pod may be Pending before it is reported.
	PendingGrace time.Duration
	// RestartWarn is the restart count at which a container is flagged.
	RestartWarn int32
}

func (e *Env) now() time.Time {
	if e.Now != nil {
		return e.Now()
	}
	return time.Now()
}

func (e *Env) inScope(ns string) bool {
	if len(e.Namespaces) == 0 {
		return true
	}
	for _, n := range e.Namespaces {
		if n == ns {
			return true
		}
	}
	return false
}

// Check is a single named health check.
type Check struct {
	Name string
	Run  func(ctx context.Context, env *Env) ([]Finding, string, error)
}

// DefaultChecks returns the core checks in display order.
func DefaultChecks() []Check {
	return []Check{
		{Name: "nodes", Run: checkNodes},
		{Name: "pods", Run: checkPods},
		{Name: "pvcs", Run: checkPVCs},
		{Name: "deployments", Run: checkDeployments},
	}
}

// Run executes the checks and returns one Result per check.
func Run(ctx context.Context, env *Env, checks []Check) []Result {
	results := make([]Result, 0, len(checks))
	for _, c := range checks {
		findings, summary, err := c.Run(ctx, env)
		r := Result{Check: c.Name, Status: OK, Summary: summary}
		if err != nil {
			r.Status = Fail
			r.Error = err.Error()
			if r.Summary == "" {
				r.Summary = "check could not run"
			}
		}
		sort.SliceStable(findings, func(i, j int) bool {
			return findings[i].Severity.rank() > findings[j].Severity.rank()
		})
		for _, f := range findings {
			if f.Severity.rank() > r.Status.rank() {
				r.Status = f.Severity
			}
		}
		r.Findings = findings
		results = append(results, r)
	}
	return results
}

// Worst returns the most severe status across results.
func Worst(results []Result) Severity {
	worst := OK
	for _, r := range results {
		if r.Status.rank() > worst.rank() {
			worst = r.Status
		}
	}
	return worst
}

func obj(kind, name string) string { return fmt.Sprintf("%s/%s", kind, name) }
