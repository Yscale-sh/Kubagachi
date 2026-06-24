package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/creack/pty"
	"golang.org/x/net/websocket"

	"github.com/jakenesler/kubagachi/internal/sprites"
	"github.com/jakenesler/kubagachi/internal/state"
	webui "github.com/jakenesler/kubagachi/web"
)

// --- wire types: the JSON contract the browser UI consumes -------------------

type webContainer struct {
	Name         string `json:"name"`
	Image        string `json:"image,omitempty"`
	Ready        bool   `json:"ready"`
	RestartCount int32  `json:"restartCount"`
	State        string `json:"state,omitempty"`
	Reason       string `json:"reason,omitempty"`
}

type webPod struct {
	UID       string `json:"uid"`
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Critter   string `json:"critter"`
	Status    string `json:"status"`
	// CritterState is the animation deck to play (sprite-sheet-<state>.png):
	// the health state by default, or a workload animation (bursting, …) when
	// one was overlaid. Separate from Status, which drives color/label.
	CritterState string         `json:"critterState"`
	Phase        string         `json:"phase"`
	Reason       string         `json:"reason,omitempty"`
	Node         string         `json:"node"`
	IP           string         `json:"ip,omitempty"`
	Ready        string         `json:"ready"`
	Restarts     int32          `json:"restarts"`
	AgeSec       int64          `json:"ageSec"`
	Owner        string         `json:"owner,omitempty"`
	CPUMilli     int64          `json:"cpuMilli"` // -1 == unknown
	MemBytes     int64          `json:"memBytes"` // -1 == unknown
	Containers   []webContainer `json:"containers"`
}

type webNode struct {
	Name     string `json:"name"`
	Status   string `json:"status"`
	CPU      string `json:"cpu"`
	Mem      string `json:"mem"`
	CPUPct   int    `json:"cpuPct"` // -1 == unknown
	MemPct   int    `json:"memPct"` // -1 == unknown
	PodCount int    `json:"podCount"`
}

type webEvent struct {
	Type      string `json:"type"`
	Reason    string `json:"reason"`
	Object    string `json:"object"`
	Namespace string `json:"namespace,omitempty"`
	Message   string `json:"message"`
	Time      string `json:"time"`
}

type webFlux struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Ready     string `json:"ready"`
	Suspended bool   `json:"suspended"`
	Revision  string `json:"revision,omitempty"`
	Source    string `json:"source,omitempty"`
	Message   string `json:"message,omitempty"`
	Age       string `json:"age"`
}

type webDeployment struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Replicas  int    `json:"replicas"`
	Ready     int    `json:"ready"`
	Updated   int    `json:"updated"`
	Available int    `json:"available"`
	Image     string `json:"image"`
	Selector  string `json:"selector"`
	AgeSec    int    `json:"ageSec"`
}

type webStatefulSet struct {
	Name          string `json:"name"`
	Namespace     string `json:"namespace"`
	Replicas      int    `json:"replicas"`
	ReadyReplicas int    `json:"readyReplicas"`
	ServiceName   string `json:"serviceName"`
	Image         string `json:"image"`
	AgeSec        int    `json:"ageSec"`
}

type webDaemonSet struct {
	Name                   string `json:"name"`
	Namespace              string `json:"namespace"`
	DesiredNumberScheduled int    `json:"desiredNumberScheduled"`
	NumberReady            int    `json:"numberReady"`
	NumberAvailable        int    `json:"numberAvailable"`
	Image                  string `json:"image"`
	AgeSec                 int    `json:"ageSec"`
}

type webReplicaSet struct {
	Name          string `json:"name"`
	Namespace     string `json:"namespace"`
	Replicas      int    `json:"replicas"`
	ReadyReplicas int    `json:"readyReplicas"`
	OwnerKind     string `json:"ownerKind,omitempty"`
	OwnerName     string `json:"ownerName,omitempty"`
	Image         string `json:"image"`
	AgeSec        int    `json:"ageSec"`
}

type webJob struct {
	Name        string `json:"name"`
	Namespace   string `json:"namespace"`
	Completions int    `json:"completions"`
	Succeeded   int    `json:"succeeded"`
	Failed      int    `json:"failed"`
	Active      int    `json:"active"`
	Status      string `json:"status"`
	Image       string `json:"image"`
	DurationSec int    `json:"durationSec,omitempty"`
	AgeSec      int    `json:"ageSec"`
}

