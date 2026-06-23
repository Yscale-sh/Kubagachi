/**
 * RanchView — the calm "pasture" rendering of the habitat.
 *
 * A toggleable alternative to the dense node-box grid (see HabitatDashboard).
 * Each cluster NODE becomes a grassy elliptical PLATFORM, and that node's PODS
 * stand on it as pixel-art critters — a Pokémon-ranch / PC-storage-box vibe
 * rather than a battle. Phase 1: critters stand scattered at deterministic
 * positions (a hash of pod.uid) so they never jump between live ticks, doing
 * only the shared idle bob. Wandering movement is a later phase.
 *
 * Selection + keyboard parity with the grid is preserved: every critter carries
 * data-pod-uid (and data-row-selected on the active one) so the shared
 * scrollSelectedRowIntoView + j/k cursor keep working, click selects, and
 * double-click inspects.
 */

import type { Node, Pod, PodStatus } from "../lib/types";
import { workspaceActions } from "../store/workspace";
import CritterPlayer from "./CritterPlayer";

// Mirror HabitatDashboard's STATUS_COLOR so sick critters glow the same hue.
const STATUS_COLOR: Record<PodStatus, string> = {
  running: "#5ec46b",
  pending: "#e0b83a",
  completed: "#4ec8c8",
  terminating: "#9a9a9a",
  crashloop: "#e05a5a",
  backoff: "#e0903a",
  error: "#e05a5a",
  unknown: "#6a6a6a",
};

const GOLD = "#c9b88a";

// To avoid a circular import with HabitatDashboard, define the group shape here.
export interface RanchNodeGroup {
  node: Node | null;
  pods: Pod[];
}

interface RanchViewProps {
  groups: RanchNodeGroup[];
  activeUid: string | null;
  activeOwner: string | null;
  onSelectPod: (uid: string) => void;
}

// FNV-ish string hash → unsigned 32-bit. Same family as HabitatDashboard's
// hashStr; used here to derive STABLE scatter positions + animation phases.
function hashStr(s: string): number {
  let h = 2166136261 >>> 0;
  for (let i = 0; i < s.length; i++) {
    h ^= s.charCodeAt(i);
    h = Math.imul(h, 16777619);
  }
  return h >>> 0;
}

// Deterministic placement of a critter on the platform's upper face from its
// uid. We spread x across the turf and lay critters into a few depth BANDS so
// they read as standing in a field (lower = nearer = higher z-index). The
// position is purely a function of uid + its index within the node, so it is
// stable across re-renders and live ticks (no jumping).
function placement(uid: string, indexInNode: number, count: number) {
  const h = hashStr(uid);
  // Horizontal: jittered even spread so they don't stack into a column.
  const slot = count > 1 ? indexInNode / (count - 1) : 0.5;
  const jitter = ((h % 1000) / 1000 - 0.5) * 0.16; // ±8%
  const xPct = clamp(8 + slot * 84 + jitter * 100, 6, 94);

  // Depth bands across the turf's vertical face. More pods → more bands so a
  // busy node doesn't crowd one line.
  const bandCount = Math.min(4, Math.max(2, Math.ceil(count / 5)));
  const band = (h >> 10) % bandCount;
  const bandJitter = (((h >> 4) % 100) / 100 - 0.5) * 0.06;
  const yPct = clamp(34 + (band / Math.max(1, bandCount - 1)) * 42 + bandJitter * 100, 28, 82);

  // Stagger the idle bob so the pasture breathes organically, not in lockstep.
  const bobDelay = `${(h % 1900) / 1000}s`;
  // z by depth so nearer (lower) critters overlap farther (upper) ones.
  const z = Math.round(yPct * 10);
  return { xPct, yPct, bobDelay, z };
}

function clamp(v: number, lo: number, hi: number): number {
  return v < lo ? lo : v > hi ? hi : v;
}

export default function RanchView({
  groups,
  activeUid,
  activeOwner,
  onSelectPod,
}: RanchViewProps) {
  return (
    <div className="flex flex-col gap-5 sm:gap-7 pt-1 pb-4">
      {groups.map((g) => (
        <Platform
          key={g.node?.name ?? "unscheduled"}
          group={g}
          activeUid={activeUid}
          activeOwner={activeOwner}
          onSelectPod={onSelectPod}
        />
      ))}
    </div>
  );
}

