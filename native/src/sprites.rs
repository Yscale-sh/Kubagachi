//! Sprite sheet discovery for /api/critters, ported from internal/sprites.
//! Scan walks the critters dir; per critter it reads the keyed sheet header
//! and each per-state animation deck, and detects per-frame content bounds
//! (equal-width frame split, tight opaque bbox per frame).

use std::collections::{BTreeMap, HashMap};
use std::io::Read as _;
use std::path::Path;
use std::sync::Mutex;

use serde::Serialize;

/// Canonical state order used everywhere in the viewers. "failed" is accepted
/// as an on-disk alias for "error".
pub const STATES: [&str; 8] = [
    "running",
    "pending",
    "completed",
    "crashloop",
    "backoff",
    "terminating",
    "unknown",
    "error",
];

fn state_aliases(state: &str) -> &'static [&'static str] {
    match state {
        "running" => &["running"],
        "pending" => &["pending"],
        "completed" => &["completed"],
        "crashloop" => &["crashloop"],
        "backoff" => &["backoff"],
        "terminating" => &["terminating"],
        "unknown" => &["unknown"],
        "error" => &["error", "failed"],
        _ => &[],
    }
}

#[derive(Serialize, Clone, Debug)]
pub struct Dim {
    pub w: u32,
    pub h: u32,
}

#[derive(Serialize, Clone, Debug)]
pub struct FrameBounds {
    pub x0: u32,
    pub y0: u32,
    pub x1: u32,
    pub y1: u32,
}

#[derive(Serialize, Clone, Debug)]
pub struct AnimSrc {
    pub url: String,
    pub w: u32,
    pub h: u32,
    pub frames: u32,
    pub has_alpha: bool,
    #[serde(skip_serializing_if = "Vec::is_empty")]
    pub bounds: Vec<FrameBounds>,
}

#[derive(Serialize, Clone, Debug)]
pub struct Info {
    pub name: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    pub keyed_url: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub keyed_dim: Option<Dim>,
    pub keyed_has_alpha: bool,
    #[serde(skip_serializing_if = "BTreeMap::is_empty")]
    pub anim: BTreeMap<String, AnimSrc>,
}

/// Memoizes frame detection by (path, mtimeNanos), like the Go boundsCache.
pub type BoundsCache = Mutex<HashMap<String, Vec<FrameBounds>>>;

fn mtime_nanos(path: &Path) -> u128 {
    std::fs::metadata(path)
        .and_then(|m| m.modified())
        .ok()
        .and_then(|t| t.duration_since(std::time::UNIX_EPOCH).ok())
        .map(|d| d.as_nanos())
        .unwrap_or(0)
}

/// png_info returns the PNG's width/height plus whether it carries an alpha
/// channel, by reading the IHDR chunk directly.
fn png_info(path: &Path) -> Option<(Dim, bool)> {
    let mut f = std::fs::File::open(path).ok()?;
    let mut head = [0u8; 26];
    f.read_exact(&mut head).ok()?;
    if &head[0..8] != b"\x89PNG\r\n\x1a\n" {
        return None;
    }
    if &head[12..16] != b"IHDR" {
        return None;
    }
    let w = u32::from_be_bytes([head[16], head[17], head[18], head[19]]);
    let h = u32::from_be_bytes([head[20], head[21], head[22], head[23]]);
    let color_type = head[25];
    let has_alpha = color_type == 4 || color_type == 6;
    Some((Dim { w, h }, has_alpha))
}

/// detect_frame_bounds returns one rect per frame with visible content: the
/// sheet is split into `expected` equal-width frames and each frame's tight
/// opaque bbox (alpha > 0) is reported with exclusive x1/y1. Fully transparent
/// frames are skipped.
pub fn detect_frame_bounds(path: &Path, expected: u32, cache: &BoundsCache) -> Vec<FrameBounds> {
    let key = format!("{}:{}", path.display(), mtime_nanos(path));
    if let Some(v) = cache.lock().unwrap().get(&key) {
        return v.clone();
    }
    let v = compute_frame_bounds(path, expected);
    if !v.is_empty() {
        cache.lock().unwrap().insert(key, v.clone());
    }
    v
}

