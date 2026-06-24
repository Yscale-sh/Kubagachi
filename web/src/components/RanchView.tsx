/**
 * RanchView — the "storage box" rendering of the habitat.
 *
 * A toggleable alternative to the dense node-box grid (see HabitatDashboard),
 * modelled on the Pokémon PC-storage / ranch screen: each cluster NODE is a
 * panel ("box"), and every pod inside it is a pixel-art critter standing on its
 * OWN little grassy LEDGE, laid out in a tidy grid. Calm and alive (idle bob),
 * not a battle. Wandering movement between ledges is a later phase.
 *
 * Selection + keyboard parity with the grid view is preserved: every critter
 * carries data-pod-uid (and data-row-selected on the active one) so the shared
 * scrollSelectedRowIntoView + j/k cursor keep working; click selects, and
 * double-click inspects.
 */

import type { Node, Pod, PodStatus } from "../lib/types";
import { workspaceActions } from "../store/workspace";
import CritterPlayer from "./CritterPlayer";

// Mirror HabitatDashboard's STATUS_COLOR so sick critters glow the same hue.
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

const GOLD = "#c9b88a";

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

// FNV-ish string hash → unsigned 32-bit. Used only to stagger the idle bob so
// the box breathes organically instead of in lockstep.
function hashStr(s: string): number {
  let h = 2166136261 >>> 0;
  for (let i = 0; i < s.length; i++) {
    h ^= s.charCodeAt(i);
    h = Math.imul(h, 16777619);
  }
  return h >>> 0;
}

function worstPodStatus(pods: Pod[]): PodStatus {
  return SEVERITY.find((s) => pods.some((p) => p.status === s)) ?? "running";
}

function nodeHealthColor(status: PodStatus): string {
  if (status === "running" || status === "completed") return STATUS_COLOR.running;
  if (status === "pending" || status === "backoff") return STATUS_COLOR[status];
  if (status === "crashloop" || status === "error") return STATUS_COLOR.error;
  return STATUS_COLOR[status];
}

