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

	got, err := AvailableContexts()
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