type webCronJob struct {
	Name               string `json:"name"`
	Namespace          string `json:"namespace"`
	Schedule           string `json:"schedule"`
	Suspend            bool   `json:"suspend"`
	LastScheduleAgeSec int    `json:"lastScheduleAgeSec,omitempty"`
	HasLastSchedule    bool   `json:"hasLastSchedule"`
	ActiveJobs         int    `json:"activeJobs"`
	Status             string `json:"status"`
	Image              string `json:"image"`
	AgeSec             int    `json:"ageSec"`
}

type webServicePort struct {
	Name       string `json:"name"`
	Port       int    `json:"port"`
	TargetPort int    `json:"targetPort"`
	NodePort   int    `json:"nodePort"`
	Protocol   string `json:"protocol"`
}

type webService struct {
	Name       string           `json:"name"`
	Namespace  string           `json:"namespace"`
	Type       string           `json:"type"`
	ClusterIP  string           `json:"clusterIP"`
	ExternalIP string           `json:"externalIP"`
	Ports      []webServicePort `json:"ports"`
	Selector   string           `json:"selector"`
	AgeSec     int              `json:"ageSec"`
}

type webIngressRule struct {
	Host        string `json:"host"`
	Path        string `json:"path"`
	ServiceName string `json:"serviceName"`
	ServicePort int    `json:"servicePort"`
}

type webIngress struct {
	Name      string           `json:"name"`
	Namespace string           `json:"namespace"`
	ClassName string           `json:"className,omitempty"`
	Hosts     []string         `json:"hosts"`
	Rules     []webIngressRule `json:"rules"`
	TLS       bool             `json:"tls"`
	Address   string           `json:"address,omitempty"`
	AgeSec    int              `json:"ageSec"`
}

type webConfigMap struct {
	Name      string   `json:"name"`
	Namespace string   `json:"namespace"`
	Keys      []string `json:"keys"`
	DataBytes int      `json:"dataBytes"`
	AgeSec    int      `json:"ageSec"`
}

type webSecret struct {
	Name      string   `json:"name"`
	Namespace string   `json:"namespace"`
	Type      string   `json:"type"`
	Keys      []string `json:"keys"`
	DataBytes int      `json:"dataBytes"`
	AgeSec    int      `json:"ageSec"`
}

type webPersistentVolumeClaim struct {
	Name             string   `json:"name"`
	Namespace        string   `json:"namespace"`
	Capacity         string   `json:"capacity"`
	AccessModes      []string `json:"accessModes"`
	StorageClassName string   `json:"storageClassName"`
	Phase            string   `json:"phase"`
	VolumeName       string   `json:"volumeName,omitempty"`
	AgeSec           int      `json:"ageSec"`
}

type webPersistentVolume struct {
	Name             string   `json:"name"`
	Capacity         string   `json:"capacity"`
	AccessModes      []string `json:"accessModes"`
	ReclaimPolicy    string   `json:"reclaimPolicy"`
	Phase            string   `json:"phase"`
	StorageClassName string   `json:"storageClassName"`
	ClaimNamespace   string   `json:"claimNamespace,omitempty"`
	ClaimName        string   `json:"claimName,omitempty"`
	AgeSec           int      `json:"ageSec"`
}

type webStorageClass struct {
	Name              string `json:"name"`
	Provisioner       string `json:"provisioner"`
	ReclaimPolicy     string `json:"reclaimPolicy"`
	VolumeBindingMode string `json:"volumeBindingMode"`
	IsDefault         bool   `json:"isDefault"`
	AgeSec            int    `json:"ageSec"`
}

type webSnapshot struct {
	Mode                   string                     `json:"mode"` // "live" | "demo"
	Context                string                     `json:"context"`
	CurrentNamespace       string                     `json:"currentNamespace"`
	FluxInstalled          bool                       `json:"fluxInstalled"`
	MetricsInstalled       bool                       `json:"metricsInstalled"`
	Pods                   []webPod                   `json:"pods"`
	Nodes                  []webNode                  `json:"nodes"`
	Namespaces             []string                   `json:"namespaces"`
	Events                 []webEvent                 `json:"events"`
	Flux                   []webFlux                  `json:"flux"`
	Deployments            []webDeployment            `json:"deployments"`
	StatefulSets           []webStatefulSet           `json:"statefulSets"`
	DaemonSets             []webDaemonSet             `json:"daemonSets"`
	ReplicaSets            []webReplicaSet            `json:"replicaSets"`
	Jobs                   []webJob                   `json:"jobs"`
	CronJobs               []webCronJob               `json:"cronJobs"`
	Services               []webService               `json:"services"`
	Ingresses              []webIngress               `json:"ingresses"`
	ConfigMaps             []webConfigMap             `json:"configMaps"`
	Secrets                []webSecret                `json:"secrets"`
	PersistentVolumeClaims []webPersistentVolumeClaim `json:"persistentVolumeClaims"`
	PersistentVolumes      []webPersistentVolume      `json:"persistentVolumes"`
	StorageClasses         []webStorageClass          `json:"storageClasses"`
}

