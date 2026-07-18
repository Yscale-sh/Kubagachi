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

	"github.com/yscale-sh/kubagachi/internal/k8s"
	"github.com/yscale-sh/kubagachi/internal/sprites"
	"github.com/yscale-sh/kubagachi/internal/state"
	webui "github.com/yscale-sh/kubagachi/web"
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
	Name           string `json:"name"`
	Status         string `json:"status"`
	KubeletVersion string `json:"kubeletVersion,omitempty"`
	CPU            string `json:"cpu"`
	Mem            string `json:"mem"`
	CPUPct         int    `json:"cpuPct"` // -1 == unknown
	MemPct         int    `json:"memPct"` // -1 == unknown
	PodCount       int    `json:"podCount"`
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
	Kind      string   `json:"kind"`
	Name      string   `json:"name"`
	Namespace string   `json:"namespace"`
	Ready     string   `json:"ready"`
	Suspended bool     `json:"suspended"`
	Revision  string   `json:"revision,omitempty"`
	Source    string   `json:"source,omitempty"`
	DependsOn []string `json:"dependsOn,omitempty"`
	Message   string   `json:"message,omitempty"`
	Age       string   `json:"age"`
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
	OwnerKind   string `json:"ownerKind,omitempty"`
	OwnerName   string `json:"ownerName,omitempty"`
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

type webEndpointSubset struct {
	Addresses []string `json:"addresses"`
	Ports     []int    `json:"ports"`
}

type webEndpoint struct {
	Name          string              `json:"name"`
	Namespace     string              `json:"namespace"`
	Subsets       []webEndpointSubset `json:"subsets"`
	TargetService string              `json:"targetService"`
	AgeSec        int                 `json:"ageSec"`
}

type webNetworkPolicy struct {
	Name         string            `json:"name"`
	Namespace    string            `json:"namespace"`
	PodSelector  map[string]string `json:"podSelector"`
	PolicyTypes  []string          `json:"policyTypes"`
	IngressRules int               `json:"ingressRules"`
	EgressRules  int               `json:"egressRules"`
	AgeSec       int               `json:"ageSec"`
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

type webResourceQuota struct {
	Name      string            `json:"name"`
	Namespace string            `json:"namespace"`
	Hard      map[string]string `json:"hard"`
	Used      map[string]string `json:"used"`
	AgeSec    int               `json:"ageSec"`
}

type webLimitRangeItem struct {
	Type           string            `json:"type"`
	DefaultRequest map[string]string `json:"defaultRequest"`
	DefaultLimit   map[string]string `json:"defaultLimit"`
}

type webLimitRange struct {
	Name      string              `json:"name"`
	Namespace string              `json:"namespace"`
	Limits    []webLimitRangeItem `json:"limits"`
	AgeSec    int                 `json:"ageSec"`
}

type webHorizontalPodAutoscaler struct {
	Name              string `json:"name"`
	Namespace         string `json:"namespace"`
	TargetKind        string `json:"targetKind"`
	TargetName        string `json:"targetName"`
	MinReplicas       int    `json:"minReplicas"`
	MaxReplicas       int    `json:"maxReplicas"`
	CurrentReplicas   int    `json:"currentReplicas"`
	TargetCPUPercent  *int   `json:"targetCPUPercent,omitempty"`
	CurrentCPUPercent *int   `json:"currentCPUPercent,omitempty"`
	AgeSec            int    `json:"ageSec"`
}

type webPodDisruptionBudget struct {
	Name           string            `json:"name"`
	Namespace      string            `json:"namespace"`
	MinAvailable   string            `json:"minAvailable,omitempty"`
	MaxUnavailable string            `json:"maxUnavailable,omitempty"`
	CurrentHealthy int               `json:"currentHealthy"`
	DesiredHealthy int               `json:"desiredHealthy"`
	ExpectedPods   int               `json:"expectedPods"`
	Selector       map[string]string `json:"selector"`
	AgeSec         int               `json:"ageSec"`
}

type webServiceAccount struct {
	Name             string   `json:"name"`
	Namespace        string   `json:"namespace"`
	Secrets          []string `json:"secrets"`
	ImagePullSecrets []string `json:"imagePullSecrets"`
	AutomountToken   bool     `json:"automountToken"`
	AgeSec           int      `json:"ageSec"`
}

type webPolicyRule struct {
	APIGroups []string `json:"apiGroups"`
	Resources []string `json:"resources"`
	Verbs     []string `json:"verbs"`
}

type webRole struct {
	Name      string          `json:"name"`
	Namespace string          `json:"namespace,omitempty"`
	Rules     []webPolicyRule `json:"rules"`
	AgeSec    int             `json:"ageSec"`
}

type webClusterRole struct {
	Name              string            `json:"name"`
	Rules             []webPolicyRule   `json:"rules"`
	AggregationLabels map[string]string `json:"aggregationLabels,omitempty"`
	AgeSec            int               `json:"ageSec"`
}

type webRoleRef struct {
	Kind string `json:"kind"`
	Name string `json:"name"`
}

type webSubject struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Namespace string `json:"namespace,omitempty"`
}

