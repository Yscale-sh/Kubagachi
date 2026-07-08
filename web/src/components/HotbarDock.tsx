/**
 * HotbarDock — persistent floating dock that surfaces pinned pods across
 * every view as little animated mascots.
 *
 * Rendered at the App-shell level (outside the main flex column) with fixed
 * positioning so it overlays whichever tab is active. Hides itself when no
 * pods are pinned.
 *
 * Each pet button:
 *   - 56px circular frame with a status-colored glow ring
 *   - crashlooping pods get an attention-grabbing pulse
 *   - bobbing CritterPlayer with a random delay so they don't sync
 *   - click → open Kubagachi tab and select the pet
 *   - right-click / long-press → unpin
 *   - tooltip on hover (pod name + status)
 */

import { useRef } from "react";
import type { Pod } from "../lib/types";
import { humanStatus } from "../lib/format";
import {
  usePinnedPods,
  workspaceActions,
  type PinnedPet,
} from "../store/workspace";
import CritterPlayer from "./CritterPlayer";

// ---------------------------------------------------------------------------
// Dock
// ---------------------------------------------------------------------------

export default function HotbarDock() {
  const pets = usePinnedPods();

  if (pets.length === 0) return null;

  return (
    <div
      aria-label="Kubagachi hotbar"
      className="fixed bottom-3 left-1/2 -translate-x-1/2 z-30 pointer-events-none"
    >
      <div
        className="pointer-events-auto flex items-center gap-2 px-3 py-2 rounded-full
                   bg-bg-panel/85 backdrop-blur border border-border-strong
                   shadow-[0_8px_32px_-4px_rgba(0,0,0,0.55),0_0_0_1px_rgba(201,184,138,0.08)]"
      >
        {pets.map((pet, i) => (
          <HotbarPet key={pet.pod.uid} pet={pet} index={i} />
        ))}
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Pet button
// ---------------------------------------------------------------------------

function ringForStatus(status: Pod["status"]): string {
  switch (status) {
    case "running":
      return "ring-status-running/70";
    case "crashloop":
      return "ring-status-crashloop/80";
    case "error":
      return "ring-status-error/80";
    case "backoff":
      return "ring-status-backoff/70";
    case "terminating":
      return "ring-status-terminating/70";
    case "pending":
      return "ring-status-pending/70";
    case "completed":
      return "ring-status-completed/70";
    case "unknown":
      return "ring-status-unknown/60";
  }
}

function HotbarPet({ pet, index }: { pet: PinnedPet; index: number }) {
  const { pod } = pet;
  const longPressTimer = useRef<number | null>(null);

  const openKubagachi = (): void => {
    workspaceActions.openTab("kubagachi");
    workspaceActions.selectResource(pod.uid);
  };

  const unpin = (): void => {
    workspaceActions.unpinPod(pod.uid);
  };

  const onContextMenu: React.MouseEventHandler<HTMLButtonElement> = (e) => {
    e.preventDefault();
    unpin();
  };

  const onPointerDown: React.PointerEventHandler<HTMLButtonElement> = (e) => {
    if (e.pointerType !== "touch") return;
    longPressTimer.current = window.setTimeout(() => {
      unpin();
      longPressTimer.current = null;
    }, 600);
  };

  const cancelLongPress = (): void => {
    if (longPressTimer.current !== null) {
      window.clearTimeout(longPressTimer.current);
      longPressTimer.current = null;
    }
  };

  const ring = ringForStatus(pod.status);
  const crash = pod.status === "crashloop";
  // Randomize bob phase per-pet using index (stable across renders).
  const bobDelay = `${((index * 173) % 1700) / 1000}s`;

  return (
    <button
      type="button"
      onClick={openKubagachi}
      onContextMenu={onContextMenu}
      onPointerDown={onPointerDown}
      onPointerUp={cancelLongPress}
      onPointerCancel={cancelLongPress}
      onPointerLeave={cancelLongPress}
      className="group relative h-10 w-10 sm:h-14 sm:w-14 shrink-0"
      aria-label={`${pod.name} — ${humanStatus(pod.status)}`}
    >
      {/* Circular frame */}
      <div
        className={
          "absolute inset-0 rounded-full bg-bg-panel2 border border-border " +
          "ring-2 ring-offset-2 ring-offset-bg-panel transition-all duration-150 " +
          ring +
          " " +
          (crash ? "kubagachi-crash-pulse" : "") +
          " group-hover:scale-110 group-hover:border-accent"
        }
        style={{ overflow: "hidden" }}
      >
        <div
          className="absolute inset-1 kubagachi-bob"
          style={{ animationDelay: bobDelay }}
        >
          <CritterPlayer critter={pod.critter} status={pod.critterState ?? pod.status} fps={6} />
        </div>
      </div>

      {/* Tooltip */}
      <span
        role="tooltip"
        className="pointer-events-none absolute bottom-[calc(100%+8px)] left-1/2 -translate-x-1/2
                   hidden group-hover:flex flex-col items-center gap-0.5
                   whitespace-nowrap rounded border border-border bg-bg-panel
                   px-2 py-1 text-[11px] text-text shadow-lg shadow-black/40 z-50"
      >
        <span className="font-medium">{pod.name}</span>
        <span className="text-text-muted">{humanStatus(pod.status)}</span>
      </span>
    </button>
  );
}
