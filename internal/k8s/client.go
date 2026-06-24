// Package k8s connects to a real Kubernetes cluster and converts live API
// objects into the normalized state used by the TUI.
package k8s

import (
	"fmt"
	"sort"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

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

// AvailableContexts returns the contexts visible through kubectl's standard
// loading rules. KUBECONFIG may be a path list; client-go handles the merge.
func AvailableContexts() (ContextList, error) {
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	cc := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, &clientcmd.ConfigOverrides{})
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

// NewClient builds a Kubernetes client using the standard kubectl kubeconfig
// loading rules (KUBECONFIG env var, then ~/.kube/config). If contextName is
// non-empty it overrides the current-context. It performs a quick discovery
// call so connection failures surface immediately instead of inside the TUI.
func NewClient(contextName string) (*Client, error) {
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	overrides := &clientcmd.ConfigOverrides{}
	if contextName != "" {
		overrides.CurrentContext = contextName
	}
	cc := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, overrides)

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