// webStatus maps kubagachi's normalized statuses onto the web UI vocabulary
// (used for color and label).
func webStatus(s string) string {
	switch s {
	case state.StatusFailed, state.StatusOOMKilled:
		return "error"
	case state.StatusImagePull:
		return "backoff"
	default:
		return s
	}
}

func toWebSnapshot(cs state.ClusterState, mode string) webSnapshot {
	snap := webSnapshot{
		Mode:                   mode,
		Context:                cs.ClusterName,
		CurrentNamespace:       cs.Namespace,
		FluxInstalled:          cs.FluxInstalled,
		MetricsInstalled:       cs.MetricsInstalled,
		Pods:                   []webPod{},
		Nodes:                  []webNode{},
		Namespaces:             []string{},
		Events:                 []webEvent{},
		Flux:                   []webFlux{},
		Deployments:            []webDeployment{},
		StatefulSets:           []webStatefulSet{},
		DaemonSets:             []webDaemonSet{},
		ReplicaSets:            []webReplicaSet{},
		Jobs:                   []webJob{},
		CronJobs:               []webCronJob{},
		Services:               []webService{},
		Ingresses:              []webIngress{},
		ConfigMaps:             []webConfigMap{},
		Secrets:                []webSecret{},
		PersistentVolumeClaims: []webPersistentVolumeClaim{},
		PersistentVolumes:      []webPersistentVolume{},
		StorageClasses:         []webStorageClass{},
	}
	nsSeen := map[string]bool{}
	for _, p := range cs.Pods {
		ready := 0
		containers := make([]webContainer, 0, len(p.Containers))
		for _, c := range p.Containers {
			if c.Ready {
				ready++
			}
			containers = append(containers, webContainer{
				Name: c.Name, Image: c.Image, Ready: c.Ready,
				RestartCount: c.RestartCount, State: c.State, Reason: c.Reason,
			})
		}
		snap.Pods = append(snap.Pods, webPod{
			UID:          p.UID,
			Name:         p.Name,
			Namespace:    p.Namespace,
			Critter:      p.Critter,
			Status:       webStatus(p.Status),
			CritterState: webStatus(p.CritterState), // canonical anim key (workload names pass through)
			Phase:        p.Phase,
			Reason:       p.Reason,
			Node:         p.NodeName,
			IP:           p.IP,
			Ready:        fmt.Sprintf("%d/%d", ready, len(p.Containers)),
			Restarts:     p.Restarts,
			AgeSec:       p.AgeSeconds,
			Owner:        p.Owner,
			CPUMilli:     p.CPUUsedMilli,
			MemBytes:     p.MemUsedBytes,
			Containers:   containers,
		})
		if !nsSeen[p.Namespace] {
			nsSeen[p.Namespace] = true
			snap.Namespaces = append(snap.Namespaces, p.Namespace)
		}
	}
	for _, n := range cs.Nodes {
		status := "ready"
		if !n.Ready {
			status = "notready"
		}
		snap.Nodes = append(snap.Nodes, webNode{
			Name: n.Name, Status: status, CPU: n.CPUText, Mem: n.MemoryText,
			CPUPct: n.CPUPercent(), MemPct: n.MemPercent(),
			PodCount: len(n.Pods),
		})
	}
	for _, e := range cs.Events {
		snap.Events = append(snap.Events, webEvent{
			Type:      strings.ToLower(e.Type),
			Reason:    e.Reason,
			Object:    e.Object,
			Namespace: e.Namespace,
			Message:   e.Message,
			Time:      e.Time,
		})
	}
	for _, f := range cs.Flux {
		snap.Flux = append(snap.Flux, webFlux{
			Kind: f.Kind, Name: f.Name, Namespace: f.Namespace,
			Ready: f.Ready, Suspended: f.Suspended, Revision: f.Revision,
			Source: f.Source, Message: f.Message, Age: f.Age,
		})
	}
	for _, d := range cs.Deployments {
		snap.Deployments = append(snap.Deployments, webDeployment{
			Name: d.Name, Namespace: d.Namespace,
			Replicas: int(d.Replicas), Ready: int(d.Ready),
			Updated: int(d.Updated), Available: int(d.Available),
			Image: d.Image, Selector: d.Selector, AgeSec: int(d.AgeSeconds),
		})
	}
	for _, s := range cs.StatefulSets {
		snap.StatefulSets = append(snap.StatefulSets, webStatefulSet{
			Name: s.Name, Namespace: s.Namespace,
			Replicas: int(s.Replicas), ReadyReplicas: int(s.ReadyReplicas),
			ServiceName: s.ServiceName, Image: s.Image, AgeSec: int(s.AgeSeconds),
		})
	}
	for _, d := range cs.DaemonSets {
		snap.DaemonSets = append(snap.DaemonSets, webDaemonSet{
			Name: d.Name, Namespace: d.Namespace,
			DesiredNumberScheduled: int(d.DesiredNumberScheduled),
			NumberReady:            int(d.NumberReady), NumberAvailable: int(d.NumberAvailable),
			Image: d.Image, AgeSec: int(d.AgeSeconds),
		})
	}
	for _, r := range cs.ReplicaSets {
		snap.ReplicaSets = append(snap.ReplicaSets, webReplicaSet{
			Name: r.Name, Namespace: r.Namespace,
			Replicas: int(r.Replicas), ReadyReplicas: int(r.ReadyReplicas),
			OwnerKind: r.OwnerKind, OwnerName: r.OwnerName,
			Image: r.Image, AgeSec: int(r.AgeSeconds),
		})
	}
	for _, j := range cs.Jobs {
		snap.Jobs = append(snap.Jobs, webJob{
			Name: j.Name, Namespace: j.Namespace,
			Completions: int(j.Completions), Succeeded: int(j.Succeeded),
			Failed: int(j.Failed), Active: int(j.Active), Status: j.Status,
			Image: j.Image, DurationSec: int(j.DurationSec), AgeSec: int(j.AgeSeconds),
		})
	}
	for _, c := range cs.CronJobs {
		snap.CronJobs = append(snap.CronJobs, webCronJob{
			Name: c.Name, Namespace: c.Namespace, Schedule: c.Schedule,
			Suspend: c.Suspend, LastScheduleAgeSec: int(c.LastScheduleAgeSec),
			HasLastSchedule: c.HasLastSchedule, ActiveJobs: c.ActiveJobs,
			Status: c.Status, Image: c.Image, AgeSec: int(c.AgeSeconds),
		})
	}
	for _, s := range cs.Services {
		ports := make([]webServicePort, 0, len(s.Ports))
		for _, p := range s.Ports {
			ports = append(ports, webServicePort{
				Name: p.Name, Port: int(p.Port), TargetPort: int(p.TargetPort),
				NodePort: int(p.NodePort), Protocol: p.Protocol,
			})
		}
		snap.Services = append(snap.Services, webService{
			Name: s.Name, Namespace: s.Namespace, Type: s.Type,
			ClusterIP: s.ClusterIP, ExternalIP: s.ExternalIP,
			Ports: ports, Selector: s.Selector, AgeSec: int(s.AgeSeconds),
		})
	}
	for _, i := range cs.Ingresses {
		hosts := i.Hosts
		if hosts == nil {
			hosts = []string{}
		}
		rules := make([]webIngressRule, 0, len(i.Rules))
		for _, r := range i.Rules {
			rules = append(rules, webIngressRule{
				Host: r.Host, Path: r.Path, ServiceName: r.ServiceName, ServicePort: int(r.ServicePort),
			})
		}
		snap.Ingresses = append(snap.Ingresses, webIngress{
			Name: i.Name, Namespace: i.Namespace, ClassName: i.ClassName,
			Hosts: hosts, Rules: rules, TLS: i.TLS, Address: i.Address,
			AgeSec: int(i.AgeSeconds),
		})
	}
	for _, c := range cs.ConfigMaps {
		keys := c.Keys
		if keys == nil {
			keys = []string{}
		}
		snap.ConfigMaps = append(snap.ConfigMaps, webConfigMap{
			Name: c.Name, Namespace: c.Namespace,
			Keys: keys, DataBytes: c.DataBytes, AgeSec: int(c.AgeSeconds),
		})
	}
	for _, s := range cs.Secrets {
		keys := s.Keys
		if keys == nil {
			keys = []string{}
		}
		snap.Secrets = append(snap.Secrets, webSecret{
			Name: s.Name, Namespace: s.Namespace, Type: s.Type,
			Keys: keys, DataBytes: s.DataBytes, AgeSec: int(s.AgeSeconds),
		})
	}
	for _, p := range cs.PersistentVolumeClaims {
		modes := p.AccessModes
		if modes == nil {
			modes = []string{}
		}
		snap.PersistentVolumeClaims = append(snap.PersistentVolumeClaims, webPersistentVolumeClaim{
			Name: p.Name, Namespace: p.Namespace, Capacity: p.Capacity,
			AccessModes: modes, StorageClassName: p.StorageClassName, Phase: p.Phase,
			VolumeName: p.VolumeName, AgeSec: int(p.AgeSeconds),
		})
	}
	for _, p := range cs.PersistentVolumes {
		modes := p.AccessModes
		if modes == nil {
			modes = []string{}
		}
		snap.PersistentVolumes = append(snap.PersistentVolumes, webPersistentVolume{
			Name: p.Name, Capacity: p.Capacity, AccessModes: modes,
			ReclaimPolicy: p.ReclaimPolicy, Phase: p.Phase,
			StorageClassName: p.StorageClassName, ClaimNamespace: p.ClaimNamespace,
			ClaimName: p.ClaimName, AgeSec: int(p.AgeSeconds),
		})
	}
	for _, s := range cs.StorageClasses {
		snap.StorageClasses = append(snap.StorageClasses, webStorageClass{
			Name: s.Name, Provisioner: s.Provisioner, ReclaimPolicy: s.ReclaimPolicy,
			VolumeBindingMode: s.VolumeBindingMode, IsDefault: s.IsDefault,
			AgeSec: int(s.AgeSeconds),
		})
	}
	return snap
}

