//! Live cluster read path: pod/node listing and the status/critter mapping,
//! ported from internal/k8s/mapper.go (detectStatus, containerSignal, MapPod,
//! MapNode) and internal/app/web.go (webStatus, toWebSnapshot).

use k8s_openapi::api::core::v1::{ContainerStatus, Node, Pod};
use k8s_openapi::apimachinery::pkg::api::resource::Quantity;
use kube::api::ListParams;
use kube::Api;

use crate::critters;
use crate::wire::{WebContainer, WebNode, WebPod, WebSnapshot};

// Normalized status vocabulary (internal/state/pod.go).
const STATUS_RUNNING: &str = "running";
const STATUS_PENDING: &str = "pending";
const STATUS_COMPLETED: &str = "completed";
const STATUS_FAILED: &str = "failed";
const STATUS_UNKNOWN: &str = "unknown";
const STATUS_CRASHLOOP: &str = "crashloop";
const STATUS_IMAGEPULL: &str = "imagepull";
const STATUS_OOMKILLED: &str = "oomkilled";
const STATUS_TERMINATING: &str = "terminating";

/// webStatus maps normalized statuses onto the web UI vocabulary.
fn web_status(s: &str) -> String {
    match s {
        STATUS_FAILED | STATUS_OOMKILLED => "error".to_string(),
        STATUS_IMAGEPULL => "backoff".to_string(),
        _ => s.to_string(),
    }
}

/// containerSignal reports an overriding health status when a container is in
/// a notable waiting/terminated state (crash loops, image pull failures, OOM).
fn container_signal(cs: &ContainerStatus) -> Option<(String, String)> {
    if let Some(w) = cs.state.as_ref().and_then(|s| s.waiting.as_ref()) {
        if let Some(reason) = w.reason.as_deref() {
            match reason {
                "CrashLoopBackOff" => {
                    return Some((STATUS_CRASHLOOP.to_string(), reason.to_string()))
                }
                "ImagePullBackOff" | "ErrImagePull" | "ImagePullBackoff" => {
                    return Some((STATUS_IMAGEPULL.to_string(), reason.to_string()))
                }
                "CreateContainerError" | "CreateContainerConfigError" | "RunContainerError" => {
                    return Some((STATUS_FAILED.to_string(), reason.to_string()))
                }
                _ => {}
            }
        }
    }
    if let Some(t) = cs.state.as_ref().and_then(|s| s.terminated.as_ref()) {
        if t.reason.as_deref() == Some("OOMKilled") {
            return Some((STATUS_OOMKILLED.to_string(), "OOMKilled".to_string()));
        }
    }
    if let Some(lt) = cs.last_state.as_ref().and_then(|s| s.terminated.as_ref()) {
        if lt.reason.as_deref() == Some("OOMKilled") {
            return Some((STATUS_OOMKILLED.to_string(), "OOMKilled".to_string()));
        }
    }
    None
}

/// detectStatus derives the normalized health status from the pod phase,
/// deletion timestamp and container states.
fn detect_status(pod: &Pod) -> (String, String) {
    if pod.metadata.deletion_timestamp.is_some() {
        return (STATUS_TERMINATING.to_string(), "Terminating".to_string());
    }

    let status = pod.status.as_ref();
    let phase = status
        .and_then(|s| s.phase.clone())
        .unwrap_or_default();
    let mut reason = phase.clone();
    if let Some(r) = status.and_then(|s| s.reason.as_deref()) {
        if !r.is_empty() {
            reason = r.to_string();
        }
    }

    let empty: Vec<ContainerStatus> = Vec::new();
    let inits = status
        .and_then(|s| s.init_container_statuses.as_ref())
        .unwrap_or(&empty);
    let mains = status
        .and_then(|s| s.container_statuses.as_ref())
        .unwrap_or(&empty);
    for cs in inits.iter().chain(mains.iter()) {
        if let Some(sig) = container_signal(cs) {
            return sig;
        }
    }

    match phase.as_str() {
        "Running" => (STATUS_RUNNING.to_string(), "Running".to_string()),
        "Pending" => (STATUS_PENDING.to_string(), reason),
        "Succeeded" => (STATUS_COMPLETED.to_string(), "Completed".to_string()),
        "Failed" => (STATUS_FAILED.to_string(), reason),
        _ => (STATUS_UNKNOWN.to_string(), "Unknown".to_string()),
    }
}

