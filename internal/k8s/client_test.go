package k8s

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAvailableContextsMergesKubeconfigPathList(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "first.yaml")
	second := filepath.Join(dir, "second.yaml")

	if err := os.WriteFile(first, []byte(`
apiVersion: v1
kind: Config
clusters:
- name: cluster-a
  cluster:
    server: https://cluster-a.example
contexts:
- name: ctx-a
  context:
    cluster: cluster-a
    user: user-a
    namespace: team-a
current-context: ctx-a
users:
- name: user-a
  user:
    token: token-a
`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(second, []byte(`
apiVersion: v1
kind: Config
clusters:
- name: cluster-b
  cluster:
    server: https://cluster-b.example
contexts:
- name: ctx-b
  context:
    cluster: cluster-b
    user: user-b
users:
- name: user-b
  user:
    token: token-b
`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KUBECONFIG", first+string(os.PathListSeparator)+second)

	got, err := AvailableContexts(KubeconfigSource{})
	if err != nil {
		t.Fatal(err)
	}
	if got.Current != "ctx-a" {
		t.Fatalf("current context = %q, want ctx-a", got.Current)
	}
	if len(got.Contexts) != 2 {
		t.Fatalf("contexts len = %d, want 2: %#v", len(got.Contexts), got.Contexts)
	}
	if got.Contexts[0].Name != "ctx-a" || got.Contexts[0].Cluster != "cluster-a" || got.Contexts[0].Namespace != "team-a" {
		t.Fatalf("ctx-a mismatch: %#v", got.Contexts[0])
	}
	if got.Contexts[1].Name != "ctx-b" || got.Contexts[1].Cluster != "cluster-b" || got.Contexts[1].Namespace != "" {
		t.Fatalf("ctx-b mismatch: %#v", got.Contexts[1])
	}
}

const singleContextKubeconfig = `
apiVersion: v1
kind: Config
clusters:
- name: cluster-x
  cluster:
    server: https://cluster-x.example
contexts:
- name: ctx-x
  context:
    cluster: cluster-x
    user: user-x
    namespace: team-x
current-context: ctx-x
users:
- name: user-x
  user:
    token: token-x
`

func TestAvailableContextsRawSource(t *testing.T) {
	// A raw source must ignore KUBECONFIG entirely and read only the inline YAML.
	t.Setenv("KUBECONFIG", filepath.Join(t.TempDir(), "does-not-exist.yaml"))
	got, err := AvailableContexts(KubeconfigSource{Raw: singleContextKubeconfig})
	if err != nil {
		t.Fatal(err)
	}
	if got.Current != "ctx-x" || len(got.Contexts) != 1 || got.Contexts[0].Name != "ctx-x" {
		t.Fatalf("raw source contexts mismatch: %#v", got)
	}
}

func TestAvailableContextsPathSource(t *testing.T) {
	path := filepath.Join(t.TempDir(), "explicit.yaml")
	if err := os.WriteFile(path, []byte(singleContextKubeconfig), 0o600); err != nil {
		t.Fatal(err)
	}
	// Point KUBECONFIG somewhere else to prove the explicit path wins.
	t.Setenv("KUBECONFIG", filepath.Join(t.TempDir(), "other.yaml"))
	got, err := AvailableContexts(KubeconfigSource{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	if got.Current != "ctx-x" || len(got.Contexts) != 1 || got.Contexts[0].Namespace != "team-x" {
		t.Fatalf("path source contexts mismatch: %#v", got)
	}
}

func TestAvailableContextsRawInvalid(t *testing.T) {
	if _, err := AvailableContexts(KubeconfigSource{Raw: "\tnot: [valid"}); err == nil {
		t.Fatal("expected an error for malformed raw kubeconfig")
	}
}
