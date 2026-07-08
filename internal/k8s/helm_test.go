package k8s

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// A minimal but realistic Helm 3 release payload (the JSON Helm stores, gzipped
// then base64'd, inside secret.Data["release"]).
const sampleReleaseJSON = `{
  "name": "carshowdb",
  "namespace": "carshowdb-dev-api",
  "version": 389,
  "info": {"status": "deployed", "last_deployed": "2026-07-02T20:53:28Z",
           "notes": "app is up", "description": "Upgrade complete"},
  "chart": {"metadata": {"name": "app", "version": "0.1.0+91b27b47", "appVersion": "0.1.0"}},
  "config": {"replicas": 2},
  "manifest": "---\napiVersion: v1\nkind: Service\n"
}`

// gzipBase64 mirrors exactly what Helm writes to the secret: base64(gzip(json)).
func gzipBase64(t *testing.T, s string) []byte {
	t.Helper()
	var gz bytes.Buffer
	w := gzip.NewWriter(&gz)
	if _, err := w.Write([]byte(s)); err != nil {
		t.Fatalf("gzip write: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("gzip close: %v", err)
	}
	return []byte(base64.StdEncoding.EncodeToString(gz.Bytes()))
}

// gzipOnly is the raw-gzip variant (no extra base64 layer) the fallback handles.
func gzipOnly(t *testing.T, s string) []byte {
	t.Helper()
	var gz bytes.Buffer
	w := gzip.NewWriter(&gz)
	if _, err := w.Write([]byte(s)); err != nil {
		t.Fatalf("gzip write: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("gzip close: %v", err)
	}
	return gz.Bytes()
}

func TestDecodeHelmRelease_Base64Gzip(t *testing.T) {
	rel, err := decodeHelmRelease(gzipBase64(t, sampleReleaseJSON))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if rel.Name != "carshowdb" || rel.Version != 389 {
		t.Errorf("name/version = %q/%d, want carshowdb/389", rel.Name, rel.Version)
	}
	if rel.Info.Status != "deployed" {
		t.Errorf("status = %q, want deployed", rel.Info.Status)
	}
	if rel.Chart.Metadata.Name != "app" || rel.Chart.Metadata.Version != "0.1.0+91b27b47" {
		t.Errorf("chart = %q@%q, want app@0.1.0+91b27b47",
			rel.Chart.Metadata.Name, rel.Chart.Metadata.Version)
	}
}

func TestDecodeHelmRelease_RawGzipFallback(t *testing.T) {
	rel, err := decodeHelmRelease(gzipOnly(t, sampleReleaseJSON))
	if err != nil {
		t.Fatalf("raw-gzip fallback decode: %v", err)
	}
	if rel.Name != "carshowdb" {
		t.Errorf("name = %q, want carshowdb", rel.Name)
	}
}

func TestDecodeHelmRelease_Garbage(t *testing.T) {
	if _, err := decodeHelmRelease([]byte("not a helm release")); err == nil {
		t.Error("expected an error decoding garbage, got nil")
	}
}

func helmSecret(ns, name string, data []byte) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: name},
		Type:       helmSecretType,
		Data:       map[string][]byte{"release": data},
	}
}

func TestMapHelmReleases_LatestRevisionAndSkips(t *testing.T) {
	v1 := `{"name":"web","namespace":"prod","version":1,"info":{"status":"superseded"},"chart":{"metadata":{"name":"web","version":"1.0"}}}`
	v2 := `{"name":"web","namespace":"prod","version":2,"info":{"status":"deployed"},"chart":{"metadata":{"name":"web","version":"1.1"}}}`

	secrets := []*corev1.Secret{
		helmSecret("prod", "sh.helm.release.v1.web.v1", gzipBase64(t, v1)),
		helmSecret("prod", "sh.helm.release.v1.web.v2", gzipBase64(t, v2)),
		// undecodable helm secret — must be skipped, not fatal
		helmSecret("prod", "sh.helm.release.v1.broken.v1", []byte("garbage")),
		// a non-helm secret — must be ignored
		{ObjectMeta: metav1.ObjectMeta{Namespace: "prod", Name: "tls"}, Type: corev1.SecretTypeTLS},
	}

	got := MapHelmReleases(secrets)
	if len(got) != 1 {
		t.Fatalf("got %d releases, want 1 (latest of web, others skipped): %+v", len(got), got)
	}
	r := got[0]
	if r.Name != "web" || r.Namespace != "prod" {
		t.Errorf("release id = %s/%s, want prod/web", r.Namespace, r.Name)
	}
	if r.Revision != 2 || r.Status != "deployed" || r.ChartVersion != "1.1" {
		t.Errorf("collapsed to rev=%d status=%q chart=%q, want rev=2 deployed 1.1",
			r.Revision, r.Status, r.ChartVersion)
	}
}