fn map_container(cs: &ContainerStatus) -> WebContainer {
    let mut out = WebContainer {
        name: cs.name.clone(),
        image: cs.image.clone(),
        ready: cs.ready,
        restart_count: cs.restart_count,
        state: "unknown".to_string(),
        reason: String::new(),
    };
    if let Some(state) = cs.state.as_ref() {
        if state.running.is_some() {
            out.state = "running".to_string();
        } else if let Some(w) = state.waiting.as_ref() {
            out.state = "waiting".to_string();
            out.reason = w.reason.clone().unwrap_or_default();
        } else if let Some(t) = state.terminated.as_ref() {
            out.state = "terminated".to_string();
            out.reason = t.reason.clone().unwrap_or_default();
        }
    }
    out
}

fn now_epoch() -> i64 {
    std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .map(|d| d.as_secs() as i64)
        .unwrap_or(0)
}

fn map_pod(pod: &Pod, names: &[String]) -> WebPod {
    let name = pod.metadata.name.clone().unwrap_or_default();
    let namespace = pod.metadata.namespace.clone().unwrap_or_default();
    let owner = pod
        .metadata
        .owner_references
        .as_ref()
        .and_then(|refs| refs.first())
        .map(|r| r.name.clone())
        .unwrap_or_default();

    let age_sec = pod
        .metadata
        .creation_timestamp
        .as_ref()
        .map(|t| now_epoch() - t.0.timestamp())
        .unwrap_or(0);

    let statuses: &[ContainerStatus] = pod
        .status
        .as_ref()
        .and_then(|s| s.container_statuses.as_deref())
        .unwrap_or(&[]);
    let containers: Vec<WebContainer> = statuses.iter().map(map_container).collect();
    let ready_count = containers.iter().filter(|c| c.ready).count();
    let restarts: i32 = statuses.iter().map(|c| c.restart_count).sum();

    let (status, reason) = detect_status(pod);
    // critterState == status for now: workload animation overlays not ported.
    let critter_state = status.clone();

    // Deterministic critter: keyed on the owner so every replica of a
    // Deployment/StatefulSet shares the same animal identity.
    let key = if owner.is_empty() {
        format!("{namespace}/{name}")
    } else {
        format!("{namespace}/{owner}")
    };
    let critter = critters::assign_critter(names, &namespace, &owner, &key);

    WebPod {
        uid: pod.metadata.uid.clone().unwrap_or_default(),
        name,
        namespace,
        critter,
        status: web_status(&status),
        critter_state: web_status(&critter_state),
        phase: pod
            .status
            .as_ref()
            .and_then(|s| s.phase.clone())
            .unwrap_or_default(),
        reason,
        node: pod
            .spec
            .as_ref()
            .and_then(|s| s.node_name.clone())
            .unwrap_or_default(),
        ip: pod
            .status
            .as_ref()
            .and_then(|s| s.pod_ip.clone())
            .unwrap_or_default(),
        ready: format!("{}/{}", ready_count, containers.len()),
        restarts,
        age_sec,
        owner,
        cpu_milli: -1,
        mem_bytes: -1,
        containers,
    }
}

