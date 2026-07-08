//! Critter assignment: a straight port of internal/critters/critters.go plus
//! the mascot reservations in internal/k8s/workload.go. The critter names come
//! from critters/manifest.json (the keys of its top-level "critters" object),
//! sorted ascending to match Go's sort.Strings over the loaded pixel set.

use std::path::Path;

use anyhow::Context as _;

/// projectMascots reserves a critter for a workload identity: first match wins
/// (substring on lowercased "namespace owner"), so vendors supersede the Nori
/// family. Mirrors internal/k8s/workload.go.
const PROJECT_MASCOTS: [(&str, &[&str]); 5] = [
    ("postgres", &["postgres"]),
    ("redis", &["redis"]),
    ("cartogopher", &["cartogopher"]),
    ("phoenix", &["yscale-agent"]),
    ("nori", &["yscale", "jak3s", "kubagachi", "kubekritter"]),
];

/// Every reserved mascot, kept out of the general pool.
const RESERVED_MASCOTS: [&str; 5] = ["postgres", "redis", "cartogopher", "phoenix", "nori"];

/// Loads the critter name set from <critters-dir>/manifest.json.
pub fn load_names(critters_dir: &Path) -> anyhow::Result<Vec<String>> {
    let manifest = critters_dir.join("manifest.json");
    let data = std::fs::read(&manifest)
        .with_context(|| format!("read {}", manifest.display()))?;
    let v: serde_json::Value = serde_json::from_slice(&data)
        .with_context(|| format!("parse {}", manifest.display()))?;
    let obj = v
        .get("critters")
        .and_then(|c| c.as_object())
        .with_context(|| format!("{}: no top-level critters object", manifest.display()))?;
    let mut names: Vec<String> = obj.keys().cloned().collect();
    names.sort();
    Ok(names)
}

/// fnv1a-32, byte-for-byte identical to Go's hash/fnv New32a.
fn fnv1a32(key: &str) -> u32 {
    let mut h: u32 = 2166136261;
    for b in key.as_bytes() {
        h ^= u32::from(*b);
        h = h.wrapping_mul(16777619);
    }
    h
}

/// projectMascot returns the reserved critter a workload identity belongs to,
/// or None when nothing matches. Order-sensitive: first entry wins.
fn project_mascot(namespace: &str, owner: &str) -> Option<&'static str> {
    let hay = format!("{namespace} {owner}").to_lowercase();
    for (critter, keywords) in PROJECT_MASCOTS {
        for kw in keywords {
            if hay.contains(kw) {
                return Some(critter);
            }
        }
    }
    None
}

/// assignCritter: a reserved mascot when the workload identity matches (and
/// the sprite is loaded), otherwise a deterministic pick from the general pool
/// (all names minus the reserved mascots) by fnv1a-32 of key.
pub fn assign_critter(names: &[String], namespace: &str, owner: &str, key: &str) -> String {
    if let Some(m) = project_mascot(namespace, owner) {
        if names.iter().any(|n| n == m) {
            return m.to_string();
        }
    }
    let pool: Vec<&String> = names
        .iter()
        .filter(|n| !RESERVED_MASCOTS.contains(&n.as_str()))
        .collect();
    if pool.is_empty() {
        // Fall back to the full set (Go's AssignExcept -> Assign fallback).
        if names.is_empty() {
            return String::new();
        }
        return names[fnv1a32(key) as usize % names.len()].clone();
    }
    pool[fnv1a32(key) as usize % pool.len()].clone()
}
