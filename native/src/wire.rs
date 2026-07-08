//! Wire types: the JSON contract the browser UI consumes. Field names must
//! match the Go server's webSnapshot/webPod/webNode/webContainer exactly
//! (internal/app/web.go).

use serde::Serialize;

#[derive(Serialize, Clone, Debug, Default)]
#[serde(rename_all = "camelCase")]
pub struct WebContainer {
    pub name: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    pub image: String,
    pub ready: bool,
    pub restart_count: i32,
    #[serde(skip_serializing_if = "String::is_empty")]
    pub state: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    pub reason: String,
}

#[derive(Serialize, Clone, Debug, Default)]
#[serde(rename_all = "camelCase")]
pub struct WebPod {
    pub uid: String,
    pub name: String,
    pub namespace: String,
    pub critter: String,
    pub status: String,
    /// The animation deck to play (sprite-sheet-<state>.png). Equal to status
    /// in the native server — workload overlays are not ported yet.
    pub critter_state: String,
    pub phase: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    pub reason: String,
    pub node: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    pub ip: String,
    pub ready: String,
    pub restarts: i32,
    pub age_sec: i64,
    #[serde(skip_serializing_if = "String::is_empty")]
    pub owner: String,
    pub cpu_milli: i64, // -1 == unknown
    pub mem_bytes: i64, // -1 == unknown
    pub containers: Vec<WebContainer>,
}

#[derive(Serialize, Clone, Debug, Default)]
#[serde(rename_all = "camelCase")]
pub struct WebNode {
    pub name: String,
    pub status: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    pub kubelet_version: String,
    pub cpu: String,
    pub mem: String,
    pub cpu_pct: i32, // -1 == unknown
    pub mem_pct: i32, // -1 == unknown
    pub pod_count: usize,
}

/// Empty is a placeholder for resource arrays the native server does not
/// populate; it always serializes as [].
pub type Empty = Vec<serde_json::Value>;

#[derive(Serialize, Clone, Debug, Default)]
#[serde(rename_all = "camelCase")]
pub struct WebSnapshot {
    pub mode: String, // "live" | "demo"
    pub context: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    pub version: String,
    pub current_namespace: String,
    pub flux_installed: bool,
    pub metrics_installed: bool,
    pub pods: Vec<WebPod>,
    pub nodes: Vec<WebNode>,
    pub namespaces: Empty,
    pub events: Empty,
    pub flux: Empty,
    pub deployments: Empty,
    pub stateful_sets: Empty,
    pub daemon_sets: Empty,
    pub replica_sets: Empty,
    pub jobs: Empty,
    pub cron_jobs: Empty,
    pub services: Empty,
    pub ingresses: Empty,
    pub endpoints: Empty,
    pub network_policies: Empty,
    pub config_maps: Empty,
    pub secrets: Empty,
    pub resource_quotas: Empty,
    pub limit_ranges: Empty,
    pub horizontal_pod_autoscalers: Empty,
    pub pod_disruption_budgets: Empty,
    pub service_accounts: Empty,
    pub roles: Empty,
    pub cluster_roles: Empty,
    pub role_bindings: Empty,
    pub cluster_role_bindings: Empty,
    pub custom_resource_definitions: Empty,
    pub persistent_volume_claims: Empty,
    pub persistent_volumes: Empty,
    pub storage_classes: Empty,
    pub helm_releases: Empty,
}

impl WebSnapshot {
    pub fn empty_live(context: &str, version: &str, namespace: &str) -> Self {
        WebSnapshot {
            mode: "live".to_string(),
            context: context.to_string(),
            version: version.to_string(),
            current_namespace: namespace.to_string(),
            ..Default::default()
        }
    }
}

#[derive(Serialize, Clone, Debug)]
pub struct WebContext {
    pub name: String,
    pub cluster: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub namespace: Option<String>,
}

#[derive(Serialize, Clone, Debug)]
pub struct WebContextList {
    pub current: String,
    pub contexts: Vec<WebContext>,
}
