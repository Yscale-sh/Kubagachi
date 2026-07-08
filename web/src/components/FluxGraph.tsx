/**
 * FluxGraph — a deterministic, hand-rolled SVG dependency graph for Flux.
 *
 * Nodes are flux objects; edges encode how GitOps objects relate:
 *   • source   — a GitRepository/OCIRepository/HelmRepository/Bucket feeding a
 *                Kustomization/HelmRelease (from FluxObject.source, "Kind/Name").
 *   • dependsOn — an explicit ordering edge from a dependency to the object that
 *                depends on it (from FluxObject.dependsOn, "namespace/name").
 *
 * Layout is a layered DAG: longest-path layering pushes sources to the left and
 * consumers to the right, so the graph reads source → consumer → its dependents.
 * Everything is computed deterministically (ties sort by kind then name) so the
 * picture is stable across renders.
 *
 * Legibility at scale: when the namespace filter is "all" the nodes are grouped
 * into labelled per-namespace clusters and the canvas scrolls, rather than
 * collapsing ~60 objects into one hairball. Status colours mirror FluxTab's
 * ReadyBadge palette. Click a node to open the shared FluxDetailPanel; hovering
 * highlights the node and its incident edges.
 *
 * No graph library — edges are <path> elements, mirroring the SVG traces in
 * HabitatDashboard. Nodes are absolutely-positioned HTML buttons layered over
 * the SVG so labels truncate cleanly and stay keyboard/click accessible.
 */

import { useEffect, useMemo, useRef, useState } from "react";
import { GitMerge } from "lucide-react";
import type { FluxObject } from "../lib/types";

const SOURCE_KINDS: ReadonlySet<string> = new Set([
  "GitRepository",
  "OCIRepository",
  "HelmRepository",
  "Bucket",
]);

// Status colours — identical vocabulary/values to FluxTab's ReadyBadge.
function nodeColor(f: FluxObject): string {
  if (f.suspended) return "#beb7aa"; // suspended — distinct (status.terminating)
  if (f.ready === "True") return "#63e07a"; // status.running
  if (f.ready === "False") return "#ff6767"; // status.crashloop
  return "#a9a296"; // status.unknown
}

/** Per-status background tint for nodes — a breath of health without shouting. */
function nodeBg(f: FluxObject): string {
  if (f.suspended) return "#141414";
  if (f.ready === "True") return "rgba(99,224,122,0.04)";
  if (f.ready === "False") return "rgba(255,103,103,0.05)";
  return "#141414";
}

/** Per-status border color — status-tinted so the health is felt at a glance. */
function nodeBorderColor(f: FluxObject): string {
  if (f.suspended) return "rgba(190,183,170,0.35)";
  if (f.ready === "True") return "rgba(99,224,122,0.45)";
  if (f.ready === "False") return "rgba(255,103,103,0.5)";
  return "#2e2e2e";
}

// Geometry. Node boxes are generous so kind + name stay readable.
const NODE_W = 184;
const NODE_H = 48;
const COL_GAP = 84;
const ROW_GAP = 18;
const COL_W = NODE_W + COL_GAP;
const ROW_H = NODE_H + ROW_GAP;
const PAD = 26;
const LANE_PAD_X = 16;
const LANE_PAD_Y = 14;
const CLUSTER_LABEL_H = 30;
const CLUSTER_GAP = 30;

// Default and hot edge colors — warm neutrals that read on the dark bg.
const EDGE_COLOR = "#6a5f50";
const EDGE_HOT = "#c9b88a";
const EDGE_FLOW = "#f0d28a";
const EDGE_FLOW_HOT = "#ffe7ad";

type EdgeKind = "source" | "depends";

interface GNode {
  obj: FluxObject;
  x: number;
  y: number;
}
interface GEdge {
  from: string;
  to: string;
  kind: EdgeKind;
}
interface GCluster {
  label: string;
  count: number;
  x: number;
  y: number;
  w: number;
  h: number;
}
interface GLayout {
  nodes: GNode[];
  edges: GEdge[];
  clusters: GCluster[];
  width: number;
  height: number;
}

/** Rank source kinds ahead of consumers for stable within-layer ordering. */
function kindRank(kind: string): number {
  return SOURCE_KINDS.has(kind) ? 0 : 1;
}

/**
 * Resolve the source + dependsOn references of `objects` into directed edges
 * (from → to, where `to` reconciles after `from`). References that don't point
 * at a node in the set are dropped, so the graph never draws dangling edges.
 */
