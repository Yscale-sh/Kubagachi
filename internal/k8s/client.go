// Package k8s connects to a real Kubernetes cluster and converts live API
// objects into the normalized state used by the TUI.
package k8s

import (
	"fmt"

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

	if _, err := clientset.Discovery().ServerVersion(); err != nil {
		return nil, fmt.Errorf("connecting to cluster (context %q): %w", resolvedCtx, err)
	}

	return &Client{
		Clientset:        clientset,
		Dynamic:          dyn,
		RestConfig:       restCfg,
		ContextName:      resolvedCtx,
		DefaultNamespace: defaultNS,
	}, nil
}
