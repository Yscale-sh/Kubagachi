/**
 * HabitatDashboard — kubagachi's home screen and the heart of the product.
 *
 * A three-column "terminal in the browser": the cluster rendered as habitats
 * (nodes) full of critters (pods).
 *
 *   left   — cluster overview counts, critter legend, controls
 *   center — node boxes with CPU/MEM bars + a grid of dashed pod cards,
 *            and the live event log along the bottom
 *   right  — details for the active pod (hero sprite, status, containers,
 *            events) and a small connections graph
 *
 * The active pod is driven by the shared row cursor (selectedRowIndex), so
 * clicking a card and the j/k keyboard layer move the same selection. Enter /
 * the inspect button opens the full DetailDrawer.
 */

import { useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";
import type { Cluster, Event as ClusterEvent, Node, Pod, PodStatus } from "../lib/types";
import { formatAge, formatBytes, formatCPU, humanStatus } from "../lib/format";
import {
  useCluster,
  useHabitatView,
  useNamespace,
  useSearch,
  useSelectedRow,
  workspaceActions,
} from "../store/workspace";
import { registerRowNav, clearRowNav } from "../lib/row-nav";
import { recordMetrics, nodeHistory, podHistory, sparkline } from "../lib/metrics-history";
import CritterPlayer from "./CritterPlayer";
import RanchView from "./RanchView";
import StatusPill from "./StatusPill";

// Vivid terminal status colors, matching the KUBE-TUI mockup.
const STATUS_COLOR: Record<PodStatus, string> = {
  running: "#63e07a",
  pending: "#f0c94a",
  completed: "#57d9da",
  terminating: "#beb7aa",
  crashloop: "#ff6767",
  backoff: "#f39a3d",
  error: "#ff6767",
  unknown: "#a9a296",
};

const TUI_PINK = "#e07b9a";

const OVERVIEW_ROWS: ReadonlyArray<[string, PodStatus]> = [
  ["RUNNING", "running"],
  ["PENDING", "pending"],
  ["CRASHLOOP", "crashloop"],
  ["BACKOFF", "backoff"],
  ["COMPLETED", "completed"],
  ["TERMINATING", "terminating"],
  ["ERROR", "error"],
];

const LEGEND_ROWS: ReadonlyArray<[string, PodStatus]> = [
  ["Running", "running"],
  ["Pending", "pending"],
  ["Completed", "completed"],
  ["CrashLoop", "crashloop"],
  ["BackOff", "backoff"],
  ["Terminating", "terminating"],
  ["Unknown", "unknown"],
];

const CONTROLS: ReadonlyArray<[string, string]> = [
  ["↑↓ / j k", "navigate"],
  ["enter", "details"],
  ["/", "search"],
  [":", "command"],
  ["s", "shell"],
  ["?", "help"],
];

export default function HabitatDashboard() {
  const cluster = useCluster();
  const namespace = useNamespace();
  const search = useSearch();
  const selectedRow = useSelectedRow();
  const habitatView = useHabitatView();
  const [sheetOpen, setSheetOpen] = useState(false);

  // Flatten pods in node-grouped render order; this is the order the row
  // cursor walks and the order the grid draws. The global search box (and `/`)
  // narrow the habitat here, so the headline screen actually responds to it.
  const { groups, flatPods } = useMemo(
    () => groupByNode(cluster, namespace, search),
    [cluster, namespace, search],
  );

  // Aggregate cluster mood — drives the Cluster Vitals hero, the room's glow,
  // and the crisis scanline. Pure derivation over data the snapshot already has.
  const mood = useMemo(() => deriveMood(cluster), [cluster]);

  // Paint the room with the cluster's mood: tint the ambient glow and, on a
  // critical cluster, flag <body> so the grain/vignette intensify (index.css).
  useEffect(() => {
    if (!mood) return;
    const body = document.body;
    if (mood.tier === "critical") body.dataset.cluster = "critical";
    else delete body.dataset.cluster;
    body.style.setProperty("--mood-glow", mood.glow);
    body.style.setProperty("--mood-glow-2", mood.glow2);
    return () => {
      delete body.dataset.cluster;
      body.style.removeProperty("--mood-glow");
      body.style.removeProperty("--mood-glow-2");
    };
  }, [mood]);

  // One-shot crisis scanline: replays only when the cluster newly turns
  // critical (keyed remount), never loops.
  const [crisisKey, setCrisisKey] = useState(0);
  const prevCriticalRef = useRef(false);
  useEffect(() => {
    const isCritical = mood?.tier === "critical";
    if (isCritical && !prevCriticalRef.current) setCrisisKey((k) => k + 1);
    prevCriticalRef.current = !!isCritical;
  }, [mood]);

  // Register the flat pod list with the keyboard layer so j/k/enter work.
  useEffect(() => {
    const reg = {
      ids: flatPods.map((p) => p.uid),
      onEnter: (id: string) => workspaceActions.selectResource(id),
    };
    registerRowNav(reg);
    return () => clearRowNav(reg);
  }, [flatPods]);

  // 'v' toggles grid <-> ranch (mirrors the TUI habitat toggle). Ignore while
  // typing in a field or when a modifier is held so it doesn't hijack shortcuts.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key !== "v" && e.key !== "V") return;
      if (e.metaKey || e.ctrlKey || e.altKey) return;
      const t = e.target as HTMLElement | null;
      if (t) {
        const tag = t.tagName;
        if (tag === "INPUT" || tag === "TEXTAREA" || tag === "SELECT" || t.isContentEditable) {
          return;
        }
      }
      workspaceActions.toggleHabitatView();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

  // Record one metrics sample per snapshot so the bars become rolling
  // sparklines of recent history.
  useEffect(() => {
    if (!cluster) return;
    recordMetrics(
      cluster.nodes.map((n) => ({ name: n.name, cpuPct: n.cpuPct ?? -1, memPct: n.memPct ?? -1 })),
      cluster.pods.map((p) => ({ uid: p.uid, cpuMilli: p.cpuMilli ?? -1 })),
    );
  }, [cluster]);

  if (!cluster) {
    return <div className="p-6 text-text-muted text-[12px] font-mono">connecting to cluster…</div>;
  }

  const activeIdx = flatPods.length ? Math.min(selectedRow, flatPods.length - 1) : -1;
  const activePod = activeIdx >= 0 ? flatPods[activeIdx] : null;
  const indexByUid = new Map(flatPods.map((p, i) => [p.uid, i]));

  const selectPod = (uid: string) => {
    const idx = indexByUid.get(uid);
    if (idx !== undefined) workspaceActions.setSelectedRow(idx);
    setSheetOpen(true);
  };

  // Connection traces: connect each workload's replicas across adjacent node
  // boxes (pods sharing an owner that live on different nodes).
  const tracePairs = useMemo(() => tracePairsFor(groups), [groups]);
  const scrollRef = useRef<HTMLDivElement | null>(null);
  const [traces, setTraces] = useState<TraceSeg[]>([]);
  const [svgH, setSvgH] = useState(0);

  useLayoutEffect(() => {
    const c = scrollRef.current;
    if (!c) return;
    let raf = 0;
    const recompute = () => {
      raf = 0;
      setSvgH(c.scrollHeight);
      setTraces(computeTraces(c, tracePairs));
    };
    const schedule = () => {
      if (!raf) raf = requestAnimationFrame(recompute);
    };
    recompute();
    c.addEventListener("scroll", schedule, { passive: true });
    const ro = new ResizeObserver(schedule);
    ro.observe(c);
    return () => {
      c.removeEventListener("scroll", schedule);
      ro.disconnect();
      if (raf) cancelAnimationFrame(raf);
    };
  }, [tracePairs, cluster]);

  return (
    <div className="flex-1 min-h-0 flex flex-col lg:grid lg:grid-cols-[216px_minmax(0,1fr)_308px] font-mono text-[12px] bg-bg-base">
      <LeftRail cluster={cluster} mood={mood} />
      <section className="relative min-w-0 flex-1 lg:flex-none min-h-0 flex flex-col lg:border-x border-border">
        {mood?.tier === "critical" && (
          <div key={crisisKey} className="kubagachi-scanline" aria-hidden="true" />
        )}
        <ViewToggle view={habitatView} />
        <div ref={scrollRef} className="flex-1 min-h-0 overflow-y-auto scrollbar-thin relative">
          {habitatView === "grid" && (
            <svg
              className="absolute top-0 left-0 w-full pointer-events-none z-0"
              style={{ height: svgH }}
              aria-hidden="true"
            >
              {traces.map((t, i) => (
                <path
                  key={i}
                  d={t.d}
                  fill="none"
                  stroke={t.color}
                  strokeWidth={1.5}
                  strokeOpacity={0.6}
                  strokeLinejoin="round"
                  strokeLinecap="round"
                />
              ))}
            </svg>
          )}
          <div className="relative z-10 p-2 sm:p-3 flex flex-col gap-2.5">
            {flatPods.length === 0 ? (
              <EmptyHabitat search={search} namespace={namespace} />
            ) : habitatView === "ranch" ? (
              <RanchView
                groups={groups}
                activeUid={activePod?.uid ?? null}
                activeOwner={activePod?.ownerName ?? null}
                onSelectPod={selectPod}
              />
            ) : (
              // Node habitats flow into as many columns as the center width
              // allows (auto-fit), so wide desktops fill across instead of one
              // node per full-width row; collapses to a single column on phones.
              // items-start lets ragged-height boxes top-align cleanly.
              <div className="grid gap-2.5 grid-cols-[repeat(auto-fit,minmax(340px,1fr))] items-start">
                {groups.map((g) => (
                  <NodeBox
                    key={g.node?.name ?? "unscheduled"}
                    group={g}
                    activeUid={activePod?.uid ?? null}
                    activeOwner={activePod?.ownerName ?? null}
                    onSelectPod={selectPod}
                  />
                ))}
              </div>
            )}
          </div>
        </div>
        <EventLog events={cluster.events} />
      </section>
      <RightRail pod={activePod} events={cluster.events} cluster={cluster} />

      {/* Mobile: pod details slide up as a bottom sheet on tap. */}
      {sheetOpen && activePod && (
        <MobilePodSheet
          pod={activePod}
          events={cluster.events}
          cluster={cluster}
          onClose={() => setSheetOpen(false)}
        />
      )}
    </div>
  );
}

// ViewToggle — compact segmented control (grid | ranch) pinned to the top-right
// of the center column. Gold marks the active mode; 'v' toggles it too.
function ViewToggle({ view }: { view: "grid" | "ranch" }) {
  const opts: ReadonlyArray<["grid" | "ranch", string]> = [
    ["grid", "grid"],
    ["ranch", "ranch"],
  ];
  return (
    <div className="absolute top-2 right-2 z-30 flex items-stretch border border-border-strong bg-bg-panel/85 backdrop-blur-[1px] k9s-square font-mono text-[10px]">
      {opts.map(([v, label], i) => {
        const on = view === v;
        return (
          <button
            key={v}
            type="button"
            onClick={() => workspaceActions.setHabitatView(v)}
            aria-pressed={on}
            title={`${label} view  ·  press v to toggle`}
            className={`px-2.5 py-1 uppercase tracking-[0.16em] transition-colors ${
              i > 0 ? "border-l border-border" : ""
            } ${on ? "text-bg-base font-medium" : "text-text-muted hover:text-text"}`}
            style={on ? { backgroundColor: "#c9b88a" } : undefined}
          >
            {label}
          </button>
        );
      })}
    </div>
  );
}

// MobilePodSheet wraps PodDetails in a dismissible bottom sheet for small
// screens; it's hidden on lg where the right rail is always present.
function MobilePodSheet({
  pod,
  events,
  cluster,
  onClose,
}: {
  pod: Pod;
  events: ClusterEvent[];
  cluster: Cluster;
  onClose: () => void;
}) {
  return (
    <div className="lg:hidden fixed inset-0 z-40 flex flex-col justify-end">
      <button
        type="button"
        aria-label="Close"
        onClick={onClose}
        className="absolute inset-0 bg-black/50 backdrop-blur-[1px]"
      />
      <div className="relative max-h-[78vh] overflow-y-auto scrollbar-thin bg-bg-panel border-t border-accent/40 kubagachi-card-in font-mono">
        <div className="sticky top-0 flex items-center justify-between px-3 py-2 bg-bg-panel border-b border-border">
          <span className="text-[10px] uppercase tracking-[0.18em] text-tui-cyan">pod details</span>
          <button onClick={onClose} className="text-text-muted hover:text-text text-[14px] leading-none px-1">
            ✕
          </button>
        </div>
        <PodDetails pod={pod} events={events} cluster={cluster} />
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Left rail — overview / legend / controls
// ---------------------------------------------------------------------------

function LeftRail({ cluster, mood }: { cluster: Cluster; mood: Mood | null }) {
  const counts = useMemo(() => statusCounts(cluster), [cluster]);
  const repCritter = cluster.pods[0]?.critter ?? "frontend";
  const fluxFailing = cluster.flux?.filter((f) => f.ready === "False" && !f.suspended).length ?? 0;

  return (
    <aside className="hidden lg:flex overflow-y-auto scrollbar-thin p-3.5 flex-col gap-4 bg-bg-panel/40">
      {mood && <ClusterVitals mood={mood} cluster={cluster} />}

      <Panel title="cluster overview">
        <Row label="NODES" value={cluster.nodes.length} />
        <Row label="PODS" value={cluster.pods.length} onClick={() => workspaceActions.openTab("Pod")} />
        <div className="h-px bg-border my-1" />
        {OVERVIEW_ROWS.map(([label, status]) => (
          <Row
            key={status}
            label={label}
            value={counts[status] ?? 0}
            color={STATUS_COLOR[status]}
            dim={(counts[status] ?? 0) === 0}
          />
        ))}
      </Panel>

      {cluster.fluxInstalled && (
        <Panel title="gitops · flux">
          <button
            onClick={() => workspaceActions.openTab("flux")}
            className="w-full flex items-center justify-between hover:text-accent transition-colors"
          >
            <span className="text-[10px] text-text-muted uppercase tracking-[0.16em]">OBJECTS</span>
            <span className="tabular-nums text-[13px] font-semibold text-text">{cluster.flux?.length ?? 0}</span>
          </button>
          {fluxFailing > 0 && (
            <Row label="FAILING" value={fluxFailing} color={STATUS_COLOR.error} />
          )}
        </Panel>
      )}

      <Panel title="legend">
        <div className="flex flex-col gap-1.5">
          {LEGEND_ROWS.map(([label, status]) => (
            <div key={status} className="flex items-center gap-2">
              <div className="w-6 h-6 shrink-0">
                <CritterPlayer critter={repCritter} status={status} fps={3} />
              </div>
              <span className="text-[11px]" style={{ color: STATUS_COLOR[status] }}>
                {label}
              </span>
            </div>
          ))}
        </div>
      </Panel>

      <Panel title="controls">
        <div className="flex flex-col gap-1">
          {CONTROLS.map(([k, desc]) => (
            <div key={k} className="flex items-center justify-between text-[11px] leading-5">
              <span className="text-accent font-semibold">{k}</span>
              <span className="text-text-muted">{desc}</span>
            </div>
          ))}
        </div>
      </Panel>
    </aside>
  );
}

function Panel({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div className="border border-border bg-bg-panel/60 k9s-square shadow-[0_0_0_1px_rgba(93,184,232,0.03)]">
      <div className="px-3 py-2 border-b border-border text-[10px] uppercase tracking-[0.18em] text-tui-cyan font-semibold">
        {title}
      </div>
      <div className="p-3 flex flex-col gap-1.5">{children}</div>
    </div>
  );
}

function Row({
  label,
  value,
  color,
  dim,
  onClick,
}: {
  label: string;
  value: number;
  color?: string;
  dim?: boolean;
  onClick?: () => void;
}) {
  const inner = (
    <>
      <span className="text-[11px]" style={{ color: color ?? undefined }}>
        {label}
      </span>
      <span className="tabular-nums text-[13px] font-semibold" style={{ color: color ?? undefined }}>
        {value}
      </span>
    </>
  );
  const cls = `w-full flex items-center justify-between ${dim ? "opacity-40" : ""} ${
    onClick ? "hover:text-accent transition-colors" : ""
  }`;
  return onClick ? (
    <button onClick={onClick} className={cls}>
      {inner}
    </button>
  ) : (
    <div className={cls}>{inner}</div>
  );
}

// ---------------------------------------------------------------------------
// Center — node boxes
// ---------------------------------------------------------------------------

interface NodeGroup {
  node: Node | null;
  pods: Pod[];
}

function NodeBox({
  group,
  activeUid,
  activeOwner,
  onSelectPod,
}: {
  group: NodeGroup;
  activeUid: string | null;
  activeOwner: string | null;
  onSelectPod: (uid: string) => void;
}) {
  const { node, pods } = group;
  const ready = node ? node.status === "ready" : false;
  const name = node?.name ?? "(unscheduled)";
  const cpu = node?.cpuPct ?? -1;
  const mem = node?.memPct ?? -1;
  const worst = worstPodStatus(pods);
  const railColor = nodeHealthColor(worst);

  return (
    <div
      className="relative border border-border-strong bg-bg-panel/25 k9s-square overflow-hidden"
      style={{ boxShadow: `0 0 22px -18px ${railColor}` }}
    >
      <div
        aria-hidden="true"
        className="absolute inset-x-0 top-0 h-[3px]"
        style={{ background: `linear-gradient(90deg, transparent, ${railColor}, transparent)` }}
      />
      <div className="flex items-center gap-x-3 gap-y-1.5 flex-wrap px-2.5 sm:px-3 py-2 border-b border-border bg-bg-panel/45">
        <span className="text-tui-cyan text-[10px] uppercase tracking-[0.18em] font-semibold">node</span>
        <span className="text-text text-[13px] font-semibold truncate max-w-[55%] sm:max-w-none">{name}</span>
        {node && (
          <span
            className="text-[11px] font-medium"
            style={{ color: ready ? STATUS_COLOR.running : STATUS_COLOR.error }}
          >
            {ready ? "Ready" : "NotReady"}
          </span>
        )}
        <span className="ml-auto flex items-center gap-3 sm:gap-4">
          <LoadBar label="CPU" pct={cpu} history={node ? nodeHistory(node.name).cpu : []} />
          <LoadBar label="MEM" pct={mem} history={node ? nodeHistory(node.name).mem : []} />
          <span className="hidden sm:inline text-[10px] uppercase tracking-[0.14em] text-text-muted">
            pods <span className="text-text tabular-nums text-[12px] font-semibold">{pods.length}</span>
          </span>
        </span>
      </div>
      <div
        className="p-2.5 sm:p-4 grid gap-2.5 sm:gap-3 grid-cols-[repeat(auto-fill,minmax(104px,1fr))] sm:grid-cols-[repeat(auto-fill,minmax(150px,1fr))]"
        style={{
          background: `radial-gradient(90% 70% at 50% 100%, ${railColor}18, transparent 62%)`,
        }}
      >
        {pods.map((p) => (
          <PodCard
            key={p.uid}
            pod={p}
            active={p.uid === activeUid}
            sibling={
              p.uid !== activeUid &&
              !!activeOwner &&
              p.ownerName === activeOwner
            }
            onSelect={() => onSelectPod(p.uid)}
            onInspect={() => workspaceActions.selectResource(p.uid)}
          />
        ))}
        {pods.length === 0 && (
          <div className="col-span-full flex items-center gap-2 py-3 px-3 border border-dashed border-border/60 text-text-muted/70 text-[11px] k9s-square">
            <span className="text-[14px] opacity-60" aria-hidden="true">⌁</span>
            <span className="italic">quiet habitat · no pods scheduled here</span>
          </div>
        )}
      </div>
    </div>
  );
}

function PodCard({
  pod,
  active,
  sibling,
  onSelect,
  onInspect,
}: {
  pod: Pod;
  active: boolean;
  sibling: boolean;
  onSelect: () => void;
  onInspect: () => void;
}) {
  const color = STATUS_COLOR[pod.status];
  const title = pod.ownerName ?? pod.name;
  // A pod in trouble should be the loudest thing on screen — solid colored
  // border + halo, and an attention pulse for acute (crashloop/error) states.
  const acute = pod.status === "crashloop" || pod.status === "error";
  const sick = acute || pod.status === "backoff";

  let stateClass: string;
  if (active) stateClass = "border-solid border-accent bg-accent/5";
  else if (sick) stateClass = "border-solid";
  else if (sibling) stateClass = "border-dashed border-accent/40 bg-accent/[0.03]";
  else stateClass = "border-dashed border-border bg-bg-panel/20 hover:border-border-strong hover:bg-bg-panel/40";

  const sickStyle = sick
    ? { borderColor: color, boxShadow: `0 0 0 1px ${color}40, 0 0 16px -3px ${color}` }
    : undefined;

  // Desync the idle bob so the grid breathes organically, not in lockstep.
  const bobDelay = `${(hashStr(pod.uid) % 1800) / 1000}s`;

  return (
    <button
      data-row-selected={active ? "true" : undefined}
      data-pod-uid={pod.uid}
      onClick={onSelect}
      onDoubleClick={onInspect}
      title={`${pod.name}  ·  double-click to inspect`}
      className={`group flex flex-col items-center gap-1.5 p-2.5 k9s-square border transition-colors ${stateClass} ${acute ? "kubagachi-crash-pulse" : ""}`}
      style={sickStyle}
    >
      <span className="text-[11px] sm:text-[12px] text-tui-pink truncate max-w-full font-medium">{title}</span>
      <div className="w-16 h-16 sm:w-24 sm:h-24 kubagachi-bob" style={{ animationDelay: bobDelay }}>
        <CritterPlayer critter={pod.critter} status={pod.critterState ?? pod.status} />
      </div>
      <StatusPill status={pod.status} />
    </button>
  );
}

function LoadBar({ label, pct, history }: { label: string; pct: number; history: number[] }) {
  if (pct < 0) {
    return (
      <span className="flex items-center gap-1 text-[10px] text-text-muted leading-5">
        {label} <span className="opacity-50">—</span>
      </span>
    );
  }
  const color = pct >= 85 ? STATUS_COLOR.error : pct >= 65 ? STATUS_COLOR.pending : STATUS_COLOR.running;
  return (
    <span className="flex items-center gap-1 text-[10px] text-text-muted leading-5">
      {label}
      <span className="tabular-nums text-[11px] font-semibold" style={{ color }}>
        {pct}%
      </span>
      <span className="hidden sm:inline tracking-[-1px]" style={{ color }} title={`${label} history`}>
        {sparkline(history.length ? history : [pct], 12, 100)}
      </span>
    </span>
  );
}

// ---------------------------------------------------------------------------
// Right rail — pod details + connections
// ---------------------------------------------------------------------------

function RightRail({
  pod,
  events,
  cluster,
}: {
  pod: Pod | null;
  events: ClusterEvent[];
  cluster: Cluster;
}) {
  return (
    <aside className="hidden lg:flex overflow-y-auto scrollbar-thin flex-col bg-bg-panel/40">
      {!pod ? (
        <div className="p-4 text-text-muted text-[12px]">
          select a critter to inspect it.
        </div>
      ) : (
        <PodDetails pod={pod} events={events} cluster={cluster} />
      )}
    </aside>
  );
}

function PodDetails({
  pod,
  events,
  cluster,
}: {
  pod: Pod;
  events: ClusterEvent[];
  cluster: Cluster;
}) {
  const color = STATUS_COLOR[pod.status];
  const podEvents = events
    .filter((e) => e.involvedObject?.name === pod.name)
    .slice(0, 6);
  const siblings = cluster.pods.filter(
    (p) => p.ownerName && p.ownerName === pod.ownerName && p.uid !== pod.uid,
  );

  return (
    <div className="flex flex-col">
      <div className="p-3 border-b border-border">
        <div className="text-[10px] uppercase tracking-[0.18em] text-tui-cyan mb-2 font-semibold">
          pod details
        </div>
        <div className="text-[13px] leading-snug font-semibold break-all" style={{ color: TUI_PINK }}>
          {pod.name}
        </div>
        <div className="mx-auto my-3 w-28 h-28">
          <CritterPlayer critter={pod.critter} status={pod.critterState ?? pod.status} fps={6} />
        </div>
        <Field label="STATUS" value={humanStatus(pod.status)} color={color} />
        <Field label="RESTARTS" value={String(pod.restartCount)} />
        <Field label="AGE" value={formatAge(pod.ageSec)} />
        {typeof pod.cpuMilli === "number" && pod.cpuMilli >= 0 && (
          <PodLoad pod={pod} />
        )}
        {typeof pod.memBytes === "number" && pod.memBytes >= 0 && (
          <Field label="MEM" value={formatBytes(pod.memBytes)} />
        )}
        <Field label="NODE" value={pod.node || "—"} />
        <Field label="IP" value={pod.podIP || "—"} />
        {pod.ownerName && <Field label="OWNER" value={pod.ownerName} />}
      </div>

      <div className="p-3 border-b border-border">
        <div className="text-[10px] uppercase tracking-[0.18em] text-tui-cyan mb-2 font-semibold">
          containers
        </div>
        {pod.containers.length === 0 && (
          <div className="text-[11px] text-text-muted">(none reported)</div>
        )}
        {pod.containers.map((c) => (
          <div key={c.name} className="mb-2 last:mb-0">
            <div className="flex items-center gap-1.5">
              <span style={{ color: c.ready ? STATUS_COLOR.running : STATUS_COLOR.error }}>●</span>
              <span className="text-[12px] text-text truncate font-medium">{c.name}</span>
            </div>
            <div className="text-[11px] text-text-muted pl-3.5 leading-5">
              {c.state ?? "—"}
              {c.reason ? ` · ${c.reason}` : ""} · ↻{c.restartCount}
            </div>
          </div>
        ))}
      </div>

      {podEvents.length > 0 && (
        <div className="p-3 border-b border-border">
          <div className="text-[10px] uppercase tracking-[0.18em] text-tui-cyan mb-2 font-semibold">
            events
          </div>
          {podEvents.map((e) => (
            <div key={e.uid} className="mb-2 last:mb-0">
              <div className="flex items-baseline gap-1.5">
                <span
                  className="text-[10px] tabular-nums font-medium"
                  style={{ color: e.type === "warning" ? STATUS_COLOR.pending : STATUS_COLOR.running }}
                >
                  {formatAge(e.lastSeenSec)}
                </span>
                <span className="text-[12px] text-text truncate font-medium">{e.reason}</span>
              </div>
              <div className="text-[11px] text-text-muted pl-1 truncate leading-5">{e.message}</div>
            </div>
          ))}
        </div>
      )}

      <div className="p-3">
        <div className="text-[10px] uppercase tracking-[0.18em] text-tui-cyan mb-2 font-semibold">
          connections
        </div>
        <Connections pod={pod} siblings={siblings} />
      </div>

      <div className="px-3 pb-4 mt-auto flex gap-2">
        <button
          onClick={() => workspaceActions.selectResource(pod.uid)}
          className="flex-1 px-2 py-1.5 text-[11px] border border-border hover:border-accent text-text k9s-square transition-colors"
        >
          inspect
        </button>
        <button
          onClick={() =>
            workspaceActions.openTerminal({
              namespace: pod.namespace ?? "",
              pod: pod.name,
              container: pod.containers[0]?.name ?? "",
            })
          }
          className="flex-1 px-2 py-1.5 text-[11px] border border-border hover:border-accent text-text k9s-square transition-colors"
        >
          shell
        </button>
      </div>
    </div>
  );
}

function Connections({ pod, siblings }: { pod: Pod; siblings: Pod[] }) {
  return (
    <div className="flex flex-col items-center gap-1 text-[10px]">
      {pod.ownerName && (
        <>
          <div className="px-2 py-1 border border-border text-text-muted k9s-square">
            {pod.ownerKind ?? "owner"} · {pod.ownerName}
          </div>
          <div className="text-text-muted">↓</div>
        </>
      )}
      <div className="px-2 py-1 border k9s-square" style={{ borderColor: STATUS_COLOR[pod.status], color: STATUS_COLOR[pod.status] }}>
        {pod.name}
      </div>
      {siblings.length > 0 && (
        <>
          <div className="text-text-muted">↓ {siblings.length} replica{siblings.length === 1 ? "" : "s"}</div>
          <div className="flex flex-wrap justify-center gap-1">
            {siblings.slice(0, 4).map((s) => (
              <span
                key={s.uid}
                className="px-1.5 py-0.5 border border-border text-text-muted k9s-square"
                style={{ borderColor: STATUS_COLOR[s.status] }}
              >
                {s.status === "running" ? "●" : "○"}
              </span>
            ))}
          </div>
        </>
      )}
    </div>
  );
}

// PodLoad shows the pod's current CPU plus a rolling sparkline of its recent
// usage history (scaled to the pod's own peak so the trend shape reads).
function PodLoad({ pod }: { pod: Pod }) {
  const history = podHistory(pod.uid);
  return (
    <div className="flex items-baseline gap-2 py-1">
      <span className="text-[10px] text-text-muted uppercase tracking-[0.16em] w-16 shrink-0">CPU</span>
      <span className="text-[12px] text-text tabular-nums font-medium">{formatCPU((pod.cpuMilli ?? 0) / 1000)}</span>
      <span className="text-[11px] tracking-[-1px] ml-auto" style={{ color: STATUS_COLOR.running }} title="cpu history">
        {sparkline(history.length ? history : [pod.cpuMilli ?? 0], 10)}
      </span>
    </div>
  );
}

function Field({ label, value, color }: { label: string; value: string; color?: string }) {
  return (
    <div className="flex items-baseline gap-2 py-1">
      <span className="text-[10px] text-text-muted uppercase tracking-[0.16em] w-16 shrink-0">{label}</span>
      <span className={`text-[12px] break-all font-medium ${color ? "" : "text-text"}`} style={color ? { color } : undefined}>
        {value}
      </span>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Bottom — event log
// ---------------------------------------------------------------------------

function EventLog({ events }: { events: ClusterEvent[] }) {
  const recent = [...events].sort((a, b) => a.lastSeenSec - b.lastSeenSec).slice(0, 40);
  return (
    <div className="shrink-0 h-28 sm:h-36 lg:h-40 border-t border-border bg-bg-panel/55 flex flex-col">
      <div className="px-3 py-1.5 border-b border-border text-[10px] uppercase tracking-[0.18em] text-tui-cyan font-semibold">
        event log
      </div>
      <div className="flex-1 overflow-y-auto scrollbar-thin px-3 py-2 flex flex-col gap-1">
        {recent.length === 0 && <span className="text-text-muted text-[11px]">no events.</span>}
        {recent.map((e) => (
          <div key={e.uid} className="flex items-baseline gap-2 text-[11px] leading-5">
            <span className="text-text-muted tabular-nums shrink-0 font-medium">{formatAge(e.lastSeenSec)}</span>
            <span
              className="shrink-0"
              style={{ color: e.type === "warning" ? STATUS_COLOR.pending : STATUS_COLOR.running }}
            >
              {e.type === "warning" ? "⚠" : "ℹ"}
            </span>
            <span className="text-text-muted truncate">
              <span className="text-text font-medium">{e.reason}</span>{" "}
              <span className="text-tui-pink">
                {e.involvedObject?.kind}/{e.involvedObject?.name}
              </span>{" "}
              — {e.message}
            </span>
          </div>
        ))}
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

function groupByNode(
  cluster: Cluster | null,
  namespace: string,
  search: string,
): { groups: NodeGroup[]; flatPods: Pod[] } {
  if (!cluster) return { groups: [], flatPods: [] };
  const q = search.trim().toLowerCase();
  const pods = cluster.pods.filter((p) => {
    if (namespace !== "all" && p.namespace !== namespace) return false;
    if (q) {
      const hay = `${p.name} ${p.ownerName ?? ""} ${p.namespace ?? ""} ${p.status} ${p.node ?? ""}`.toLowerCase();
      if (!hay.includes(q)) return false;
    }
    return true;
  });

  const byNode = new Map<string, Pod[]>();
  for (const p of pods) {
    const key = p.node || "";
    const arr = byNode.get(key);
    if (arr) arr.push(p);
    else byNode.set(key, [p]);
  }

  const groups: NodeGroup[] = [];
  const flatPods: Pod[] = [];
  for (const node of cluster.nodes) {
    const list = (byNode.get(node.name) ?? []).slice().sort(byName);
    groups.push({ node, pods: list });
    flatPods.push(...list);
  }
  const orphans = (byNode.get("") ?? []).slice().sort(byName);
  if (orphans.length) {
    groups.push({ node: null, pods: orphans });
    flatPods.push(...orphans);
  }
  return { groups, flatPods };
}

function byName(a: Pod, b: Pod): number {
  return a.name.localeCompare(b.name);
}

function statusCounts(cluster: Cluster): Record<PodStatus, number> {
  const out = {} as Record<PodStatus, number>;
  for (const p of cluster.pods) out[p.status] = (out[p.status] ?? 0) + 1;
  return out;
}

// ---------------------------------------------------------------------------
// Cluster mood — the felt state of the whole habitat
// ---------------------------------------------------------------------------

type MoodTier = "thriving" | "warn" | "critical";

interface Mood {
  tier: MoodTier;
  /** Hue for the mood word + glow. */
  color: string;
  /** Bracketed label, e.g. "CRITICAL". */
  label: string;
  /** running+completed / total, 0..1. */
  healthRatio: number;
  /** Color of the unhealthy arc on the vitals ring. */
  unhealthyColor: string;
  /** The single most interesting pod right now — the cluster's "face". */
  champion: Pod | null;
  glow: string;
  glow2: string;
}

// Worst-first severity for picking the champion / unhealthy hue.
const SEVERITY: PodStatus[] = [
  "crashloop",
  "error",
  "backoff",
  "pending",
  "terminating",
  "unknown",
  "completed",
  "running",
];

function worstPodStatus(pods: Pod[]): PodStatus {
  return SEVERITY.find((s) => pods.some((p) => p.status === s)) ?? "running";
}

function nodeHealthColor(status: PodStatus): string {
  if (status === "running" || status === "completed") return STATUS_COLOR.running;
  if (status === "pending" || status === "backoff") return STATUS_COLOR[status];
  if (status === "crashloop" || status === "error") return STATUS_COLOR.error;
  return STATUS_COLOR[status];
}

const MOOD_STYLE: Record<MoodTier, { color: string; label: string; glow: string; glow2: string }> = {
  thriving: { color: "#c9b88a", label: "THRIVING", glow: "rgba(201, 184, 138, 0.16)", glow2: "rgba(93, 184, 232, 0.09)" },
  warn:     { color: "#f39a3d", label: "DEGRADED", glow: "rgba(243, 154, 61, 0.16)", glow2: "rgba(240, 201, 74, 0.08)" },
  critical: { color: "#ff6767", label: "CRITICAL", glow: "rgba(255, 103, 103, 0.18)", glow2: "rgba(224, 123, 154, 0.10)" },
};

function deriveMood(cluster: Cluster | null): Mood | null {
  if (!cluster) return null;
  const pods = cluster.pods;
  const total = pods.length;
  const counts = statusCounts(cluster);
  const healthy = (counts.running ?? 0) + (counts.completed ?? 0);
  const healthRatio = total > 0 ? healthy / total : 1;

  const acute = (counts.crashloop ?? 0) + (counts.error ?? 0);
  const tier: MoodTier =
    acute > 0 ? "critical" : (counts.backoff ?? 0) > 0 || healthRatio < 0.75 ? "warn" : "thriving";

  const worstBad = SEVERITY.find(
    (s) => s !== "running" && s !== "completed" && (counts[s] ?? 0) > 0,
  );
  const unhealthyColor = worstBad ? STATUS_COLOR[worstBad] : "#1c1c1c";

  // Champion = the worst-health pod if any is unwell, else the busiest runner.
  let champion: Pod | null = worstBad ? pods.find((p) => p.status === worstBad) ?? null : null;
  if (!champion) {
    champion =
      pods
        .filter((p) => p.status === "running")
        .reduce<Pod | null>(
          (best, p) => (!best || (p.cpuMilli ?? -1) > (best.cpuMilli ?? -1) ? p : best),
          null,
        ) ??
      pods[0] ??
      null;
  }

  return { tier, ...MOOD_STYLE[tier], healthRatio, unhealthyColor, champion };
}

// Element-wise average of every node's recent CPU history → one cluster heartbeat.
function clusterCpuHistory(cluster: Cluster): number[] {
  const series = cluster.nodes.map((n) => nodeHistory(n.name).cpu).filter((a) => a.length > 0);
  if (series.length === 0) return [];
  const len = Math.min(...series.map((a) => a.length));
  const out: number[] = [];
  for (let i = 0; i < len; i++) {
    let sum = 0;
    for (const a of series) sum += a[a.length - len + i];
    out.push(Math.round(sum / series.length));
  }
  return out;
}

function hashStr(s: string): number {
  let h = 0;
  for (let i = 0; i < s.length; i++) h = (h * 31 + s.charCodeAt(i)) >>> 0;
  return h;
}

// ClusterVitals — the signature moment. A Champion critter (the cluster's face)
// inside a conic-gradient health ring, the mood word, and a cluster-CPU
// heartbeat. One glance tells you if the cluster is happy or dying.
function ClusterVitals({ mood, cluster }: { mood: Mood; cluster: Cluster }) {
  const champ = mood.champion;
  const ratioDeg = Math.round(mood.healthRatio * 360);
  const pct = Math.round(mood.healthRatio * 100);
  const cpuSeries = useMemo(() => clusterCpuHistory(cluster), [cluster]);

  return (
    <div className="border border-border-strong bg-bg-panel/60 k9s-square shadow-[0_0_26px_-18px_rgba(201,184,138,0.9)]">
      <div className="px-3 py-2 border-b border-border flex items-center justify-between">
        <span className="text-[10px] uppercase tracking-[0.18em] text-tui-cyan font-semibold">cluster vitals</span>
        <span className="text-[12px] tabular-nums font-semibold" style={{ color: mood.color }}>
          {pct}%
        </span>
      </div>
      <div className="p-3 flex flex-col items-center gap-2">
        <div className="relative w-28 h-28">
          <div
            className="absolute inset-0 rounded-full"
            style={{
              background: `conic-gradient(#d8c89a 0 ${ratioDeg}deg, ${mood.unhealthyColor} ${ratioDeg}deg 360deg)`,
              boxShadow: `0 0 22px -4px ${mood.color}, 0 0 44px -22px #5db8e8`,
            }}
            aria-hidden="true"
          />
          <div
            className="absolute inset-[9px] rounded-full bg-bg-base overflow-hidden flex items-center justify-center ring-1 ring-black/40"
            style={{ boxShadow: `inset 0 0 22px -12px ${mood.color}` }}
          >
            {champ && (
              <div className="w-[94%] h-[94%] kubagachi-bob">
                <CritterPlayer critter={champ.critter} status={champ.critterState ?? champ.status} />
              </div>
            )}
          </div>
        </div>
        <span className="k9s-bracket text-[12px] tracking-[0.22em] font-semibold" style={{ color: mood.color }}>
          {mood.label}
        </span>
        {champ && (
          <span
            className="text-[12px] break-all text-center leading-tight font-medium"
            style={{ color: TUI_PINK }}
            title={champ.name}
          >
            {champ.ownerName ?? champ.name}
          </span>
        )}
        <div className="self-stretch flex items-center gap-2 text-[10px] text-text-muted pt-2 mt-1 border-t border-border/60">
          <span className="uppercase tracking-[0.16em]">cluster cpu</span>
          <span className="tracking-[-1px] flex-1 text-right" style={{ color: mood.color }}>
            {sparkline(cpuSeries.length ? cpuSeries : [0], 16, 100)}
          </span>
        </div>
      </div>
    </div>
  );
}

// EmptyHabitat — shown when the namespace/search filter leaves no pods, instead
// of a wall of empty node boxes.
function EmptyHabitat({ search, namespace }: { search: string; namespace: string }) {
  const q = search.trim();
  return (
    <div className="flex flex-col items-center justify-center gap-3 py-20 text-center text-text-muted">
      <span className="text-[32px] opacity-50" aria-hidden="true">⌁</span>
      {q ? (
        <>
          <span className="text-[12px]">
            no critters match <span className="text-accent">“{q}”</span>
          </span>
          <button
            onClick={() => workspaceActions.setSearch("")}
            className="text-[11px] border border-border hover:border-accent px-2.5 py-1 k9s-square text-text-muted hover:text-text transition-colors"
          >
            clear search
          </button>
        </>
      ) : (
        <span className="text-[12px]">
          no pods{namespace !== "all" ? ` in ${namespace}` : ""} right now
        </span>
      )}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Connection traces
// ---------------------------------------------------------------------------

interface TracePair {
  a: string;
  b: string;
  color: string;
}
interface TraceSeg {
  d: string;
  color: string;
}

// tracePairsFor links each workload's replicas that live on different node
// boxes: sort an owner's pods by node-group index, then connect consecutive
// pods whose groups differ. That's the cross-node replica topology.
function tracePairsFor(groups: NodeGroup[]): TracePair[] {
  const byOwner = new Map<string, { uid: string; status: PodStatus; gi: number }[]>();
  groups.forEach((g, gi) => {
    for (const p of g.pods) {
      if (!p.ownerName) continue;
      const key = `${p.namespace ?? ""}/${p.ownerName}`;
      const arr = byOwner.get(key);
      const entry = { uid: p.uid, status: p.status, gi };
      if (arr) arr.push(entry);
      else byOwner.set(key, [entry]);
    }
  });
  const pairs: TracePair[] = [];
  for (const arr of byOwner.values()) {
    if (arr.length < 2) continue;
    arr.sort((a, b) => a.gi - b.gi);
    for (let i = 0; i < arr.length - 1; i++) {
      if (arr[i].gi !== arr[i + 1].gi) {
        pairs.push({ a: arr[i].uid, b: arr[i + 1].uid, color: STATUS_COLOR[arr[i].status] });
      }
    }
  }
  return pairs;
}

// computeTraces measures the live card positions and builds a Manhattan path
// (down → across → down) from each upper card's bottom to the lower card's top,
// in the scroll container's content coordinate space.
function computeTraces(container: HTMLElement, pairs: TracePair[]): TraceSeg[] {
  const cr = container.getBoundingClientRect();
  const sl = container.scrollLeft;
  const st = container.scrollTop;
  const pos = (uid: string) => {
    const el = container.querySelector<HTMLElement>(`[data-pod-uid="${CSS.escape(uid)}"]`);
    if (!el) return null;
    const er = el.getBoundingClientRect();
    const x = er.left - cr.left + sl + er.width / 2;
    const top = er.top - cr.top + st;
    return { x, top, bot: top + er.height };
  };
  const out: TraceSeg[] = [];
  for (const pr of pairs) {
    const a = pos(pr.a);
    const b = pos(pr.b);
    if (!a || !b) continue;
    const midY = (a.bot + b.top) / 2;
    out.push({
      d: `M ${a.x} ${a.bot} V ${midY} H ${b.x} V ${b.top}`,
      color: pr.color,
    });
  }
  return out;
}