// --- snapshot hub: latest value + SSE fan-out --------------------------------

type snapshotHub struct {
	mu     sync.RWMutex
	latest webSnapshot
	subs   map[chan webSnapshot]struct{}
}

func newSnapshotHub() *snapshotHub {
	return &snapshotHub{subs: map[chan webSnapshot]struct{}{}}
}

func (h *snapshotHub) set(s webSnapshot) {
	h.mu.Lock()
	h.latest = s
	for ch := range h.subs {
		select {
		case ch <- s:
		default: // slow subscriber: drop, it will catch up on the next tick
		}
	}
	h.mu.Unlock()
}

func (h *snapshotHub) get() webSnapshot {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.latest
}

func (h *snapshotHub) subscribe() (chan webSnapshot, func()) {
	ch := make(chan webSnapshot, 4)
	h.mu.Lock()
	h.subs[ch] = struct{}{}
	h.mu.Unlock()
	return ch, func() {
		h.mu.Lock()
		delete(h.subs, ch)
		h.mu.Unlock()
	}
}

// --- server -------------------------------------------------------------------

func runWeb(ctx context.Context, cfg Config, source ClusterSource, snapshots <-chan state.ClusterState) error {
	hub := newSnapshotHub()
	go func() {
		for snap := range snapshots {
			hub.set(toWebSnapshot(snap, source.Label()))
		}
	}()

	mux := http.NewServeMux()
	registerUI(mux)
	registerSprites(mux, cfg)
	registerAPI(mux, hub, source)

	srv := &http.Server{Addr: cfg.WebAddr, Handler: mux}
	errc := make(chan error, 1)
	go func() {
		url := webURL(cfg.WebAddr)
		fmt.Printf("kubagachi web · %s\n", url)
		if cfg.App {
			go openAppWindow(url)
		}
		errc <- srv.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		_ = srv.Shutdown(context.Background())
		return ctx.Err()
	case err := <-errc:
		if err == http.ErrServerClosed {
			return nil
		}
		return err
	}
}

