package doctor

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var (
	certificateGVR = schema.GroupVersionResource{Group: "cert-manager.io", Version: "v1", Resource: "certificates"}
	applicationGVR = schema.GroupVersionResource{Group: "argoproj.io", Version: "v1alpha1", Resource: "applications"}
)

// AllChecks returns the core checks plus certificates, PDBs and Argo CD.
func AllChecks() []Check {
	return append(DefaultChecks(),
		Check{Name: "certificates", Run: checkCertificates},
		Check{Name: "pdbs", Run: checkPDBs},
		Check{Name: "argocd", Run: checkArgoCD},
	)
}

func (e *Env) certWarn() time.Duration {
	if e.CertWarn > 0 {
		return e.CertWarn
	}
	return 21 * 24 * time.Hour
}

func (e *Env) certFail() time.Duration {
	if e.CertFail > 0 {
		return e.CertFail
	}
	return 7 * 24 * time.Hour
}

func (e *Env) expirySeverity(notAfter time.Time) (Severity, string) {
	left := notAfter.Sub(e.now())
	days := int(left.Hours() / 24)
	switch {
	case left <= 0:
		return Fail, fmt.Sprintf("expired on %s", notAfter.UTC().Format(time.RFC3339))
	case left < e.certFail():
		return Fail, fmt.Sprintf("expires in %d days (%s)", days, notAfter.UTC().Format("2006-01-02"))
	case left < e.certWarn():
		return Warn, fmt.Sprintf("expires in %d days (%s)", days, notAfter.UTC().Format("2006-01-02"))
	default:
		return OK, fmt.Sprintf("valid for %d more days", days)
	}
}