/// quantity_bytes parses a Kubernetes resource quantity into bytes.
fn quantity_bytes(q: &str) -> i64 {
    let s = q.trim();
    const BIN: [(&str, f64); 6] = [
        ("Ki", 1024.0),
        ("Mi", 1048576.0),
        ("Gi", 1073741824.0),
        ("Ti", 1099511627776.0),
        ("Pi", 1125899906842624.0),
        ("Ei", 1152921504606846976.0),
    ];
    const DEC: [(&str, f64); 6] = [
        ("k", 1e3),
        ("M", 1e6),
        ("G", 1e9),
        ("T", 1e12),
        ("P", 1e15),
        ("E", 1e18),
    ];
    for (suf, mul) in BIN.iter().chain(DEC.iter()) {
        if let Some(num) = s.strip_suffix(suf) {
            return num.parse::<f64>().map(|v| (v * mul) as i64).unwrap_or(0);
        }
    }
    if let Some(num) = s.strip_suffix('m') {
        return num.parse::<f64>().map(|v| (v / 1000.0) as i64).unwrap_or(0);
    }
    s.parse::<f64>().map(|v| v as i64).unwrap_or(0)
}

/// humanize_bytes ports Go's humanizeBytes (KiB/MiB/... with one decimal).
fn humanize_bytes(b: i64) -> String {
    const UNIT: i64 = 1024;
    if b < UNIT {
        return format!("{b}B");
    }
    let mut div = UNIT;
    let mut exp = 0usize;
    let mut n = b / UNIT;
    while n >= UNIT {
        div *= UNIT;
        exp += 1;
        n /= UNIT;
    }
    const UNITS: [char; 6] = ['K', 'M', 'G', 'T', 'P', 'E'];
    format!("{:.1}{}iB", b as f64 / div as f64, UNITS[exp])
}

fn map_node(node: &Node, pod_count: usize) -> WebNode {
    let name = node.metadata.name.clone().unwrap_or_default();
    let unschedulable = node
        .spec
        .as_ref()
        .and_then(|s| s.unschedulable)
        .unwrap_or(false);
    let mut ready = false;
    if let Some(conds) = node.status.as_ref().and_then(|s| s.conditions.as_ref()) {
        for c in conds {
            if c.type_ == "Ready" {
                ready = c.status == "True";
            }
        }
    }
    let status = if unschedulable {
        "schedulingdisabled"
    } else if !ready {
        "notready"
    } else {
        "ready"
    };
    let allocatable = node.status.as_ref().and_then(|s| s.allocatable.as_ref());
    let zero = Quantity("0".to_string());
    let cpu = allocatable.and_then(|a| a.get("cpu")).unwrap_or(&zero);
    let mem = allocatable.and_then(|a| a.get("memory")).unwrap_or(&zero);
    WebNode {
        name,
        status: status.to_string(),
        kubelet_version: node
            .status
            .as_ref()
            .and_then(|s| s.node_info.as_ref())
            .map(|i| i.kubelet_version.clone())
            .unwrap_or_default(),
        cpu: format!("{} cpu", cpu.0),
        mem: humanize_bytes(quantity_bytes(&mem.0)),
        cpu_pct: -1,
        mem_pct: -1,
        pod_count,
    }
}

/// Lists pods and nodes and builds the full web snapshot.
pub async fn build_snapshot(
    client: &kube::Client,
    namespace: Option<&str>,
    context: &str,
    version: &str,
    names: &[String],
) -> anyhow::Result<WebSnapshot> {
    let pods_api: Api<Pod> = match namespace {
        Some(ns) => Api::namespaced(client.clone(), ns),
        None => Api::all(client.clone()),
    };
    let nodes_api: Api<Node> = Api::all(client.clone());

    let lp = ListParams::default();
    let pods = pods_api.list(&lp).await?;
    let nodes = nodes_api.list(&lp).await?;

    let mut snap = WebSnapshot::empty_live(context, version, namespace.unwrap_or(""));
    snap.pods = pods.items.iter().map(|p| map_pod(p, names)).collect();
    snap.nodes = nodes
        .items
        .iter()
        .map(|n| {
            let node_name = n.metadata.name.as_deref().unwrap_or("");
            let count = snap
                .pods
                .iter()
                .filter(|p| !node_name.is_empty() && p.node == node_name)
                .count();
            map_node(n, count)
        })
        .collect();
    Ok(snap)
}