export default function RanchView({
  groups,
  activeUid,
  activeOwner,
  onSelectPod,
}: RanchViewProps) {
  return (
    <div className="grid gap-3 sm:gap-4 grid-cols-[repeat(auto-fit,minmax(320px,1fr))] items-start pt-1 pb-4">
      {groups.map((g) => (
        <NodeBox
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

function NodeBox({
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
  const ledgeUrl = ready ? "/ranch/ledge-green.png" : "/ranch/ledge-dry.png";
  const railColor = nodeHealthColor(worstPodStatus(pods));

  return (
    <section
      className="relative mx-auto w-full max-w-[min(96%,1000px)] border border-border-strong bg-bg-panel/35 k9s-square overflow-hidden"
      style={{ boxShadow: `0 0 24px -18px ${railColor}` }}
    >
      <div
        aria-hidden="true"
        className="absolute inset-x-0 top-0 h-[3px]"
        style={{ background: `linear-gradient(90deg, transparent, ${railColor}, transparent)` }}
      />
      {/* Node header — the "box" label bar. */}
      <div className="flex items-center gap-x-3 gap-y-1.5 flex-wrap px-3 py-2 border-b border-border bg-bg-panel/70 font-mono">
        <span className="text-tui-cyan text-[10px] uppercase tracking-[0.18em] font-semibold">node</span>
        <span className="text-text text-[13px] font-semibold truncate max-w-[50%] sm:max-w-none">{name}</span>
        {node && (
          <span
            className="text-[11px] font-medium"
            style={{ color: ready ? STATUS_COLOR.running : STATUS_COLOR.error }}
          >
            {ready ? "Ready" : "NotReady"}
          </span>
        )}
        <span className="ml-auto text-[10px] uppercase tracking-[0.14em] text-text-muted">
          pods <span className="text-text tabular-nums text-[12px] font-semibold">{pods.length}</span>
        </span>
      </div>

      {/* The box floor: a subtle vignette so the grassy ledges pop, like the
          dark PC-box background in the games. */}
      <div
        className="p-3 sm:p-4"
        style={{
          background:
            `radial-gradient(120% 90% at 50% 100%, ${railColor}1c, transparent 62%), radial-gradient(130% 120% at 50% 0%, rgba(93,184,232,0.055), transparent 60%), #0c0e0c`,
        }}
      >
        {pods.length === 0 ? (
          <div className="flex items-center justify-center py-8">
            <span className="text-text-muted text-[11px] italic font-mono px-3 py-1 bg-bg-base/50 k9s-square">
              quiet pasture · no pods grazing here
            </span>
          </div>
        ) : (
          <div className="grid gap-2 sm:gap-3 grid-cols-[repeat(auto-fill,minmax(104px,1fr))] sm:grid-cols-[repeat(auto-fill,minmax(124px,1fr))]">
            {pods.map((p) => (
              <Critter
                key={p.uid}
                pod={p}
                ledgeUrl={ledgeUrl}
                active={p.uid === activeUid}
                sibling={p.uid !== activeUid && !!activeOwner && p.ownerName === activeOwner}
                onSelect={() => onSelectPod(p.uid)}
              />
            ))}
          </div>
        )}
      </div>
    </section>
  );
}

function Critter({
  pod,
  ledgeUrl,
  active,
  sibling,
  onSelect,
}: {
  pod: Pod;
  ledgeUrl: string;
  active: boolean;
  sibling: boolean;
  onSelect: () => void;
}) {
  const color = STATUS_COLOR[pod.status];
  const title = pod.ownerName ?? pod.name;
  const acute = pod.status === "crashloop" || pod.status === "error";
  const sick = acute || pod.status === "backoff";
  const bobDelay = `${(hashStr(pod.uid) % 1900) / 1000}s`;

  // Sprite glow: selected = gold, sick = status hue, else a soft drop shadow.
  const spriteShadow = active
    ? `drop-shadow(0 0 8px ${GOLD}) drop-shadow(0 2px 1px rgba(0,0,0,0.5))`
    : sick
      ? `drop-shadow(0 0 7px ${color}) drop-shadow(0 2px 1px rgba(0,0,0,0.5))`
      : "drop-shadow(0 3px 2px rgba(0,0,0,0.5))";

  return (
    <button
      type="button"
      data-pod-uid={pod.uid}
      data-row-selected={active ? "true" : undefined}
      onClick={onSelect}
      onDoubleClick={() => workspaceActions.selectResource(pod.uid)}
      title={`${pod.name}  ·  double-click to inspect`}
      className={`group relative flex flex-col items-center pt-1 ${active ? "z-10" : ""}`}
    >
      {/* Stage: the critter stands ON the ledge. */}
      <div className="relative w-full h-[78px] sm:h-[88px] flex items-end justify-center">
        {/* Selection / sibling halo behind the ledge. */}
        {(active || sibling) && (
          <span
            aria-hidden="true"
            className="absolute bottom-1 left-1/2 -translate-x-1/2 rounded-[50%] pointer-events-none"
            style={{
              width: active ? "92px" : "78px",
              height: active ? "30px" : "24px",
              background: active
                ? `radial-gradient(closest-side, ${GOLD}66, transparent)`
                : `radial-gradient(closest-side, ${GOLD}2e, transparent)`,
            }}
          />
        )}

        {/* The grassy ledge. */}
        <div
          aria-hidden="true"
          className="absolute bottom-0 left-1/2 -translate-x-1/2 w-[92px] sm:w-[104px] h-[30px] sm:h-[34px]"
          style={{
            backgroundImage: `url("${ledgeUrl}")`,
            backgroundSize: "contain",
            backgroundPosition: "center bottom",
            backgroundRepeat: "no-repeat",
            imageRendering: "pixelated",
            filter: active
              ? `drop-shadow(0 0 5px ${GOLD}aa)`
              : sick
                ? `drop-shadow(0 0 5px ${color}88)`
                : "drop-shadow(0 4px 5px rgba(0,0,0,0.55))",
          }}
        />

        {/* The bobbing critter, feet resting on the ledge top. */}
        <div
          className={`relative w-[52px] h-[52px] sm:w-[58px] sm:h-[58px] mb-[12px] sm:mb-[14px] kubagachi-bob ${
            acute ? "kubagachi-crash-pulse rounded-full" : ""
          }`}
          style={{ animationDelay: bobDelay, filter: spriteShadow }}
        >
          <CritterPlayer critter={pod.critter} status={pod.critterState ?? pod.status} />
        </div>
      </div>

      {/* Name tag under the ledge. */}
      <span
        className="mt-1 max-w-full truncate font-mono text-[10px] sm:text-[11px] leading-none px-1.5 py-1 k9s-square border font-medium"
        style={{
          // Selection reads as a terminal cursor/underline (gold text + gold
          // underline), not a chunky sticker badge.
          color: active ? GOLD : sick ? color : "#e07b9a",
          backgroundColor: "transparent",
          borderColor: sibling && !active ? `${GOLD}66` : "transparent",
          boxShadow: active ? `inset 0 -2px 0 0 ${GOLD}` : undefined,
        }}
      >
        {title}
      </span>
    </button>
  );
}