// checkCertificates inspects cert-manager Certificates (Ready + notAfter)
// and every kubernetes.io/tls Secret's leaf certificate.
func checkCertificates(ctx context.Context, env *Env) ([]Finding, string, error) {
	var out []Finding
	certCount, soonest := 0, time.Time{}
	track := func(t time.Time) {
		if soonest.IsZero() || t.Before(soonest) {
			soonest = t
		}
	}

	if env.Dynamic != nil {
		list, err := env.Dynamic.Resource(certificateGVR).Namespace(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
		switch {
		case apierrors.IsNotFound(err):
			// cert-manager not installed; TLS secrets are still checked.
		case err != nil:
			return nil, "", fmt.Errorf("listing certificates: %w", err)
		default:
			for _, c := range list.Items {
				if !env.inScope(c.GetNamespace()) {
					continue
				}
				certCount++
				name := obj("certificate", c.GetName())
				if st, msg := readyCondition(c); st != "True" {
					out = append(out, Finding{Fail, c.GetNamespace(), name, "not Ready: " + msg})
				}
				if na, ok, _ := unstructured.NestedString(c.Object, "status", "notAfter"); ok {
					if t, err := time.Parse(time.RFC3339, na); err == nil {
						track(t)
						if sev, msg := env.expirySeverity(t); sev != OK {
							out = append(out, Finding{sev, c.GetNamespace(), name, msg})
						}
					}
				}
			}
		}
	}

	secrets, err := env.Client.CoreV1().Secrets(metav1.NamespaceAll).List(ctx, metav1.ListOptions{FieldSelector: "type=" + string(corev1.SecretTypeTLS)})
	if err != nil {
		return nil, "", fmt.Errorf("listing TLS secrets: %w", err)
	}
	for _, s := range secrets.Items {
		if s.Type != corev1.SecretTypeTLS || !env.inScope(s.Namespace) {
			continue
		}
		block, _ := pem.Decode(s.Data[corev1.TLSCertKey])
		if block == nil {
			out = append(out, Finding{Warn, s.Namespace, obj("secret", s.Name), "tls.crt is empty or not PEM"})
			continue
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			out = append(out, Finding{Warn, s.Namespace, obj("secret", s.Name), "cannot parse tls.crt: " + err.Error()})
			continue
		}
		certCount++
		track(cert.NotAfter)
		if sev, msg := env.expirySeverity(cert.NotAfter); sev != OK {
			out = append(out, Finding{sev, s.Namespace, obj("secret", s.Name), fmt.Sprintf("%s (CN=%s)", msg, cert.Subject.CommonName)})
		}
	}

	summary := fmt.Sprintf("%d certificates checked", certCount)
	if !soonest.IsZero() {
		summary += ", soonest expiry " + soonest.UTC().Format("2006-01-02")
	}
	return out, summary, nil
}

func checkPDBs(ctx context.Context, env *Env) ([]Finding, string, error) {
	pdbs, err := env.Client.PolicyV1().PodDisruptionBudgets(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, "", err
	}
	var out []Finding
	total, blocking := 0, 0
	for _, p := range pdbs.Items {
		if !env.inScope(p.Namespace) {
			continue
		}
		total++
		name := obj("pdb", p.Name)
		if p.Status.ExpectedPods == 0 {
			out = append(out, Finding{Warn, p.Namespace, name, "selector matches no pods"})
			continue
		}
		if p.Status.DisruptionsAllowed == 0 {
			blocking++
			sev := Warn
			if p.Status.CurrentHealthy < p.Status.DesiredHealthy {
				sev = Fail
			}
			out = append(out, Finding{sev, p.Namespace, name, fmt.Sprintf(
				"0 disruptions allowed (healthy %d/%d, expected %d): node drains will block",
				p.Status.CurrentHealthy, p.Status.DesiredHealthy, p.Status.ExpectedPods)})
		}
	}
	return out, fmt.Sprintf("%d PDBs, %d blocking drains", total, blocking), nil
}

func checkArgoCD(ctx context.Context, env *Env) ([]Finding, string, error) {
	if env.Dynamic == nil {
		return nil, "skipped (no dynamic client)", nil
	}
	list, err := env.Dynamic.Resource(applicationGVR).Namespace(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
	if apierrors.IsNotFound(err) {
		return nil, "Argo CD not installed", nil
	}
	if err != nil {
		return nil, "", fmt.Errorf("listing applications: %w", err)
	}
	var out []Finding
	good := 0
	for _, a := range list.Items {
		name := obj("application", a.GetName())
		sync, _, _ := unstructured.NestedString(a.Object, "status", "sync", "status")
		health, _, _ := unstructured.NestedString(a.Object, "status", "health", "status")
		op, _, _ := unstructured.NestedString(a.Object, "status", "operationState", "phase")
		before := len(out)
		switch health {
		case "Healthy":
		case "Progressing", "Suspended":
			out = append(out, Finding{Warn, a.GetNamespace(), name, "health " + health})
		case "":
			out = append(out, Finding{Warn, a.GetNamespace(), name, "health unknown"})
		default: // Degraded, Missing, Unknown
			out = append(out, Finding{Fail, a.GetNamespace(), name, "health " + health})
		}
		if sync != "Synced" {
			out = append(out, Finding{Warn, a.GetNamespace(), name, "sync " + firstNonEmpty(sync, "Unknown")})
		}
		if op == "Failed" || op == "Error" {
			msg, _, _ := unstructured.NestedString(a.Object, "status", "operationState", "message")
			out = append(out, Finding{Fail, a.GetNamespace(), name, "last sync " + op + ": " + msg})
		}
		if len(out) == before {
			good++
		}
	}
	return out, fmt.Sprintf("%d/%d applications Synced and Healthy", good, len(list.Items)), nil
}

func readyCondition(u unstructured.Unstructured) (status, message string) {
	conds, _, _ := unstructured.NestedSlice(u.Object, "status", "conditions")
	for _, c := range conds {
		m, ok := c.(map[string]interface{})
		if !ok || m["type"] != "Ready" {
			continue
		}
		s, _ := m["status"].(string)
		msg, _ := m["message"].(string)
		return s, msg
	}
	return "Unknown", "no Ready condition"
}
