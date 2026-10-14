package doctor

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynfake "k8s.io/client-go/dynamic/fake"
)

func selfSigned(t *testing.T, cn string, notAfter time.Time) []byte {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: cn},
		NotBefore:    notAfter.Add(-90 * 24 * time.Hour),
		NotAfter:     notAfter,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}

func tlsSecret(ns, name string, crt []byte) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: name},
		Type:       corev1.SecretTypeTLS,
		Data:       map[string][]byte{corev1.TLSCertKey: crt},
	}
}

func newDynamic(objs ...runtime.Object) *dynfake.FakeDynamicClient {
	lists := map[schema.GroupVersionResource]string{
		certificateGVR: "CertificateList",
		applicationGVR: "ApplicationList",
	}
	return dynfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), lists, objs...)
}

func u(apiVersion, kind, ns, name string, status map[string]interface{}) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": apiVersion,
		"kind":       kind,
		"metadata":   map[string]interface{}{"namespace": ns, "name": name},
		"status":     status,
	}}
}

func runOne(t *testing.T, env *Env, name string) Result {
	t.Helper()
	for _, r := range Run(context.Background(), env, AllChecks()) {
		if r.Check == name {
			return r
		}
	}
	t.Fatalf("check %s not run", name)
	return Result{}
}

func TestCertificates(t *testing.T) {
	env := newEnv(
		tlsSecret("satark", "fresh", selfSigned(t, "api.satark.example", now.Add(60*24*time.Hour))),
		tlsSecret("satark", "soon", selfSigned(t, "bot.satark.example", now.Add(10*24*time.Hour))),
		tlsSecret("satark", "expired", selfSigned(t, "old.satark.example", now.Add(-time.Hour))),
	)
	env.Dynamic = newDynamic(
		u("cert-manager.io/v1", "Certificate", "satark", "api", map[string]interface{}{
			"notAfter":   now.Add(3 * 24 * time.Hour).Format(time.RFC3339),
			"conditions": []interface{}{map[string]interface{}{"type": "Ready", "status": "False", "message": "ACME order failed"}},
		}),
	)
	r := runOne(t, env, "certificates")
	if r.Status != Fail {
		t.Fatalf("want fail, got %+v", r)
	}
	sev := map[string]Severity{}
	for _, f := range r.Findings {
		if cur, ok := sev[f.Object]; !ok || f.Severity.rank() > cur.rank() {
			sev[f.Object] = f.Severity
		}
	}
	want := map[string]Severity{"secret/soon": Warn, "secret/expired": Fail, "certificate/api": Fail}
	for k, v := range want {
		if sev[k] != v {
			t.Errorf("%s: want %s, got %q (findings %+v)", k, v, sev[k], r.Findings)
		}
	}
	if _, ok := sev["secret/fresh"]; ok {
		t.Error("fresh certificate should not be reported")
	}
}

func TestPDBs(t *testing.T) {
	env := newEnv(
		&policyv1.PodDisruptionBudget{ObjectMeta: metav1.ObjectMeta{Namespace: "satark", Name: "ok"},
			Status: policyv1.PodDisruptionBudgetStatus{DisruptionsAllowed: 1, CurrentHealthy: 3, DesiredHealthy: 2, ExpectedPods: 3}},
		&policyv1.PodDisruptionBudget{ObjectMeta: metav1.ObjectMeta{Namespace: "satark", Name: "tight"},
			Status: policyv1.PodDisruptionBudgetStatus{DisruptionsAllowed: 0, CurrentHealthy: 1, DesiredHealthy: 1, ExpectedPods: 1}},
		&policyv1.PodDisruptionBudget{ObjectMeta: metav1.ObjectMeta{Namespace: "satark", Name: "orphan"}},
	)
	r := runOne(t, env, "pdbs")
	if r.Status != Warn || r.Summary != "3 PDBs, 1 blocking drains" || len(r.Findings) != 2 {
		t.Fatalf("unexpected: %+v", r)
	}
}

func TestArgoCD(t *testing.T) {
	env := newEnv()
	env.Dynamic = newDynamic(
		u("argoproj.io/v1alpha1", "Application", "argocd", "gateway-api", map[string]interface{}{
			"sync": map[string]interface{}{"status": "Synced"}, "health": map[string]interface{}{"status": "Healthy"},
		}),
		u("argoproj.io/v1alpha1", "Application", "argocd", "url-intel", map[string]interface{}{
			"sync": map[string]interface{}{"status": "OutOfSync"}, "health": map[string]interface{}{"status": "Degraded"},
			"operationState": map[string]interface{}{"phase": "Failed", "message": "one or more objects failed to apply"},
		}),
	)
	r := runOne(t, env, "argocd")
	if r.Status != Fail || r.Summary != "1/2 applications Synced and Healthy" || len(r.Findings) != 3 {
		t.Fatalf("unexpected: %+v", r)
	}
}

func TestJSONOutput(t *testing.T) {
	env := newEnv(node("cp1", corev1.ConditionTrue))
	env.Dynamic = newDynamic()
	var buf bytes.Buffer
	if err := RenderJSON(&buf, Run(context.Background(), env, AllChecks()), "2026-10-01T12:00:00Z"); err != nil {
		t.Fatal(err)
	}
	var rep Report
	if err := json.Unmarshal(buf.Bytes(), &rep); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, buf.String())
	}
	if rep.Overall != OK || len(rep.Results) != 7 || rep.CheckedAt == "" {
		t.Fatalf("unexpected report: %+v", rep)
	}
}