// registerUI serves the embedded vite build with an SPA fallback.
func registerUI(mux *http.ServeMux) {
	dist, err := fs.Sub(webui.Dist, "dist")
	if err != nil {
		return
	}
	fileServer := http.FileServer(http.FS(dist))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(r.URL.Path, "/")
		if p != "" {
			if f, err := dist.Open(p); err == nil {
				_ = f.Close()
				fileServer.ServeHTTP(w, r)
				return
			}
		}
		// SPA shell for every unknown path.
		w.Header().Set("Cache-Control", "no-store, must-revalidate")
		index, err := fs.ReadFile(dist, "index.html")
		if err != nil {
			http.Error(w, "web UI not built — run `npm run build` in web/", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(index)
	})
}

// registerSprites exposes the critter sprite sheets when a critters dir exists.
func registerSprites(mux *http.ServeMux, cfg Config) {
	crittersDir := cfg.PixelCritters
	if crittersDir == "" {
		crittersDir = "critters"
	}
	abs, err := filepath.Abs(crittersDir)
	if err == nil {
		if _, statErr := os.Stat(abs); statErr == nil {
			mux.Handle("/critters/", http.StripPrefix("/critters/",
				noStore(http.FileServer(http.Dir(abs)))))
		}
	}
	mux.HandleFunc("/api/critters", func(w http.ResponseWriter, r *http.Request) {
		list, err := sprites.Scan(abs)
		if err != nil {
			list = []sprites.Info{}
		}
		writeJSON(w, map[string]any{"states": sprites.States, "critters": list})
	})
}

