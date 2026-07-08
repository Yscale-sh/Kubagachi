package k8s

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sync/atomic"
	"time"

	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
)

// PortForwardInfo describes one active pod port-forward.
type PortForwardInfo struct {
	ID         string
	Namespace  string
	Pod        string
	RemotePort int
	LocalPort  int
	AgeSec     int64
}

// activeForward is a registry entry for a running port-forward.
type activeForward struct {
	info      PortForwardInfo
	stopCh    chan struct{}
	startedAt time.Time
}

// pfSeq is an atomic counter used to generate unique forward IDs.
var pfSeq uint64

// PortForwardStart establishes a port-forward from localPort on 127.0.0.1 to
// remotePort on the named pod. localPort 0 lets the kernel choose; the actual
// local port is reflected in the returned PortForwardInfo.
// The forward's lifetime is owned by the registry — it outlives this call's ctx.
func (c *Client) PortForwardStart(_ context.Context, namespace, pod string, remotePort, localPort int) (PortForwardInfo, error) {
	url := c.Clientset.CoreV1().RESTClient().Post().
		Resource("pods").
		Namespace(namespace).
		Name(pod).
		SubResource("portforward").
		URL()

	roundTripper, upgrader, err := spdy.RoundTripperFor(c.RestConfig)
	if err != nil {
		return PortForwardInfo{}, fmt.Errorf("spdy round-tripper: %w", err)
	}
	dialer := spdy.NewDialer(upgrader, &http.Client{Transport: roundTripper}, "POST", url)

	stopCh := make(chan struct{})
	readyCh := make(chan struct{})
	errCh := make(chan error, 1)

	portSpec := fmt.Sprintf("%d:%d", localPort, remotePort)
	fw, err := portforward.NewOnAddresses(
		dialer,
		[]string{"127.0.0.1"},
		[]string{portSpec},
		stopCh, readyCh,
		io.Discard, io.Discard,
	)
	if err != nil {
		return PortForwardInfo{}, fmt.Errorf("create port-forwarder: %w", err)
	}

	seq := atomic.AddUint64(&pfSeq, 1)
	id := fmt.Sprintf("%s/%s/%d/%d", namespace, pod, remotePort, seq)

	go func() {
		// ForwardPorts blocks until stopCh is closed or the connection fails.
		if fwErr := fw.ForwardPorts(); fwErr != nil {
			select {
			case errCh <- fwErr:
			default:
			}
		}
		// Self-clean the registry so List remains truthful.
		c.pfMu.Lock()
		delete(c.pfRegistry, id)
		c.pfMu.Unlock()
	}()

	// Wait for ready (or failure) with a 10-second timeout.
	select {
	case <-readyCh:
		// Forward is up; fall through to register it.
	case fwErr := <-errCh:
		if fwErr == nil {
			fwErr = fmt.Errorf("port-forward exited before becoming ready")
		}
		return PortForwardInfo{}, fwErr
	case <-time.After(10 * time.Second):
		close(stopCh)
		return PortForwardInfo{}, fmt.Errorf("port-forward ready timeout after 10s")
	}

	ports, err := fw.GetPorts()
	if err != nil || len(ports) == 0 {
		close(stopCh)
		return PortForwardInfo{}, fmt.Errorf("get forwarded ports: %w", err)
	}
	actualLocal := int(ports[0].Local)

	info := PortForwardInfo{
		ID:         id,
		Namespace:  namespace,
		Pod:        pod,
		RemotePort: remotePort,
		LocalPort:  actualLocal,
	}

	c.pfMu.Lock()
	if c.pfRegistry == nil {
		c.pfRegistry = make(map[string]*activeForward)
	}
	c.pfRegistry[id] = &activeForward{info: info, stopCh: stopCh, startedAt: time.Now()}
	c.pfMu.Unlock()

	return info, nil
}

// PortForwardStop stops a forward by ID and removes it from the registry.
func (c *Client) PortForwardStop(id string) error {
	c.pfMu.Lock()
	af, ok := c.pfRegistry[id]
	if ok {
		delete(c.pfRegistry, id)
	}
	c.pfMu.Unlock()
	if !ok {
		return fmt.Errorf("port-forward %q not found", id)
	}
	close(af.stopCh)
	return nil
}

// PortForwardStopAll stops every active forward. Called when switching
// Kubernetes contexts so forwards do not leak across clusters.
func (c *Client) PortForwardStopAll() {
	c.pfMu.Lock()
	fwds := make([]*activeForward, 0, len(c.pfRegistry))
	for _, af := range c.pfRegistry {
		fwds = append(fwds, af)
	}
	c.pfRegistry = make(map[string]*activeForward)
	c.pfMu.Unlock()
	for _, af := range fwds {
		close(af.stopCh)
	}
}

// PortForwardList returns a snapshot of all active forwards with their current
// age computed from startedAt.
func (c *Client) PortForwardList() []PortForwardInfo {
	c.pfMu.RLock()
	defer c.pfMu.RUnlock()
	now := time.Now()
	out := make([]PortForwardInfo, 0, len(c.pfRegistry))
	for _, af := range c.pfRegistry {
		info := af.info
		info.AgeSec = int64(now.Sub(af.startedAt).Seconds())
		out = append(out, info)
	}
	return out
}
