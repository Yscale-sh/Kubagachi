// Package k8s connects to a real Kubernetes cluster and converts live API
// objects into the normalized state used by the TUI.
package k8s

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// KubeconfigSource selects where kubeconfig data is read from. The zero value
// uses kubectl's standard loading rules (the KUBECONFIG env var, then
// ~/.kube/config), so existing behavior is unchanged when nothing is set.
type KubeconfigSource struct {
	// Path is an explicit kubeconfig file to load instead of the default search
	// path. Ignored when Raw is set.
	Path string
	// Raw is inline kubeconfig YAML. When non-empty it takes precedence over the
	// default rules and Path, so a pasted config needs no file on disk.
	Raw string
}

// clientConfig builds a clientcmd.ClientConfig for this source with the given
// overrides applied (e.g. a current-context override).
func (s KubeconfigSource) clientConfig(overrides *clientcmd.ConfigOverrides) (clientcmd.ClientConfig, error) {
	if raw := strings.TrimSpace(s.Raw); raw != "" {
		apiCfg, err := clientcmd.Load([]byte(s.Raw))
		if err != nil {
			return nil, fmt.Errorf("parsing kubeconfig: %w", err)
		}
		return clientcmd.NewDefaultClientConfig(*apiCfg, overrides), nil
	}
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	if s.Path != "" {
		rules.ExplicitPath = s.Path
	}
	return clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, overrides), nil
}

// Client bundles a ready-to-use clientset with the resolved context and
// default namespace pulled from the kubeconfig. The dynamic client backs the
// Flux CRD watcher and the REST config is retained for log streaming.
type Client struct {
	Clientset        kubernetes.Interface
	Dynamic          dynamic.Interface
	RestConfig       *rest.Config
	ContextName      string
	DefaultNamespace string
	ServerVersion    string

	// Port-forward registry — guarded by pfMu.
	pfMu       sync.RWMutex
	pfRegistry map[string]*activeForward
}

// ContextInfo is one real kubeconfig context from the merged clientcmd config.
type ContextInfo struct {
	Name      string
	Cluster   string
	Namespace string
}

// ContextList is the merged kubeconfig context inventory plus its current
// context name.
type ContextList struct {
	Current  string
	Contexts []ContextInfo
}

// AvailableContexts returns the contexts visible in the given kubeconfig source.
// For the zero source this is kubectl's standard loading rules; KUBECONFIG may
// be a path list, which client-go merges.
func AvailableContexts(src KubeconfigSource) (ContextList, error) {
	cc, err := src.clientConfig(&clientcmd.ConfigOverrides{})
	if err != nil {
		return ContextList{}, err
	}
	raw, err := cc.RawConfig()
	if err != nil {
		return ContextList{}, fmt.Errorf("loading kubeconfig: %w", err)
	}
	out := ContextList{
		Current:  raw.CurrentContext,
		Contexts: make([]ContextInfo, 0, len(raw.Contexts)),
	}
	for name, ctx := range raw.Contexts {
		out.Contexts = append(out.Contexts, ContextInfo{
			Name:      name,
			Cluster:   ctx.Cluster,
			Namespace: ctx.Namespace,
		})
	}
	sort.Slice(out.Contexts, func(i, j int) bool {
		return out.Contexts[i].Name < out.Contexts[j].Name
	})
	return out, nil
}

// NewClient builds a Kubernetes client from the given kubeconfig source (the
// zero source uses the standard kubectl loading rules: KUBECONFIG env var, then
// ~/.kube/config). If contextName is non-empty it overrides the current-context.
// It performs a quick discovery call so connection failures surface immediately
// instead of inside the TUI.
func NewClient(src KubeconfigSource, contextName string) (*Client, error) {
	overrides := &clientcmd.ConfigOverrides{}
	if contextName != "" {
		overrides.CurrentContext = contextName
	}
	cc, err := src.clientConfig(overrides)
	if err != nil {
		return nil, err
	}

	restCfg, err := cc.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("loading kubeconfig: %w", err)
	}
	clientset, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		return nil, fmt.Errorf("creating Kubernetes client: %w", err)
	}
	dyn, err := dynamic.NewForConfig(restCfg)
	if err != nil {
		return nil, fmt.Errorf("creating dynamic client: %w", err)
	}

	resolvedCtx := contextName
	if resolvedCtx == "" {
		if raw, rerr := cc.RawConfig(); rerr == nil {
			resolvedCtx = raw.CurrentContext
		}
	}
	defaultNS, _, _ := cc.Namespace()
	if defaultNS == "" {
		defaultNS = "default"
	}

	version, err := clientset.Discovery().ServerVersion()
	if err != nil {
		return nil, fmt.Errorf("connecting to cluster (context %q): %w", resolvedCtx, err)
	}

	return &Client{
		Clientset:        clientset,
		Dynamic:          dyn,
		RestConfig:       restCfg,
		ContextName:      resolvedCtx,
		DefaultNamespace: defaultNS,
		ServerVersion:    version.GitVersion,
	}, nil
}