func registerAPI(mux *http.ServeMux, hub *snapshotHub, source ClusterSource) {
	actions := source.Actions()

	mux.HandleFunc("/api/snapshot", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, hub.get())
	})

	// SSE: one snapshot per cluster change plus a heartbeat comment.
	mux.HandleFunc("/api/stream", func(w http.ResponseWriter, r *http.Request) {
		fl, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		send := func(s webSnapshot) bool {
			data, err := json.Marshal(s)
			if err != nil {
				return false
			}
			if _, err := fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
				return false
			}
			fl.Flush()
			return true
		}

		ch, cancel := hub.subscribe()
		defer cancel()
		if !send(hub.get()) {
			return
		}
		heartbeat := time.NewTicker(25 * time.Second)
		defer heartbeat.Stop()
		for {
			select {
			case <-r.Context().Done():
				return
			case s := <-ch:
				if !send(s) {
					return
				}
			case <-heartbeat.C:
				if _, err := io.WriteString(w, ": ping\n\n"); err != nil {
					return
				}
				fl.Flush()
			}
		}
	})

	mux.HandleFunc("/api/logs", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		ns, pod := q.Get("namespace"), q.Get("pod")
		if ns == "" || pod == "" {
			http.Error(w, "namespace and pod are required", http.StatusBadRequest)
			return
		}
		tail := int64(200)
		if t, err := strconv.ParseInt(q.Get("tail"), 10, 64); err == nil && t > 0 && t <= 5000 {
			tail = t
		}
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		body, err := actions.Logs(ctx, ns, pod, q.Get("container"), tail)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		writeJSON(w, map[string]string{"logs": body})
	})

	mux.HandleFunc("/api/describe", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		ns, pod := q.Get("namespace"), q.Get("pod")
		if ns == "" || pod == "" {
			http.Error(w, "namespace and pod are required", http.StatusBadRequest)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		body, err := actions.Describe(ctx, ns, pod)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		writeJSON(w, map[string]string{"describe": body})
	})

	mux.HandleFunc("/api/pods/delete", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST only", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			Namespace string `json:"namespace"`
			Name      string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Namespace == "" || req.Name == "" {
			http.Error(w, "namespace and name are required", http.StatusBadRequest)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		if err := actions.DeletePod(ctx, req.Namespace, req.Name); err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		writeJSON(w, map[string]string{"status": "ok"})
	})

	mux.HandleFunc("/api/flux/action", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST only", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			Kind      string `json:"kind"`
			Namespace string `json:"namespace"`
			Name      string `json:"name"`
			Action    string `json:"action"` // reconcile | suspend | resume
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Kind == "" || req.Name == "" {
			http.Error(w, "kind, name and action are required", http.StatusBadRequest)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		var err error
		switch req.Action {
		case "reconcile":
			err = actions.FluxReconcile(ctx, req.Kind, req.Namespace, req.Name)
		case "suspend":
			err = actions.FluxSuspend(ctx, req.Kind, req.Namespace, req.Name, true)
		case "resume":
			err = actions.FluxSuspend(ctx, req.Kind, req.Namespace, req.Name, false)
		default:
			http.Error(w, "unknown action", http.StatusBadRequest)
			return
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		writeJSON(w, map[string]string{"status": "ok"})
	})

	mux.Handle("/api/exec", websocket.Handler(func(ws *websocket.Conn) {
		execSession(ws, actions)
	}))
}