type webRoleBinding struct {
	Name      string       `json:"name"`
	Namespace string       `json:"namespace"`
	RoleRef   webRoleRef   `json:"roleRef"`
	Subjects  []webSubject `json:"subjects"`
	AgeSec    int          `json:"ageSec"`
}

type webClusterRoleBinding struct {
	Name     string       `json:"name"`
	RoleRef  webRoleRef   `json:"roleRef"`
	Subjects []webSubject `json:"subjects"`
	AgeSec   int          `json:"ageSec"`
}

type webNamespace struct {
	Name   string            `json:"name"`
	Phase  string            `json:"phase"`
	Labels map[string]string `json:"labels,omitempty"`
	AgeSec int               `json:"ageSec"`
}

type webCustomResourceDefinition struct {
	Name         string   `json:"name"`
	Group        string   `json:"group"`
	Scope        string   `json:"scope"`
	Versions     []string `json:"versions"`
	PluralName   string   `json:"pluralName"`
	SingularName string   `json:"singularName"`
	ListKind     string   `json:"listKind"`
	ShortNames   []string `json:"shortNames"`
	AgeSec       int      `json:"ageSec"`
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

type webHelmRelease struct {
	Name          string `json:"name"`
	Namespace     string `json:"namespace"`
	Chart         string `json:"chart"`
	ChartVersion  string `json:"chartVersion"`
	AppVersion    string `json:"appVersion"`
	Revision      int    `json:"revision"`
	Status        string `json:"status"`
	UpdatedAgeSec int64  `json:"updatedAgeSec"`
	AgeSec        int64  `json:"ageSec"`
}

type webContext struct {
	Name      string `json:"name"`
	Cluster   string `json:"cluster"`
	Namespace string `json:"namespace,omitempty"`
}

type webContextList struct {
	Current  string       `json:"current"`
	Contexts []webContext `json:"contexts"`
}

type webSnapshot struct {
	Mode                      string                        `json:"mode"` // "live" | "demo"
	Context                   string                        `json:"context"`
	Version                   string                        `json:"version,omitempty"`
	CurrentNamespace          string                        `json:"currentNamespace"`
	FluxInstalled             bool                          `json:"fluxInstalled"`
	MetricsInstalled          bool                          `json:"metricsInstalled"`
	Pods                      []webPod                      `json:"pods"`
	Nodes                     []webNode                     `json:"nodes"`
	Namespaces                []webNamespace                `json:"namespaces"`
	Events                    []webEvent                    `json:"events"`
	Flux                      []webFlux                     `json:"flux"`
	Deployments               []webDeployment               `json:"deployments"`
	StatefulSets              []webStatefulSet              `json:"statefulSets"`
	DaemonSets                []webDaemonSet                `json:"daemonSets"`
	ReplicaSets               []webReplicaSet               `json:"replicaSets"`
	Jobs                      []webJob                      `json:"jobs"`
	CronJobs                  []webCronJob                  `json:"cronJobs"`
	Services                  []webService                  `json:"services"`
	Ingresses                 []webIngress                  `json:"ingresses"`
	Endpoints                 []webEndpoint                 `json:"endpoints"`
	NetworkPolicies           []webNetworkPolicy            `json:"networkPolicies"`
	ConfigMaps                []webConfigMap                `json:"configMaps"`
	Secrets                   []webSecret                   `json:"secrets"`
	ResourceQuotas            []webResourceQuota            `json:"resourceQuotas"`
	LimitRanges               []webLimitRange               `json:"limitRanges"`
	HorizontalPodAutoscalers  []webHorizontalPodAutoscaler  `json:"horizontalPodAutoscalers"`
	PodDisruptionBudgets      []webPodDisruptionBudget      `json:"podDisruptionBudgets"`
	ServiceAccounts           []webServiceAccount           `json:"serviceAccounts"`
	Roles                     []webRole                     `json:"roles"`
	ClusterRoles              []webClusterRole              `json:"clusterRoles"`
	RoleBindings              []webRoleBinding              `json:"roleBindings"`
	ClusterRoleBindings       []webClusterRoleBinding       `json:"clusterRoleBindings"`
	CustomResourceDefinitions []webCustomResourceDefinition `json:"customResourceDefinitions"`
	PersistentVolumeClaims    []webPersistentVolumeClaim    `json:"persistentVolumeClaims"`
	PersistentVolumes         []webPersistentVolume         `json:"persistentVolumes"`
	StorageClasses            []webStorageClass             `json:"storageClasses"`
	HelmReleases              []webHelmRelease              `json:"helmReleases"`
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

func webStringSlice(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

func webStringMap(values map[string]string) map[string]string {
	if values == nil {
		return map[string]string{}
	}
	return values
}

func webPolicyRules(rules []state.PolicyRuleView) []webPolicyRule {
	out := make([]webPolicyRule, 0, len(rules))
	for _, rule := range rules {
		out = append(out, webPolicyRule{
			APIGroups: webStringSlice(rule.APIGroups),
			Resources: webStringSlice(rule.Resources),
			Verbs:     webStringSlice(rule.Verbs),
		})
	}
	return out
}

func webSubjects(subjects []state.SubjectView) []webSubject {
	out := make([]webSubject, 0, len(subjects))
	for _, subject := range subjects {
		out = append(out, webSubject{
			Kind:      subject.Kind,
			Name:      subject.Name,
			Namespace: subject.Namespace,
		})
	}
	return out
}

func toWebSnapshot(cs state.ClusterState, mode string) webSnapshot {
	snap := webSnapshot{
		Mode:                      mode,
		Context:                   cs.ClusterName,
		Version:                   cs.ServerVersion,
		CurrentNamespace:          cs.Namespace,
		FluxInstalled:             cs.FluxInstalled,
		MetricsInstalled:          cs.MetricsInstalled,
		Pods:                      []webPod{},
		Nodes:                     []webNode{},
		Namespaces:                []webNamespace{},
		Events:                    []webEvent{},
		Flux:                      []webFlux{},
		Deployments:               []webDeployment{},
		StatefulSets:              []webStatefulSet{},
		DaemonSets:                []webDaemonSet{},
		ReplicaSets:               []webReplicaSet{},
		Jobs:                      []webJob{},
		CronJobs:                  []webCronJob{},
		Services:                  []webService{},
		Ingresses:                 []webIngress{},
		Endpoints:                 []webEndpoint{},
		NetworkPolicies:           []webNetworkPolicy{},
		ConfigMaps:                []webConfigMap{},
		Secrets:                   []webSecret{},
		ResourceQuotas:            []webResourceQuota{},
		LimitRanges:               []webLimitRange{},
		HorizontalPodAutoscalers:  []webHorizontalPodAutoscaler{},
		PodDisruptionBudgets:      []webPodDisruptionBudget{},
		ServiceAccounts:           []webServiceAccount{},
		Roles:                     []webRole{},
		ClusterRoles:              []webClusterRole{},
		RoleBindings:              []webRoleBinding{},
		ClusterRoleBindings:       []webClusterRoleBinding{},
		CustomResourceDefinitions: []webCustomResourceDefinition{},
		PersistentVolumeClaims:    []webPersistentVolumeClaim{},
		PersistentVolumes:         []webPersistentVolume{},
		StorageClasses:            []webStorageClass{},
		HelmReleases:              []webHelmRelease{},
	}
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
	}
	for _, n := range cs.Nodes {
		status := "ready"
		if n.Unschedulable {
			status = "schedulingdisabled"
		} else if !n.Ready {
			status = "notready"
		}
		snap.Nodes = append(snap.Nodes, webNode{
			Name: n.Name, Status: status, KubeletVersion: n.KubeletVersion,
			CPU: n.CPUText, Mem: n.MemoryText,
			CPUPct: n.CPUPercent(), MemPct: n.MemPercent(),
			PodCount: len(n.Pods),
		})
	}
	for _, n := range cs.Namespaces {
		snap.Namespaces = append(snap.Namespaces, webNamespace{
			Name: n.Name, Phase: n.Phase, Labels: n.Labels, AgeSec: int(n.AgeSeconds),
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
			Source: f.Source, DependsOn: f.DependsOn, Message: f.Message, Age: f.Age,
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
			OwnerKind: j.OwnerKind, OwnerName: j.OwnerName,
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
	for _, e := range cs.Endpoints {
		subsets := make([]webEndpointSubset, 0, len(e.Subsets))
		for _, subset := range e.Subsets {
			subsets = append(subsets, webEndpointSubset{
				Addresses: webStringSlice(subset.Addresses),
				Ports:     subset.Ports,
			})
		}
		snap.Endpoints = append(snap.Endpoints, webEndpoint{
			Name: e.Name, Namespace: e.Namespace, Subsets: subsets,
			TargetService: e.TargetService, AgeSec: int(e.AgeSeconds),
		})
	}
	for _, n := range cs.NetworkPolicies {
		snap.NetworkPolicies = append(snap.NetworkPolicies, webNetworkPolicy{
			Name: n.Name, Namespace: n.Namespace, PodSelector: webStringMap(n.PodSelector),
			PolicyTypes: webStringSlice(n.PolicyTypes), IngressRules: n.IngressRules,
			EgressRules: n.EgressRules, AgeSec: int(n.AgeSeconds),
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
	for _, r := range cs.ResourceQuotas {
		snap.ResourceQuotas = append(snap.ResourceQuotas, webResourceQuota{
			Name: r.Name, Namespace: r.Namespace, Hard: webStringMap(r.Hard),
			Used: webStringMap(r.Used), AgeSec: int(r.AgeSeconds),
		})
	}
	for _, l := range cs.LimitRanges {
		limits := make([]webLimitRangeItem, 0, len(l.Limits))
		for _, item := range l.Limits {
			limits = append(limits, webLimitRangeItem{
				Type:           item.Type,
				DefaultRequest: webStringMap(item.DefaultRequest),
				DefaultLimit:   webStringMap(item.DefaultLimit),
			})
		}
		snap.LimitRanges = append(snap.LimitRanges, webLimitRange{
			Name: l.Name, Namespace: l.Namespace, Limits: limits, AgeSec: int(l.AgeSeconds),
		})
	}
	for _, h := range cs.HorizontalPodAutoscalers {
		var targetCPUPercent *int
		if h.HasTargetCPUPercent {
			v := int(h.TargetCPUPercent)
			targetCPUPercent = &v
		}
		var currentCPUPercent *int
		if h.HasCurrentCPUPercent {
			v := int(h.CurrentCPUPercent)
			currentCPUPercent = &v
		}
		snap.HorizontalPodAutoscalers = append(snap.HorizontalPodAutoscalers, webHorizontalPodAutoscaler{
			Name: h.Name, Namespace: h.Namespace,
			TargetKind: h.TargetKind, TargetName: h.TargetName,
			MinReplicas: int(h.MinReplicas), MaxReplicas: int(h.MaxReplicas),
			CurrentReplicas:  int(h.CurrentReplicas),
			TargetCPUPercent: targetCPUPercent, CurrentCPUPercent: currentCPUPercent,
			AgeSec: int(h.AgeSeconds),
		})
	}
	for _, p := range cs.PodDisruptionBudgets {
		snap.PodDisruptionBudgets = append(snap.PodDisruptionBudgets, webPodDisruptionBudget{
			Name: p.Name, Namespace: p.Namespace,
			MinAvailable: p.MinAvailable, MaxUnavailable: p.MaxUnavailable,
			CurrentHealthy: int(p.CurrentHealthy), DesiredHealthy: int(p.DesiredHealthy),
			ExpectedPods: int(p.ExpectedPods), Selector: webStringMap(p.Selector),
			AgeSec: int(p.AgeSeconds),
		})
	}
	for _, s := range cs.ServiceAccounts {
		snap.ServiceAccounts = append(snap.ServiceAccounts, webServiceAccount{
			Name: s.Name, Namespace: s.Namespace, Secrets: webStringSlice(s.Secrets),
			ImagePullSecrets: webStringSlice(s.ImagePullSecrets),
			AutomountToken:   s.AutomountToken,
			AgeSec:           int(s.AgeSeconds),
		})
	}
	for _, r := range cs.Roles {
		snap.Roles = append(snap.Roles, webRole{
			Name: r.Name, Namespace: r.Namespace, Rules: webPolicyRules(r.Rules),
			AgeSec: int(r.AgeSeconds),
		})
	}
	for _, r := range cs.ClusterRoles {
		snap.ClusterRoles = append(snap.ClusterRoles, webClusterRole{
			Name: r.Name, Rules: webPolicyRules(r.Rules),
			AggregationLabels: r.AggregationLabels, AgeSec: int(r.AgeSeconds),
		})
	}
	for _, r := range cs.RoleBindings {
		snap.RoleBindings = append(snap.RoleBindings, webRoleBinding{
			Name: r.Name, Namespace: r.Namespace,
			RoleRef:  webRoleRef{Kind: r.RoleRef.Kind, Name: r.RoleRef.Name},
			Subjects: webSubjects(r.Subjects), AgeSec: int(r.AgeSeconds),
		})
	}
	for _, r := range cs.ClusterRoleBindings {
		snap.ClusterRoleBindings = append(snap.ClusterRoleBindings, webClusterRoleBinding{
			Name: r.Name, RoleRef: webRoleRef{Kind: r.RoleRef.Kind, Name: r.RoleRef.Name},
			Subjects: webSubjects(r.Subjects), AgeSec: int(r.AgeSeconds),
		})
	}
	for _, c := range cs.CustomResourceDefinitions {
		snap.CustomResourceDefinitions = append(snap.CustomResourceDefinitions, webCustomResourceDefinition{
			Name: c.Name, Group: c.Group, Scope: c.Scope,
			Versions: webStringSlice(c.Versions), PluralName: c.PluralName,
			SingularName: c.SingularName, ListKind: c.ListKind,
			ShortNames: webStringSlice(c.ShortNames), AgeSec: int(c.AgeSeconds),
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
	for _, h := range cs.HelmReleases {
		snap.HelmReleases = append(snap.HelmReleases, webHelmRelease{
			Name:          h.Name,
			Namespace:     h.Namespace,
			Chart:         h.Chart,
			ChartVersion:  h.ChartVersion,
			AppVersion:    h.AppVersion,
			Revision:      h.Revision,
			Status:        h.Status,
			UpdatedAgeSec: h.UpdatedAgeSec,
			AgeSec:        h.AgeSeconds,
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
		// Consume snapshots until shutdown. The manager never closes its
		// snapshot channel (a context switch swaps the upstream beneath it),
		// so terminate on ctx instead of relying on a channel close.
		for {
			select {
			case <-ctx.Done():
				return
			case snap, ok := <-snapshots:
				if !ok {
					return
				}
				hub.set(toWebSnapshot(snap, source.Label()))
			}
		}
	}()

	mux := http.NewServeMux()
	registerUI(mux)
	registerSprites(mux, cfg)
	registerYscale(mux, cfg)
	manager, _ := source.(*sourceManager)
	registerAPI(ctx, mux, hub, source, manager)

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

func registerAPI(ctx context.Context, mux *http.ServeMux, hub *snapshotHub, source ClusterSource, manager *sourceManager) {
	actions := source.Actions()

	mux.HandleFunc("/api/snapshot", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, hub.get())
	})

	mux.HandleFunc("/api/contexts", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "GET only", http.StatusMethodNotAllowed)
			return
		}
		if manager == nil {
			http.Error(w, "context switching unavailable", http.StatusServiceUnavailable)
			return
		}
		contexts, err := manager.contexts()
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		writeJSON(w, toWebContexts(contexts))
	})

	mux.HandleFunc("/api/contexts/select", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST only", http.StatusMethodNotAllowed)
			return
		}
		if manager == nil {
			http.Error(w, "context switching unavailable", http.StatusServiceUnavailable)
			return
		}
		var req struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
			http.Error(w, "context name is required", http.StatusBadRequest)
			return
		}
		if err := manager.selectContext(ctx, req.Name); err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		contexts, err := manager.contexts()
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		writeJSON(w, toWebContexts(contexts))
	})

	mux.HandleFunc("/api/kubeconfig", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST only", http.StatusMethodNotAllowed)
			return
		}
		if manager == nil {
			http.Error(w, "kubeconfig switching unavailable", http.StatusServiceUnavailable)
			return
		}
		var req struct {
			Mode string `json:"mode"`
			Path string `json:"path"`
			Raw  string `json:"raw"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		var src k8s.KubeconfigSource
		switch req.Mode {
		case "raw":
			if strings.TrimSpace(req.Raw) == "" {
				http.Error(w, "raw kubeconfig is required", http.StatusBadRequest)
				return
			}
			src = k8s.KubeconfigSource{Raw: req.Raw}
		case "path":
			if strings.TrimSpace(req.Path) == "" {
				http.Error(w, "kubeconfig path is required", http.StatusBadRequest)
				return
			}
			src = k8s.KubeconfigSource{Path: req.Path}
		default:
			http.Error(w, `mode must be "raw" or "path"`, http.StatusBadRequest)
			return
		}
		contexts, err := manager.setKubeconfig(ctx, src)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		writeJSON(w, toWebContexts(contexts))
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

	mux.HandleFunc("/api/object", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		kind, name := q.Get("kind"), q.Get("name")
		if kind == "" || name == "" {
			http.Error(w, "kind and name are required", http.StatusBadRequest)
			return
		}
		apiVersion := q.Get("apiVersion")
		namespace := q.Get("namespace")
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		y, err := actions.ObjectYAML(ctx, apiVersion, kind, namespace, name)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		writeJSON(w, map[string]string{"yaml": y})
	})

	mux.HandleFunc("/api/customresources", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		version, resource := q.Get("version"), q.Get("resource")
		if version == "" || resource == "" {
			http.Error(w, "version and resource are required", http.StatusBadRequest)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		items, err := actions.CustomResources(ctx, q.Get("group"), version, resource, q.Get("namespace"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		writeJSON(w, map[string]any{"items": items})
	})

	mux.HandleFunc("/api/object/apply", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST only", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			YAML string `json:"yaml"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.YAML == "" {
			http.Error(w, "yaml is required", http.StatusBadRequest)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		applied, err := actions.ApplyYAML(ctx, req.YAML)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		writeJSON(w, map[string]string{"yaml": applied})
	})

	mux.HandleFunc("/api/secret", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		namespace, name := q.Get("namespace"), q.Get("name")
		if namespace == "" || name == "" {
			http.Error(w, "namespace and name are required", http.StatusBadRequest)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		data, err := actions.SecretData(ctx, namespace, name)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		writeJSON(w, map[string]any{"data": data})
	})

	mux.HandleFunc("/api/helm/history", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		ns, name := q.Get("namespace"), q.Get("name")
		if ns == "" || name == "" {
			http.Error(w, "namespace and name are required", http.StatusBadRequest)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		revisions, err := actions.HelmHistory(ctx, ns, name)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		writeJSON(w, map[string]any{"revisions": revisions, "helmCli": actions.HelmAvailable()})
	})

	mux.HandleFunc("/api/helm/rollback", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST only", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			Namespace string `json:"namespace"`
			Name      string `json:"name"`
			Revision  int    `json:"revision"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Namespace == "" || req.Name == "" || req.Revision < 1 {
			http.Error(w, "namespace, name and positive revision are required", http.StatusBadRequest)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
		defer cancel()
		out, err := actions.HelmRollback(ctx, req.Namespace, req.Name, req.Revision)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		writeJSON(w, map[string]string{"output": out})
	})

	mux.HandleFunc("/api/helm/uninstall", func(w http.ResponseWriter, r *http.Request) {
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
		ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
		defer cancel()
		out, err := actions.HelmUninstall(ctx, req.Namespace, req.Name)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		writeJSON(w, map[string]string{"output": out})
	})

	mux.HandleFunc("/api/helm/release", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		ns, name, revStr := q.Get("namespace"), q.Get("name"), q.Get("revision")
		if ns == "" || name == "" || revStr == "" {
			http.Error(w, "namespace, name and revision are required", http.StatusBadRequest)
			return
		}
		rev, err := strconv.Atoi(revStr)
		if err != nil || rev < 1 {
			http.Error(w, "revision must be a positive integer", http.StatusBadRequest)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		detail, err := actions.HelmReleaseDetail(ctx, ns, name, rev)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		writeJSON(w, detail)
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

	mux.HandleFunc("/api/resource/delete", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST only", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			APIVersion string `json:"apiVersion"`
			Kind       string `json:"kind"`
			Namespace  string `json:"namespace"`
			Name       string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Kind == "" || req.Name == "" {
			http.Error(w, "kind and name are required", http.StatusBadRequest)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		if err := actions.DeleteResource(ctx, req.APIVersion, req.Kind, req.Namespace, req.Name); err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		writeJSON(w, map[string]string{"status": "ok"})
	})

	mux.HandleFunc("/api/resource/scale", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST only", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			APIVersion string `json:"apiVersion"`
			Kind       string `json:"kind"`
			Namespace  string `json:"namespace"`
			Name       string `json:"name"`
			Replicas   *int32 `json:"replicas"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Kind == "" || req.Name == "" || req.Replicas == nil || *req.Replicas < 0 {
			http.Error(w, "kind, name and non-negative replicas are required", http.StatusBadRequest)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		if err := actions.ScaleResource(ctx, req.APIVersion, req.Kind, req.Namespace, req.Name, *req.Replicas); err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		writeJSON(w, map[string]string{"status": "ok"})
	})

	mux.HandleFunc("/api/resource/restart", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST only", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			APIVersion string `json:"apiVersion"`
			Kind       string `json:"kind"`
			Namespace  string `json:"namespace"`
			Name       string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Kind == "" || req.Name == "" {
			http.Error(w, "kind and name are required", http.StatusBadRequest)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		if err := actions.RestartResource(ctx, req.APIVersion, req.Kind, req.Namespace, req.Name); err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		writeJSON(w, map[string]string{"status": "ok"})
	})

	mux.HandleFunc("/api/node/cordon", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST only", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			Name   string `json:"name"`
			Cordon bool   `json:"cordon"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
			http.Error(w, "name is required", http.StatusBadRequest)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		if err := actions.CordonNode(ctx, req.Name, req.Cordon); err != nil {
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

	mux.HandleFunc("/api/portforward/start", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST only", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			Namespace  string `json:"namespace"`
			Pod        string `json:"pod"`
			RemotePort int    `json:"remotePort"`
			LocalPort  int    `json:"localPort"` // 0 = kernel-assigned
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Namespace == "" || req.Pod == "" || req.RemotePort <= 0 {
			http.Error(w, "namespace, pod and remotePort are required", http.StatusBadRequest)
			return
		}
		// Use a 15-second request-scoped context for the ready-wait only.
		// The forward itself is owned by the registry after Start returns.
		startCtx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		pf, err := actions.PortForwardStart(startCtx, req.Namespace, req.Pod, req.RemotePort, req.LocalPort)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		writeJSON(w, pf)
	})

	mux.HandleFunc("/api/portforward/stop", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST only", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			ID string `json:"id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ID == "" {
			http.Error(w, "id is required", http.StatusBadRequest)
			return
		}
		if err := actions.PortForwardStop(req.ID); err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		writeJSON(w, map[string]string{"status": "ok"})
	})

	mux.HandleFunc("/api/portforward/list", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "GET only", http.StatusMethodNotAllowed)
			return
		}
		forwards := actions.PortForwardList()
		writeJSON(w, map[string]any{"forwards": forwards})
	})
}

func toWebContexts(contexts k8s.ContextList) webContextList {
	out := webContextList{
		Current:  contexts.Current,
		Contexts: make([]webContext, 0, len(contexts.Contexts)),
	}
	for _, ctx := range contexts.Contexts {
		out.Contexts = append(out.Contexts, webContext{
			Name:      ctx.Name,
			Cluster:   ctx.Cluster,
			Namespace: ctx.Namespace,
		})
	}
	return out
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
