package k8s

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os/exec"
	"sort"
	"strconv"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	sigsyaml "sigs.k8s.io/yaml"

	"github.com/yscale-sh/kubagachi/internal/state"
	"github.com/yscale-sh/kubagachi/internal/tui"
)

const helmSecretType = "helm.sh/release.v1"

// helmReleaseJSON is the structure decoded from the release secret's payload.
// Helm stores: base64(gzip(json)) inside secret.Data["release"]; client-go
// base64-decodes Data once, so we see base64(gzip(json)) and must decode again.
type helmReleaseJSON struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Version   int    `json:"version"`
	Info      struct {
		Status       string `json:"status"`
		LastDeployed string `json:"last_deployed"` // RFC3339
		Notes        string `json:"notes"`
		Description  string `json:"description,omitempty"`
	} `json:"info"`
	Chart struct {
		Metadata struct {
			Name       string `json:"name"`
			Version    string `json:"version"`
			AppVersion string `json:"appVersion"`
		} `json:"metadata"`
	} `json:"chart"`
	Config   map[string]interface{} `json:"config"`
	Manifest string                 `json:"manifest"`
}

// decodeHelmRelease decodes the payload from secret.Data["release"].
// Helm encodes: base64(gzip(json)). client-go base64-decodes Data once,
// leaving base64(gzip(json)), so we must decode base64 again then gzip-decompress.
// Fallback: if base64 fails, try gzip directly (raw gzip case).
func decodeHelmRelease(raw []byte) (*helmReleaseJSON, error) {
	if b64, err := helmBase64Decode(raw); err == nil {
		if rel, err := helmUngzipJSON(b64); err == nil {
			return rel, nil
		}
	}
	// Fallback: payload may already be raw gzip without the extra base64 layer.
	if rel, err := helmUngzipJSON(raw); err == nil {
		return rel, nil
	}
	return nil, fmt.Errorf("helm: could not decode release payload")
}

func helmBase64Decode(src []byte) ([]byte, error) {
	dst := make([]byte, base64.StdEncoding.DecodedLen(len(src)))
	n, err := base64.StdEncoding.Decode(dst, bytes.TrimSpace(src))
	if err != nil {
		return nil, err
	}
	return dst[:n], nil
}

func helmUngzipJSON(data []byte) (*helmReleaseJSON, error) {
	gr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer gr.Close()
	payload, err := io.ReadAll(io.LimitReader(gr, 16<<20)) // 16 MiB cap
	if err != nil {
		return nil, err
	}
	var rel helmReleaseJSON
	if err := json.Unmarshal(payload, &rel); err != nil {
		return nil, err
	}
	return &rel, nil
}

// MapHelmReleases inspects a Secret slice, decodes any helm.sh/release.v1
// entries, and returns the latest revision per (namespace, name).
// Undecodable secrets are skipped with a log line — they never fail the snapshot.
func MapHelmReleases(secrets []*corev1.Secret) []state.HelmReleaseView {
	type key struct{ ns, name string }
	type entry struct {
		rel       *helmReleaseJSON
		ageSec    int64
		updatedAge int64
	}
	best := map[key]entry{}

	for _, s := range secrets {
		if s.Type != helmSecretType {
			continue
		}
		raw, ok := s.Data["release"]
		if !ok || len(raw) == 0 {
			continue
		}
		rel, err := decodeHelmRelease(raw)
		if err != nil {
			log.Printf("kubagachi: helm decode %s/%s: %v", s.Namespace, s.Name, err)
			continue
		}
		k := key{rel.Namespace, rel.Name}
		if e, exists := best[k]; exists && rel.Version <= e.rel.Version {
			continue
		}
		var ageSec int64
		if !s.CreationTimestamp.IsZero() {
			ageSec = int64(time.Since(s.CreationTimestamp.Time).Seconds())
		}
		var updatedAge int64
		if t, err := time.Parse(time.RFC3339, rel.Info.LastDeployed); err == nil {
			updatedAge = int64(time.Since(t).Seconds())
		}
		best[k] = entry{rel: rel, ageSec: ageSec, updatedAge: updatedAge}
	}

	out := make([]state.HelmReleaseView, 0, len(best))
	for k, e := range best {
		out = append(out, state.HelmReleaseView{
			Name:          k.name,
			Namespace:     k.ns,
			Chart:         e.rel.Chart.Metadata.Name,
			ChartVersion:  e.rel.Chart.Metadata.Version,
			AppVersion:    e.rel.Chart.Metadata.AppVersion,
			Revision:      e.rel.Version,
			Status:        e.rel.Info.Status,
			UpdatedAgeSec: e.updatedAge,
			AgeSeconds:    e.ageSec,
		})
	}
	return out
}