function resolveEdges(objects: FluxObject[]): GEdge[] {
  // kind/name → nodes (a source may exist in several namespaces).
  const byKindName = new Map<string, FluxObject[]>();
  // namespace/name → nodes (dependsOn targets, deterministic first wins).
  const byNsName = new Map<string, FluxObject[]>();
  for (const o of objects) {
    const kn = `${o.kind}/${o.name}`;
    (byKindName.get(kn) ?? byKindName.set(kn, []).get(kn)!).push(o);
    const nn = `${o.namespace}/${o.name}`;
    (byNsName.get(nn) ?? byNsName.set(nn, []).get(nn)!).push(o);
  }

  const edges: GEdge[] = [];
  const seen = new Set<string>();
  const push = (from: string, to: string, kind: EdgeKind): void => {
    if (from === to) return;
    const key = `${from} ${to} ${kind}`;
    if (seen.has(key)) return;
    seen.add(key);
    edges.push({ from, to, kind });
  };

  for (const o of objects) {
    // source → consumer
    const src = (o.source ?? "").trim();
    if (src && src !== "—") {
      const slash = src.indexOf("/");
      if (slash > 0) {
        const refKind = src.slice(0, slash);
        const refName = src.slice(slash + 1);
        const cands = byKindName.get(`${refKind}/${refName}`);
        if (cands && cands.length) {
          // Prefer a source in the same namespace, else a deterministic pick.
          const pick =
            cands.find((c) => c.namespace === o.namespace) ??
            [...cands].sort((a, b) => a.namespace.localeCompare(b.namespace))[0];
          push(pick.uid, o.uid, "source");
        }
      }
    }
    // dependency → dependent
    for (const dep of o.dependsOn ?? []) {
      const cands = byNsName.get(dep);
      if (cands && cands.length) {
        const pick = [...cands].sort((a, b) => a.kind.localeCompare(b.kind))[0];
        push(pick.uid, o.uid, "depends");
      }
    }
  }
  return edges;
}

/** Longest-path layer index per node, restricted to intra-group edges. */
function layerNodes(group: FluxObject[], parents: Map<string, string[]>): Map<string, number> {
  const layer = new Map<string, number>();
  const visit = (uid: string, stack: Set<string>): number => {
    const cached = layer.get(uid);
    if (cached !== undefined) return cached;
    if (stack.has(uid)) return 0; // cycle guard (dependsOn should never cycle)
    stack.add(uid);
    let best = 0;
    for (const p of parents.get(uid) ?? []) best = Math.max(best, visit(p, stack) + 1);
    stack.delete(uid);
    layer.set(uid, best);
    return best;
  };
  for (const o of group) visit(o.uid, new Set());
  return layer;
}

/**
 * Build the full layout: group → layered columns → absolute coordinates, then
 * stack the (optionally per-namespace) clusters vertically.
 */
function computeLayout(objects: FluxObject[], clustered: boolean): GLayout {
  const edges = resolveEdges(objects);
  const uidSet = new Set(objects.map((o) => o.uid));

  // Group objects (one bucket when scoped to a single namespace).
  const groups = new Map<string, FluxObject[]>();
  for (const o of objects) {
    const key = clustered ? o.namespace : "";
    (groups.get(key) ?? groups.set(key, []).get(key)!).push(o);
  }
  const groupKeys = [...groups.keys()].sort((a, b) => a.localeCompare(b));

  const nodes: GNode[] = [];
  const clusters: GCluster[] = [];
  const posByUid = new Map<string, GNode>();
  let cursorY = PAD;
  let maxRight = 0;

  for (const key of groupKeys) {
    const members = groups.get(key)!;
    const memberSet = new Set(members.map((m) => m.uid));

    // Intra-group parent edges drive layering inside this cluster.
    const parents = new Map<string, string[]>();
    for (const e of edges) {
      if (!memberSet.has(e.from) || !memberSet.has(e.to)) continue;
      (parents.get(e.to) ?? parents.set(e.to, []).get(e.to)!).push(e.from);
    }
    const layer = layerNodes(members, parents);

    // Bucket by layer, sort within a layer for a stable picture.
    const byLayer = new Map<number, FluxObject[]>();
    let maxLayer = 0;
    for (const m of members) {
      const l = layer.get(m.uid) ?? 0;
      maxLayer = Math.max(maxLayer, l);
      (byLayer.get(l) ?? byLayer.set(l, []).get(l)!).push(m);
    }
    let maxRows = 0;
    for (const [, arr] of byLayer) {
      arr.sort(
        (a, b) =>
          kindRank(a.kind) - kindRank(b.kind) ||
          a.kind.localeCompare(b.kind) ||
          a.name.localeCompare(b.name),
      );
      maxRows = Math.max(maxRows, arr.length);
    }

    const labelH = clustered ? CLUSTER_LABEL_H : 0;
    const originX = PAD + LANE_PAD_X;
    const originY = cursorY + labelH + LANE_PAD_Y;
    for (let l = 0; l <= maxLayer; l++) {
      const arr = byLayer.get(l);
      if (!arr) continue;
      arr.forEach((obj, row) => {
        const gn: GNode = { obj, x: originX + l * COL_W, y: originY + row * ROW_H };
        nodes.push(gn);
        posByUid.set(obj.uid, gn);
      });
    }

    const contentW = (maxLayer + 1) * COL_W - COL_GAP;
    const contentH = Math.max(maxRows, 1) * ROW_H - ROW_GAP;
    const bandW = LANE_PAD_X * 2 + contentW;
    const bandH = labelH + LANE_PAD_Y * 2 + contentH;
    if (clustered) {
      clusters.push({ label: key || "—", count: members.length, x: PAD, y: cursorY, w: bandW, h: bandH });
    }
    maxRight = Math.max(maxRight, PAD + bandW);
    cursorY += bandH + (clustered ? CLUSTER_GAP : ROW_GAP);
  }

  // Drop edges whose endpoints fell out of the set (defensive).
  const liveEdges = edges.filter((e) => uidSet.has(e.from) && uidSet.has(e.to) && posByUid.has(e.from) && posByUid.has(e.to));

  return {
    nodes,
    edges: liveEdges,
    clusters,
    width: maxRight + PAD,
    height: cursorY + PAD,
  };
}