// --- exec: kubectl passthrough over a websocket -------------------------------

// termFrame is the Freelens-style terminal message envelope.
type termFrame struct {
	Type string `json:"type"` // stdin | stdout | resize | connected | ping | exit
	Data string `json:"data,omitempty"`
	Cols uint16 `json:"cols,omitempty"`
	Rows uint16 `json:"rows,omitempty"`
}

func execSession(ws *websocket.Conn, actions interface {
	ExecArgs(namespace, pod, container string) []string
}) {
	defer ws.Close()
	q := ws.Request().URL.Query()
	ns, pod, container := q.Get("namespace"), q.Get("pod"), q.Get("container")

	fail := func(msg string) {
		_ = websocket.JSON.Send(ws, termFrame{Type: "stdout", Data: "\r\n\x1b[31m" + msg + "\x1b[0m\r\n"})
		_ = websocket.JSON.Send(ws, termFrame{Type: "exit"})
	}

	if ns == "" || pod == "" {
		fail("namespace and pod are required")
		return
	}
	argv := actions.ExecArgs(ns, pod, container)
	if len(argv) == 0 {
		fail("shell not available in this mode (demo)")
		return
	}
	if _, err := exec.LookPath(argv[0]); err != nil {
		fail(argv[0] + " not found on PATH")
		return
	}

	cmd := exec.Command(argv[0], argv[1:]...)
	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: 30, Cols: 100})
	if err != nil {
		fail("start shell: " + err.Error())
		return
	}
	defer func() {
		_ = ptmx.Close()
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	}()

	_ = websocket.JSON.Send(ws, termFrame{Type: "connected"})

	// pty -> websocket
	done := make(chan struct{})
	go func() {
		defer close(done)
		buf := make([]byte, 8192)
		for {
			n, err := ptmx.Read(buf)
			if n > 0 {
				if sendErr := websocket.JSON.Send(ws, termFrame{Type: "stdout", Data: string(buf[:n])}); sendErr != nil {
					return
				}
			}
			if err != nil {
				_ = websocket.JSON.Send(ws, termFrame{Type: "exit"})
				return
			}
		}
	}()

	// websocket -> pty
	for {
		var frame termFrame
		if err := websocket.JSON.Receive(ws, &frame); err != nil {
			break
		}
		switch frame.Type {
		case "stdin":
			// Write errors surface through the pty read loop as an exit.
			_, _ = ptmx.Write([]byte(frame.Data))
		case "resize":
			if frame.Cols > 0 && frame.Rows > 0 {
				_ = pty.Setsize(ptmx, &pty.Winsize{Rows: frame.Rows, Cols: frame.Cols})
			}
		case "ping":
			// keepalive only
		}
	}
	<-done
}

// --- helpers -------------------------------------------------------------------

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-cache")
	_ = json.NewEncoder(w).Encode(v)
}

func noStore(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store, must-revalidate")
		h.ServeHTTP(w, r)
	})
}

func webURL(addr string) string {
	if strings.HasPrefix(addr, ":") {
		return "http://127.0.0.1" + addr
	}
	return "http://" + addr
}

// openAppWindow opens the UI in a chromeless app window when a Chromium
// browser is available, falling back to the default browser.
func openAppWindow(url string) {
	time.Sleep(300 * time.Millisecond) // give the listener a beat
	switch runtime.GOOS {
	case "darwin":
		for _, app := range []string{"Google Chrome", "Chromium", "Brave Browser", "Microsoft Edge"} {
			if exec.Command("open", "-na", app, "--args", "--app="+url).Run() == nil {
				return
			}
		}
		_ = exec.Command("open", url).Run()
	case "linux":
		for _, bin := range []string{"google-chrome", "chromium", "chromium-browser", "brave-browser", "microsoft-edge"} {
			if _, err := exec.LookPath(bin); err == nil {
				if exec.Command(bin, "--app="+url).Start() == nil {
					return
				}
			}
		}
		_ = exec.Command("xdg-open", url).Start()
	}
}