function Platform({
  group,
  activeUid,
  activeOwner,
  onSelectPod,
}: {
  group: RanchNodeGroup;
  activeUid: string | null;
  activeOwner: string | null;
  onSelectPod: (uid: string) => void;
}) {
  const { node, pods } = group;
  const ready = node ? node.status === "ready" : false;
  const name = node?.name ?? "(unscheduled)";
  // Healthy ready nodes get lush green turf; not-ready / unscheduled get the
  // dry pasture. CSS gradients sit BEHIND the PNG so it still reads as turf if
  // the image 404s.
  const healthy = ready;
  const turfUrl = healthy ? "/ranch/turf-green.png" : "/ranch/turf-dry.png";
  const turfBody = healthy
    ? "radial-gradient(120% 90% at 50% 18%, #4f7a3e 0%, #3c5f30 46%, #2c4724 78%, #233a1e 100%)"
    : "radial-gradient(120% 90% at 50% 18%, #8a7d4e 0%, #6f6440 46%, #564d33 78%, #443d29 100%)";
  const rim = healthy ? "#1c2f17" : "#352f1f";

  return (
    <section className="relative">
      {/* Node header — Yscale dark TUI bar that caps the platform. */}
      <div className="relative z-20 mx-auto max-w-[min(94%,860px)] flex items-center gap-x-3 gap-y-1 flex-wrap px-3 py-1.5 border border-border-strong bg-bg-panel/80 backdrop-blur-[1px] k9s-square font-mono">
        <span className="text-tui-cyan/80 text-[10px] uppercase tracking-wider">node</span>
        <span className="text-text text-[12px] truncate max-w-[50%] sm:max-w-none">{name}</span>
        {node && (
          <span
            className="text-[11px]"
            style={{ color: ready ? STATUS_COLOR.running : STATUS_COLOR.error }}
          >
            {ready ? "Ready" : "NotReady"}
          </span>
        )}
        <span className="ml-auto text-[10px] text-text-muted">
          pods <span className="text-text tabular-nums">{pods.length}</span>
        </span>
      </div>

      {/* The grassy platform: a wide ellipse of turf the critters stand on. */}
      <div
        className="relative mx-auto -mt-1.5 w-full max-w-[min(94%,860px)] h-[200px] sm:h-[240px]"
        style={{
          // PNG turf layered ON TOP of a CSS turf gradient, so the platform
          // still reads as grass even if the PNG 404s.
          backgroundImage: `url("${turfUrl}"), ${turfBody}`,
          backgroundSize: "cover, cover",
          backgroundPosition: "center, center",
          backgroundRepeat: "no-repeat, no-repeat",
          imageRendering: "pixelated",
          borderRadius: "50% / 38%",
          boxShadow: `inset 0 -14px 26px -10px ${rim}, inset 0 8px 22px -10px rgba(255,255,255,0.10), 0 18px 30px -16px rgba(0,0,0,0.7)`,
          border: `2px solid ${rim}`,
        }}
      >
        {pods.length === 0 ? (
          <div className="absolute inset-0 flex items-center justify-center">
            <span className="text-text-muted/70 text-[11px] italic font-mono px-3 py-1 bg-bg-base/30 k9s-square">
              quiet pasture · no pods grazing here
            </span>
          </div>
        ) : (
          pods.map((p, i) => (
            <Critter
              key={p.uid}
              pod={p}
              indexInNode={i}
              count={pods.length}
              active={p.uid === activeUid}
              sibling={
                p.uid !== activeUid && !!activeOwner && p.ownerName === activeOwner
              }
              onSelect={() => onSelectPod(p.uid)}
            />
          ))
        )}
      </div>
    </section>
  );
}

function Critter({
  pod,
  indexInNode,
  count,
  active,
  sibling,
  onSelect,
}: {
  pod: Pod;
  indexInNode: number;
  count: number;
  active: boolean;
  sibling: boolean;
  onSelect: () => void;
}) {
  const { xPct, yPct, bobDelay, z } = placement(pod.uid, indexInNode, count);
  const color = STATUS_COLOR[pod.status];
  const title = pod.ownerName ?? pod.name;
  // Mirror PodCard: acute (crashloop/error) pulses; backoff still glows sick.
  const acute = pod.status === "crashloop" || pod.status === "error";
  const sick = acute || pod.status === "backoff";

  // The sprite box. Selected critters get a gold spotlight; sick ones a colored
  // glow that overrides the rest. Sibling pods get a soft gold hint.
  let spriteShadow: string | undefined;
  if (active) {
    spriteShadow = `drop-shadow(0 0 8px ${GOLD}) drop-shadow(0 2px 1px rgba(0,0,0,0.5))`;
  } else if (sick) {
    spriteShadow = `drop-shadow(0 0 7px ${color}) drop-shadow(0 2px 1px rgba(0,0,0,0.5))`;
  } else {
    spriteShadow = "drop-shadow(0 2px 1px rgba(0,0,0,0.45))";
  }

  return (
    <button
      type="button"
      data-pod-uid={pod.uid}
      data-row-selected={active ? "true" : undefined}
      onClick={onSelect}
      onDoubleClick={() => workspaceActions.selectResource(pod.uid)}
      title={`${pod.name}  ·  double-click to inspect`}
      className={`group absolute flex flex-col items-center gap-0.5 -translate-x-1/2 -translate-y-full ${
        acute ? "kubagachi-crash-pulse" : ""
      }`}
      style={{
        left: `${xPct}%`,
        top: `${yPct}%`,
        zIndex: active ? 999 : z,
      }}
    >
      {/* Name tag above the critter. */}
      <span
        className={`max-w-[88px] truncate font-mono text-[9px] leading-none px-1 py-0.5 k9s-square border ${
          active
            ? "text-bg-base font-medium"
            : sick
            ? "text-text"
            : "text-text-muted/90"
        }`}
        style={{
          backgroundColor: active ? GOLD : "rgba(10,12,10,0.55)",
          borderColor: active ? GOLD : sibling ? `${GOLD}66` : "transparent",
        }}
      >
        {title}
      </span>

      {/* The bobbing critter sprite. */}
      <div
        className="w-14 h-14 sm:w-16 sm:h-16 kubagachi-bob"
        style={{ animationDelay: bobDelay, filter: spriteShadow }}
      >
        <CritterPlayer critter={pod.critter} status={pod.critterState ?? pod.status} />
      </div>

      {/* Ground shadow / selection spotlight beneath the critter. */}
      <span
        aria-hidden="true"
        className="absolute bottom-[-3px] left-1/2 -translate-x-1/2 rounded-[50%] pointer-events-none"
        style={{
          width: active ? "44px" : "30px",
          height: active ? "13px" : "8px",
          background: active
            ? `radial-gradient(closest-side, ${GOLD}cc, ${GOLD}33 65%, transparent)`
            : sick
            ? `radial-gradient(closest-side, ${color}99, transparent)`
            : "radial-gradient(closest-side, rgba(0,0,0,0.42), transparent)",
        }}
      />
    </button>
  );
}