fn compute_frame_bounds(path: &Path, expected: u32) -> Vec<FrameBounds> {
    let img = match image::open(path) {
        Ok(i) => i,
        Err(_) => return Vec::new(),
    };
    let rgba = img.to_rgba8();
    let (w, h) = rgba.dimensions();
    if w == 0 || h == 0 || expected == 0 {
        return Vec::new();
    }
    let buf = rgba.as_raw();
    let stride = (w * 4) as usize;

    let mut out = Vec::new();
    for i in 0..expected {
        let fx0 = i * w / expected;
        let fx1 = (i + 1) * w / expected;
        let (mut min_x, mut min_y) = (u32::MAX, u32::MAX);
        let (mut max_x, mut max_y) = (0u32, 0u32);
        let mut any = false;
        for y in 0..h {
            let row = y as usize * stride;
            for x in fx0..fx1 {
                if buf[row + x as usize * 4 + 3] > 0 {
                    any = true;
                    if x < min_x {
                        min_x = x;
                    }
                    if x > max_x {
                        max_x = x;
                    }
                    if y < min_y {
                        min_y = y;
                    }
                    if y > max_y {
                        max_y = y;
                    }
                }
            }
        }
        if any {
            out.push(FrameBounds {
                x0: min_x,
                y0: min_y,
                x1: max_x + 1, // exclusive
                y1: max_y + 1,
            });
        }
    }
    out
}

/// scan walks dir and returns one Info per critter that has at least one
/// renderable artifact. URLs are rooted at /critters/.
pub fn scan(dir: &Path, cache: &BoundsCache) -> Vec<Info> {
    let mut out = Vec::new();
    let entries = match std::fs::read_dir(dir) {
        Ok(e) => e,
        Err(_) => return out,
    };
    for e in entries.flatten() {
        if !e.file_type().map(|t| t.is_dir()).unwrap_or(false) {
            continue;
        }
        let name = e.file_name().to_string_lossy().to_string();
        let critter_dir = dir.join(&name);
        let mut info = Info {
            name: name.clone(),
            keyed_url: String::new(),
            keyed_dim: None,
            keyed_has_alpha: false,
            anim: BTreeMap::new(),
        };

        let keyed = critter_dir.join("sprite-sheet-keyed.png");
        if let Some((d, has_alpha)) = png_info(&keyed) {
            info.keyed_url = format!(
                "/critters/{}/sprite-sheet-keyed.png?v={}",
                name,
                mtime_nanos(&keyed)
            );
            info.keyed_dim = Some(d);
            info.keyed_has_alpha = has_alpha;
        }

        for state in STATES {
            for alias in state_aliases(state) {
                let p = critter_dir.join(format!("sprite-sheet-{alias}.png"));
                if let Some((d, has_alpha)) = png_info(&p) {
                    info.anim.insert(
                        state.to_string(),
                        AnimSrc {
                            url: format!(
                                "/critters/{}/sprite-sheet-{}.png?v={}",
                                name,
                                alias,
                                mtime_nanos(&p)
                            ),
                            w: d.w,
                            h: d.h,
                            frames: 8,
                            has_alpha,
                            bounds: detect_frame_bounds(&p, 8, cache),
                        },
                    );
                    break;
                }
            }
        }

        // Workload animation decks (bursting, scaling, …) live alongside the
        // base states but aren't in the canonical list. Discover any extra
        // sprite-sheet-<name>.png and serve it under its own key.
        if let Ok(files) = std::fs::read_dir(&critter_dir) {
            let mut extra: Vec<String> = files
                .flatten()
                .filter_map(|f| {
                    let n = f.file_name().to_string_lossy().to_string();
                    (n.starts_with("sprite-sheet-") && n.ends_with(".png")).then_some(n)
                })
                .collect();
            extra.sort();
            for base in extra {
                if base == "sprite-sheet-keyed.png" {
                    continue;
                }
                let stem = base
                    .strip_prefix("sprite-sheet-")
                    .and_then(|s| s.strip_suffix(".png"))
                    .unwrap_or(&base)
                    .to_string();
                if info.anim.contains_key(&stem) {
                    continue;
                }
                let p = critter_dir.join(&base);
                if let Some((d, has_alpha)) = png_info(&p) {
                    info.anim.insert(
                        stem,
                        AnimSrc {
                            url: format!("/critters/{}/{}?v={}", name, base, mtime_nanos(&p)),
                            w: d.w,
                            h: d.h,
                            frames: 8,
                            has_alpha,
                            bounds: detect_frame_bounds(&p, 8, cache),
                        },
                    );
                }
            }
        }

        if info.keyed_url.is_empty() && info.anim.is_empty() {
            continue;
        }
        out.push(info);
    }
    out.sort_by(|a, b| a.name.to_lowercase().cmp(&b.name.to_lowercase()));
    out
}
