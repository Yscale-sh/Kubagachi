package k8s

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// kubectlPath resolves the kubectl binary. It prefers $PATH but falls back to
// the common install locations, so a server launched with a stripped PATH
// (e.g. from a GUI / non-login shell) still finds it.
func kubectlPath() string {
	if p, err := exec.LookPath("kubectl"); err == nil {
		return p
	}
	home, _ := os.UserHomeDir()
	for _, p := range []string{
		"/opt/homebrew/bin/kubectl",
		"/usr/local/bin/kubectl",
		"/usr/bin/kubectl",
		filepath.Join(home, ".rd/bin/kubectl"),   // Rancher Desktop
		filepath.Join(home, ".docker/bin/kubectl"),
	} {
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			return p
		}
	}
	return "kubectl" // last resort — exec will surface a clear error
}

// PodLogs fetches the last tail lines of a container's logs. An empty
// container name lets the API server pick the default container.
func (c *Client) PodLogs(ctx context.Context, namespace, name, container string, tail int64) (string, error) {
	opts := &corev1.PodLogOptions{TailLines: &tail}
	if container != "" {
		opts.Container = container
	}
	req := c.Clientset.CoreV1().Pods(namespace).GetLogs(name, opts)
	rc, err := req.Stream(ctx)
	if err != nil {
		return "", fmt.Errorf("stream logs: %w", err)
	}
	defer rc.Close()
	data, err := io.ReadAll(io.LimitReader(rc, 2<<20)) // 2MiB cap
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// DeletePod deletes a pod with default grace.
func (c *Client) DeletePod(ctx context.Context, namespace, name string) error {
	return c.Clientset.CoreV1().Pods(namespace).Delete(ctx, name, metav1.DeleteOptions{})
}

// ExecArgs returns the argv for an interactive shell into a pod container —
// the same kubectl passthrough trick k9s uses: probe for bash, fall back to sh.
func (c *Client) ExecArgs(namespace, pod, container string) []string {
	args := []string{kubectlPath(), "exec", "-it", "-n", namespace, pod}
	if c.ContextName != "" {
		args = append(args, "--context", c.ContextName)
	}
	if container != "" {
		args = append(args, "-c", container)
	}
	return append(args, "--", "sh", "-c",
		`command -v bash >/dev/null && exec bash || exec sh`)
}

// Describe builds a kubectl-describe-style text summary of a pod, including
// its recent events.
func (c *Client) Describe(ctx context.Context, namespace, name string) (string, error) {
	pod, err := c.Clientset.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}

	var b strings.Builder
	w := func(k, v string) { fmt.Fprintf(&b, "%-16s %s\n", k, v) }
	w("name", pod.Name)
	w("namespace", pod.Namespace)
	w("node", orEmpty(pod.Spec.NodeName))
	w("status", string(pod.Status.Phase))
	if pod.Status.Reason != "" {
		w("reason", pod.Status.Reason)
	}
	w("pod ip", orEmpty(pod.Status.PodIP))
	w("host ip", orEmpty(pod.Status.HostIP))
	w("qos", string(pod.Status.QOSClass))
	w("service account", orEmpty(pod.Spec.ServiceAccountName))
	if len(pod.OwnerReferences) > 0 {
		w("owner", pod.OwnerReferences[0].Kind+"/"+pod.OwnerReferences[0].Name)
	}

	if len(pod.Labels) > 0 {
		b.WriteString("\nlabels\n")
		for _, k := range sortedKeys(pod.Labels) {
			fmt.Fprintf(&b, "  %s=%s\n", k, pod.Labels[k])
		}
	}

	b.WriteString("\ncontainers\n")
	statusByName := map[string]corev1.ContainerStatus{}
	for _, cs := range pod.Status.ContainerStatuses {
		statusByName[cs.Name] = cs
	}
	for _, ct := range pod.Spec.Containers {
		fmt.Fprintf(&b, "  %s\n", ct.Name)
		fmt.Fprintf(&b, "    image     %s\n", ct.Image)
		if cs, ok := statusByName[ct.Name]; ok {
			fmt.Fprintf(&b, "    ready     %v · restarts %d\n", cs.Ready, cs.RestartCount)
			switch {
			case cs.State.Running != nil:
				fmt.Fprintf(&b, "    state     running since %s\n", cs.State.Running.StartedAt.Format("15:04:05"))
			case cs.State.Waiting != nil:
				fmt.Fprintf(&b, "    state     waiting · %s\n", cs.State.Waiting.Reason)
			case cs.State.Terminated != nil:
				fmt.Fprintf(&b, "    state     terminated · %s · exit %d\n", cs.State.Terminated.Reason, cs.State.Terminated.ExitCode)
			}
		}
		if req := ct.Resources.Requests; len(req) > 0 {
			fmt.Fprintf(&b, "    requests  cpu %s · mem %s\n", req.Cpu().String(), req.Memory().String())
		}
		if lim := ct.Resources.Limits; len(lim) > 0 {
			fmt.Fprintf(&b, "    limits    cpu %s · mem %s\n", lim.Cpu().String(), lim.Memory().String())
		}
	}

	if len(pod.Status.Conditions) > 0 {
		b.WriteString("\nconditions\n")
		for _, cond := range pod.Status.Conditions {
			fmt.Fprintf(&b, "  %-16s %s\n", string(cond.Type), string(cond.Status))
		}
	}

	events, err := c.Clientset.CoreV1().Events(namespace).List(ctx, metav1.ListOptions{
		FieldSelector: "involvedObject.name=" + name,
	})
	if err == nil && len(events.Items) > 0 {
		b.WriteString("\nevents\n")
		items := events.Items
		sort.Slice(items, func(i, j int) bool {
			return eventTime(&items[i]).After(eventTime(&items[j]))
		})
		max := len(items)
		if max > 15 {
			max = 15
		}
		for _, e := range items[:max] {
			fmt.Fprintf(&b, "  %-6s %-8s %-18s %s\n",
				humanizeAge(eventTime(&e)), e.Type, e.Reason, singleLine(e.Message))
		}
	}
	return b.String(), nil
}

func orEmpty(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