// HelmHistory lists all revision secrets for a named release in a namespace
// and returns them decoded, sorted by revision descending. Uses a live List
// with a label selector rather than the informer cache.
func (c *Client) HelmHistory(ctx context.Context, namespace, name string) ([]tui.HelmRevision, error) {
	list, err := c.Clientset.CoreV1().Secrets(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: "name=" + name + ",owner=helm",
	})
	if err != nil {
		return nil, fmt.Errorf("list helm secrets: %w", err)
	}

	var out []tui.HelmRevision
	for i := range list.Items {
		s := &list.Items[i]
		if s.Type != helmSecretType {
			continue
		}
		raw, ok := s.Data["release"]
		if !ok || len(raw) == 0 {
			continue
		}
		rel, err := decodeHelmRelease(raw)
		if err != nil {
			log.Printf("kubagachi: helm history decode %s/%s: %v", s.Namespace, s.Name, err)
			continue
		}
		var updatedAge int64
		if t, err := time.Parse(time.RFC3339, rel.Info.LastDeployed); err == nil {
			updatedAge = int64(time.Since(t).Seconds())
		}
		out = append(out, tui.HelmRevision{
			Revision:      rel.Version,
			Status:        rel.Info.Status,
			ChartVersion:  rel.Chart.Metadata.Version,
			AppVersion:    rel.Chart.Metadata.AppVersion,
			UpdatedAgeSec: updatedAge,
			Description:   rel.Info.Description,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Revision > out[j].Revision })
	return out, nil
}

// helmPathOnce caches the result of exec.LookPath("helm") so the filesystem
// is only searched once per process lifetime.
var helmPathOnce struct {
	sync.Once
	path string
	err  error
}

// helmPath returns the absolute path to the helm binary, or an error if it is
// not on PATH.
func (c *Client) helmPath() (string, error) {
	helmPathOnce.Do(func() {
		helmPathOnce.path, helmPathOnce.err = exec.LookPath("helm")
	})
	return helmPathOnce.path, helmPathOnce.err
}

// HelmAvailable reports whether the helm binary is available on PATH.
func (c *Client) HelmAvailable() bool {
	_, err := c.helmPath()
	return err == nil
}

// HelmRollback rolls back the named release to the given revision number using
// the helm binary. It passes --kube-context when the client has a named context
// so helm talks to the same cluster. The caller's ctx governs the process
// lifetime; the handler should supply a generous timeout (90 s).
func (c *Client) HelmRollback(ctx context.Context, namespace, name string, revision int) (string, error) {
	helm, err := c.helmPath()
	if err != nil {
		return "", fmt.Errorf("helm CLI not found on the kubagachi host: %w", err)
	}
	args := []string{
		"rollback", name, strconv.Itoa(revision),
		"-n", namespace,
		"--wait", "--timeout", "60s",
	}
	if c.ContextName != "" {
		args = append(args, "--kube-context", c.ContextName)
	}
	out, err := exec.CommandContext(ctx, helm, args...).CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("helm rollback: %w\n%s", err, out)
	}
	return string(out), nil
}

// HelmUninstall removes the named release from the given namespace using the
// helm binary. Like HelmRollback it respects the kube-context and the caller's
// ctx.
func (c *Client) HelmUninstall(ctx context.Context, namespace, name string) (string, error) {
	helm, err := c.helmPath()
	if err != nil {
		return "", fmt.Errorf("helm CLI not found on the kubagachi host: %w", err)
	}
	args := []string{
		"uninstall", name,
		"-n", namespace,
		"--wait", "--timeout", "60s",
	}
	if c.ContextName != "" {
		args = append(args, "--kube-context", c.ContextName)
	}
	out, err := exec.CommandContext(ctx, helm, args...).CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("helm uninstall: %w\n%s", err, out)
	}
	return string(out), nil
}

// HelmReleaseDetail fetches the config (values), manifest, and notes for a
// specific revision by constructing the canonical secret name.
func (c *Client) HelmReleaseDetail(ctx context.Context, namespace, name string, revision int) (tui.HelmDetail, error) {
	secretName := fmt.Sprintf("sh.helm.release.v1.%s.v%d", name, revision)
	s, err := c.Clientset.CoreV1().Secrets(namespace).Get(ctx, secretName, metav1.GetOptions{})
	if err != nil {
		return tui.HelmDetail{}, fmt.Errorf("get helm secret %s: %w", secretName, err)
	}
	raw, ok := s.Data["release"]
	if !ok {
		return tui.HelmDetail{}, fmt.Errorf("secret %s: missing release key", secretName)
	}
	rel, err := decodeHelmRelease(raw)
	if err != nil {
		return tui.HelmDetail{}, err
	}
	var valuesYAML string
	if len(rel.Config) > 0 {
		if y, err := sigsyaml.Marshal(rel.Config); err == nil {
			valuesYAML = string(y)
		}
	}
	return tui.HelmDetail{
		Values:   valuesYAML,
		Manifest: rel.Manifest,
		Notes:    rel.Info.Notes,
	}, nil
}