/** Horizontal cubic bezier from one node's right edge to another's left edge. */
function edgePath(a: GNode, b: GNode): string {
  const x1 = a.x + NODE_W;
  const y1 = a.y + NODE_H / 2;
  const x2 = b.x;
  const y2 = b.y + NODE_H / 2;
  const dx = Math.max(40, Math.abs(x2 - x1) / 2);
  return `M ${x1} ${y1} C ${x1 + dx} ${y1}, ${x2 - dx} ${y2}, ${x2} ${y2}`;
}

// ---------------------------------------------------------------------------
// Zoom control bar — k9s-square styled, matches ViewToggle from FluxTab.
// ---------------------------------------------------------------------------

function ZoomBar({
  zoom,
  onZoom,
}: {
  zoom: number | "fit";
  onZoom: (z: number | "fit") => void;
}) {
  const bump = (delta: number): void => {
    const base = typeof zoom === "number" ? zoom : 1;
    const next = Math.round(Math.max(0.25, Math.min(2.0, base + delta)) * 4) / 4;
    onZoom(next);
  };
  const seg = (id: string, label: string, active: boolean, onClick: () => void) => (
    <button
      key={id}
      type="button"
      onClick={onClick}
      className={
        "px-2.5 py-1 text-[11px] transition-colors " +
        (active ? "bg-accent-dim text-accent" : "text-text-muted hover:text-text")
      }
    >
      {label}
    </button>
  );
  return (
    <div
      className="inline-flex items-center border border-border k9s-square overflow-hidden"
      role="group"
      aria-label="Zoom controls"
    >
      {seg("fit", "fit", zoom === "fit", () => onZoom("fit"))}
      <span className="w-px self-stretch bg-border" aria-hidden="true" />
      {seg("100", "100%", zoom === 1, () => onZoom(1))}
      <span className="w-px self-stretch bg-border" aria-hidden="true" />
      {seg("plus", "+", false, () => bump(0.25))}
      <span className="w-px self-stretch bg-border" aria-hidden="true" />
      {seg("minus", "−", false, () => bump(-0.25))}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Main component
// ---------------------------------------------------------------------------

export default function FluxGraph({
  objects,
  clustered,
  selectedUid,
  onSelect,
}: {
  objects: FluxObject[];
  clustered: boolean;
  selectedUid: string | null;
  onSelect: (uid: string) => void;
}) {
  const layout = useMemo(() => computeLayout(objects, clustered), [objects, clustered]);
  const [hover, setHover] = useState<string | null>(null);
  const [zoom, setZoom] = useState<number | "fit">("fit");

  // Measure the scrollable canvas so we can compute scale-to-fit.
  const containerRef = useRef<HTMLDivElement>(null);
  const [containerW, setContainerW] = useState(0);
  const [containerH, setContainerH] = useState(0);
  useEffect(() => {
    const el = containerRef.current;
    if (!el) return;
    const update = (): void => {
      setContainerW(el.clientWidth);
      setContainerH(el.clientHeight);
    };
    update();
    const obs = new ResizeObserver(update);
    obs.observe(el);
    return () => obs.disconnect();
  }, []);

  const resolvedZoom = useMemo<number>(() => {
    if (zoom !== "fit") return zoom;
    if (!containerW || !containerH || !layout.width || !layout.height) return 1;
    return Math.min(containerW / layout.width, containerH / layout.height, 1);
  }, [zoom, containerW, containerH, layout.width, layout.height]);

  // Adjacency for hover dimming: a node stays lit if it's the hovered node or
  // shares an edge with it.
  const neighbors = useMemo(() => {
    const m = new Map<string, Set<string>>();
    const link = (a: string, b: string): void => {
      (m.get(a) ?? m.set(a, new Set()).get(a)!).add(b);
    };
    for (const e of layout.edges) {
      link(e.from, e.to);
      link(e.to, e.from);
    }
    return m;
  }, [layout.edges]);

  const posByUid = useMemo(() => {
    const m = new Map<string, GNode>();
    for (const n of layout.nodes) m.set(n.obj.uid, n);
    return m;
  }, [layout.nodes]);

  const isLit = (uid: string): boolean =>
    hover === null || hover === uid || (neighbors.get(hover)?.has(uid) ?? false);

  return (
    // 12rem, not 9rem: FluxTab adds its own sub-header (filter chips + view
    // toggle) above this component — the shorter offset overflowed main and
    // produced a second outer scrollbar.
    <div style={{ maxHeight: "calc(100vh - 12rem)", display: "flex", flexDirection: "column" }}>
      {/* Zoom control bar — doesn't scroll with the graph. */}
      <div className="shrink-0 flex items-center justify-end gap-2 px-3 py-1.5 border-b border-border bg-bg-panel">
        <ZoomBar zoom={zoom} onZoom={setZoom} />
      </div>

      {/* Scrollable graph canvas. */}
      <div
        ref={containerRef}
        className="flex-1 min-h-[200px] overflow-auto scrollbar-thin"
        style={{ display: "flex", alignItems: "flex-start", justifyContent: "center" }}
      >
        {layout.nodes.length === 0 ? (
          <div className="flex flex-col items-center justify-center py-16 text-text-muted gap-2">
            <GitMerge className="w-6 h-6 opacity-50" />
            <div className="text-xs">no flux objects in the selected scope.</div>
          </div>
        ) : (
          /*
           * Two-div scale pattern: the outer div tells the scroll container how
           * large the post-scale content is; the inner div applies the transform.
           * margin:auto centers the content when it fits within the canvas.
           */
          <div
            style={{
              position: "relative",
              width: layout.width * resolvedZoom,
              height: layout.height * resolvedZoom,
              flexShrink: 0,
              margin: "auto",
              // Clip the inner (unscaled) layout box: at fit zoom < 1 its
              // absolute extent would otherwise inflate the scrollable area
              // to the pre-scale size, creating phantom scroll range.
              overflow: "hidden",
            }}
          >
            <div
              style={{
                position: "absolute",
                top: 0,
                left: 0,
                width: layout.width,
                height: layout.height,
                transform: `scale(${resolvedZoom})`,
                transformOrigin: "top left",
              }}
            >
              {/* Cluster bands — tinted boxes that make namespace grouping legible. */}
              {layout.clusters.map((c) => (
                <div
                  key={c.label}
                  className="absolute z-0 k9s-square"
                  style={{
                    left: c.x,
                    top: c.y,
                    width: c.w,
                    height: c.h,
                    border: "1px solid rgba(201,184,138,0.18)",
                    background: "rgba(20,20,20,0.85)",
                  }}
                  aria-hidden="true"
                />
              ))}

              {/* Edges. */}
              <svg
                className="absolute top-0 left-0 z-0 pointer-events-none"
                width={layout.width}
                height={layout.height}
                aria-hidden="true"
              >
                <defs>
                  <marker
                    id="fg-arrow"
                    viewBox="0 0 8 8"
                    refX="7"
                    refY="4"
                    markerWidth="6"
                    markerHeight="6"
                    orient="auto-start-reverse"
                  >
                    <path d="M0 0 L8 4 L0 8 z" fill={EDGE_COLOR} />
                  </marker>
                  <marker
                    id="fg-arrow-hot"
                    viewBox="0 0 8 8"
                    refX="7"
                    refY="4"
                    markerWidth="6.5"
                    markerHeight="6.5"
                    orient="auto-start-reverse"
                  >
                    <path d="M0 0 L8 4 L0 8 z" fill={EDGE_HOT} />
                  </marker>
                </defs>
                {layout.edges.map((e, i) => {
                  const a = posByUid.get(e.from);
                  const b = posByUid.get(e.to);
                  if (!a || !b) return null;
                  const hot = hover !== null && (hover === e.from || hover === e.to);
                  const dim = hover !== null && !hot;
                  const pathD = edgePath(a, b);
                  const flowClass =
                    "flux-edge-flow" +
                    (hot ? " flux-edge-flow--hot" : "") +
                    (e.kind === "depends" ? " flux-edge-flow--depends" : "");
                  return (
                    <g key={i}>
                      <path
                        d={pathD}
                        fill="none"
                        stroke={hot ? EDGE_HOT : EDGE_COLOR}
                        strokeWidth={hot ? 2.2 : 1.8}
                        strokeOpacity={dim ? 0.15 : hot ? 1 : 0.85}
                        strokeDasharray={e.kind === "depends" ? "5 4" : undefined}
                        strokeLinecap="round"
                        markerEnd={`url(#${hot ? "fg-arrow-hot" : "fg-arrow"})`}
                      />
                      <path
                        className={flowClass}
                        d={pathD}
                        fill="none"
                        stroke={hot ? EDGE_FLOW_HOT : EDGE_FLOW}
                        strokeWidth={hot ? 3.6 : 3.05}
                        strokeOpacity={dim ? 0.1 : hot ? 0.98 : 0.68}
                        strokeDasharray={e.kind === "depends" ? "4 6 2 24" : "10 7 3 16"}
                        strokeLinecap="round"
                        style={{ animationDelay: `${-((i % 11) * 0.38)}s` }}
                      />
                    </g>
                  );
                })}
              </svg>

              {/* Cluster labels — accent stripe + namespace name. */}
              {layout.clusters.map((c) => (
                <div
                  key={`lbl-${c.label}`}
                  className="absolute z-10 pointer-events-none flex items-center gap-2 px-3 text-[11px]"
                  style={{ left: c.x + 4, top: c.y + 6, height: CLUSTER_LABEL_H - 10 }}
                >
                  <span
                    aria-hidden="true"
                    style={{
                      display: "inline-block",
                      width: 2,
                      alignSelf: "stretch",
                      background: "#c9b88a",
                      borderRadius: 1,
                    }}
                  />
                  <span className="text-accent uppercase tracking-wider font-semibold">{c.label}</span>
                  <span className="text-text-muted/70 tabular-nums">{c.count}</span>
                </div>
              ))}

              {/* Nodes. */}
              {layout.nodes.map((n) => {
                const f = n.obj;
                const color = nodeColor(f);
                const selected = f.uid === selectedUid;
                const lit = isLit(f.uid);
                const failing = f.ready === "False" && !f.suspended;
                return (
                  <button
                    key={f.uid}
                    type="button"
                    onClick={() => onSelect(f.uid)}
                    onMouseEnter={() => setHover(f.uid)}
                    onMouseLeave={() => setHover(null)}
                    title={`${f.kind}/${f.namespace}/${f.name}`}
                    className={
                      "absolute z-10 flex flex-col justify-center gap-0.5 pl-3 pr-2 text-left k9s-square " +
                      "transition-[opacity,filter] duration-150 motion-reduce:transition-none " +
                      "hover:brightness-110 focus:outline-none " +
                      (!selected && failing ? "kubagachi-crash-pulse " : "") +
                      (lit ? "opacity-100 " : "opacity-30 ")
                    }
                    style={{
                      left: n.x,
                      top: n.y,
                      width: NODE_W,
                      height: NODE_H,
                      background: nodeBg(f),
                      borderWidth: selected ? 2 : 1,
                      borderStyle: "solid",
                      borderColor: selected ? "#c9b88a" : nodeBorderColor(f),
                      boxShadow: selected
                        ? "0 0 0 3px rgba(201,184,138,0.18), 0 4px 16px rgba(0,0,0,0.4)"
                        : "0 2px 6px rgba(0,0,0,0.25)",
                    }}
                  >
                    {/* Status spine — 4px wide, full height. */}
                    <span
                      aria-hidden="true"
                      className="absolute left-0 top-0 bottom-0 w-[4px]"
                      style={{ background: color }}
                    />
                    <span className="flex items-center gap-1.5 text-[10px] uppercase tracking-wider text-text-muted/90 leading-none">
                      <span
                        aria-hidden="true"
                        className="inline-block w-1.5 h-1.5 rounded-full shrink-0"
                        style={{ background: color }}
                      />
                      <span className="truncate">{f.kind}</span>
                      {f.suspended && <span className="text-status-terminating">‖</span>}
                    </span>
                    <span className="text-[12px] text-text font-medium truncate leading-tight">{f.name}</span>
                  </button>
                );
              })}
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
