/**
 * KubagachiTab — the dedicated full-tab "my pinned pods" view.
 *
 * Triggered by MainView when `activeTab.kind === "kubagachi"`.
 *
 * Empty state encourages pinning. Otherwise renders a responsive grid of
 * pet cards (one per pinned pod) with a big animated critter, the literal
 * pod status, and k8s ops (Restart / Logs / Shell) + Unpin.
 *
 * No mood / vitals / care system here — this view is a pure pod-status
 * mascot view; everything is driven off `pod.status`.
 */

import { Pin, PinOff, RefreshCw, ScrollText, Terminal } from "lucide-react";
import { deletePod } from "../lib/cluster-api";
import { formatAge } from "../lib/format";
import type { Pod, PodStatus } from "../lib/types";
import {
  useCluster,
  usePinnedPods,
  workspaceActions,
  type PinnedPet,
} from "../store/workspace";
import ConfirmButton from "./ConfirmButton";
import CritterPlayer from "./CritterPlayer";
import StatusPill from "./StatusPill";

// ---------------------------------------------------------------------------
// Tab
// ---------------------------------------------------------------------------

export default function KubagachiTab() {
  const pets = usePinnedPods();

  if (pets.length === 0) return <EmptyState />;

  return (
    <div className="p-4 sm:p-6">
      <Header count={pets.length} />
      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-4 mt-4">
        {pets.map((pet) => (
          <KubagachiCard key={pet.pod.uid} pet={pet} />
        ))}
      </div>
    </div>
  );
}

function Header({ count }: { count: number }) {
  return (
    <div className="flex items-center gap-2">
      <Pin className="w-4 h-4 text-accent" />
      <h2 className="text-[14px] font-semibold text-text">Kubagachi</h2>
      <span className="text-[11px] text-text-muted">
        {count} pinned pod{count === 1 ? "" : "s"}
      </span>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Empty state
// ---------------------------------------------------------------------------

function EmptyState() {
  return (
    <div className="flex-1 min-h-[60vh] flex flex-col items-center justify-center p-8 text-center">
      <div className="relative">
        <div className="text-[56px] sm:text-[72px] font-mono font-semibold text-text tracking-tight leading-none">
          kube<span className="text-accent">kritters</span>
        </div>
        <div className="absolute -top-3 -right-6 sm:-right-10 text-accent">
          <Pin className="w-8 h-8 sm:w-10 sm:h-10" />
        </div>
      </div>
      <div className="mt-6 max-w-md text-[13px] text-text-muted leading-relaxed">
        Pin pods from any list to bring them home as your Kubagachi pets.
      </div>
      <button
        type="button"
        onClick={() => workspaceActions.openTab("Pod")}
        className="mt-6 inline-flex items-center gap-2 px-3 py-2 text-[12px] rounded border border-border bg-bg-panel hover:border-accent hover:text-accent transition-colors"
      >
        Browse pods
      </button>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Card
// ---------------------------------------------------------------------------

/** Tailwind border + box-shadow per status — gives each card a status glow. */
const STATUS_BORDER: Record<PodStatus, string> = {
  running:     "border-status-running/80",
  pending:     "border-status-pending/80",
  completed:   "border-status-completed/80",
  crashloop:   "border-status-crashloop/90",
  backoff:     "border-status-backoff/80",
  terminating: "border-status-terminating/70",
  unknown:     "border-status-unknown/60",
  error:       "border-status-error/90",
};

const STATUS_GLOW: Record<PodStatus, string> = {
  running:     "0 0 0 1px rgba(74,222,128,0.25), 0 10px 30px -8px rgba(74,222,128,0.35)",
  pending:     "0 0 0 1px rgba(251,191,36,0.30), 0 10px 30px -8px rgba(251,191,36,0.30)",
  completed:   "0 0 0 1px rgba(52,211,153,0.25), 0 10px 30px -8px rgba(52,211,153,0.30)",
  crashloop:   "0 0 0 1px rgba(248,113,113,0.45), 0 12px 36px -8px rgba(248,113,113,0.55)",
  backoff:     "0 0 0 1px rgba(251,146,60,0.30), 0 10px 30px -8px rgba(251,146,60,0.30)",
  terminating: "0 0 0 1px rgba(156,163,175,0.25), 0 10px 30px -8px rgba(156,163,175,0.25)",
  unknown:     "0 0 0 1px rgba(209,213,219,0.20), 0 10px 30px -8px rgba(209,213,219,0.18)",
  error:       "0 0 0 1px rgba(244,63,94,0.45), 0 12px 36px -8px rgba(244,63,94,0.55)",
};

/** Soft radial tint behind the mascot, colored by pod status. */
function statusTintBg(status: PodStatus): string {
  const stop = (rgba: string): string =>
    `radial-gradient(circle at 50% 40%, ${rgba} 0%, rgba(7,9,13,0) 72%), ` +
    // soft scanlines — give it that pixel-art CRT vibe
    `repeating-linear-gradient(0deg, rgba(255,255,255,0.020) 0 1px, transparent 1px 3px)`;
  switch (status) {
    case "running":     return stop("rgba(74, 222, 128, 0.30)");
    case "pending":     return stop("rgba(251, 191, 36, 0.30)");
    case "completed":   return stop("rgba(52, 211, 153, 0.30)");
    case "crashloop":   return stop("rgba(248, 113, 113, 0.38)");
    case "backoff":     return stop("rgba(251, 146, 60, 0.28)");
    case "terminating": return stop("rgba(156, 163, 175, 0.24)");
    case "unknown":     return stop("rgba(209, 213, 219, 0.22)");
    case "error":       return stop("rgba(244, 63, 94, 0.38)");
  }
}

function podHash(uid: string): number {
  let h = 2166136261;
  for (let i = 0; i < uid.length; i++) {
    h ^= uid.charCodeAt(i);
    h = (h * 16777619) >>> 0;
  }
  return h;
}

function KubagachiCard({ pet }: { pet: PinnedPet }) {
  const { pod } = pet;
  const cluster = useCluster();
  const isMock = !cluster || cluster.mode === "demo";
  const isCrash = pod.status === "crashloop";

  const onCardClick = (): void => {
    workspaceActions.selectResource(pod.uid);
  };

  // Restart = delete the pod and let the controller recreate it. In demo/mock
  // mode there is no real cluster to mutate, so be honest about the no-op.
  const onRestart = (): void => {
    if (isMock) {
      workspaceActions.toast(
        `Demo mode — restart of ${pod.name} is a no-op`,
        "info",
      );
      return;
    }
    void (async () => {
      const res = await deletePod(pod.namespace ?? "", pod.name);
      if (res.ok) {
        workspaceActions.toast(
          `Restarting ${pod.name} (pod deleted)`,
          "success",
        );
      } else {
        workspaceActions.toast(
          `Restart failed: ${res.error ?? "unknown error"}`,
          "error",
        );
      }
    })();
  };

  const onLogs = (): void => {
    workspaceActions.selectResource(pod.uid, "logs");
  };

  const onShell = (): void => {
    workspaceActions.openTerminal({
      namespace: pod.namespace ?? "",
      pod: pod.name,
      container: pod.containers[0]?.name ?? "",
    });
  };

  const onUnpin = (): void => {
    workspaceActions.unpinPod(pod.uid);
  };

  return (
    <div
      role="button"
      tabIndex={0}
      onClick={onCardClick}
      onKeyDown={(e) => {
        if (e.key === "Enter" || e.key === " ") {
          e.preventDefault();
          onCardClick();
        }
      }}
      style={{ boxShadow: STATUS_GLOW[pod.status] }}
      className={
        "relative bg-bg-panel border-2 rounded-xl overflow-hidden flex flex-col text-left " +
        "cursor-pointer transition-all duration-200 hover:-translate-y-0.5 " +
        "kubagachi-card-in " +
        STATUS_BORDER[pod.status] +
        " " +
        (isCrash ? "kubagachi-crash-pulse" : "")
      }
    >
      {/* Mascot stage — square viewport with status-colored radial glow,
          retro scanlines, and an inner pixel frame. Mascot fills the box. */}
      <div
        className="relative aspect-square w-full bg-bg-panel2"
        style={{ backgroundImage: statusTintBg(pod.status) }}
      >
        {/* Inner double-stroke frame for the pixel-art tamagotchi feel */}
        <div className="absolute inset-1.5 rounded-lg border border-white/5 pointer-events-none" />
        {/* Bobbing mascot — animation-delay randomized per pod uid for non-sync */}
        <div
          className="absolute inset-0 flex items-center justify-center kubagachi-bob pixelated"
          style={{
            animationDelay: `${(podHash(pod.uid) % 1800) / 1000}s`,
            animationDuration: `${2 + ((podHash(pod.uid) % 1100) / 1000)}s`,
          }}
        >
          <CritterPlayer critter={pod.critter} status={pod.critterState ?? pod.status} fps={7} />
        </div>
        {/* Status nameplate over the bottom edge */}
        <div className="absolute left-2 right-2 bottom-2 flex items-center justify-between gap-2">
          <StatusPill status={pod.status} compact />
          <span className="text-[10px] uppercase tracking-[0.18em] text-white/45 font-semibold">
            {pod.critter}
          </span>
        </div>
      </div>

      {/* Body */}
      <div className="p-3 flex flex-col gap-2 flex-1">
        <div className="flex-1 min-w-0">
          <div className="text-[13px] font-semibold text-text truncate">
            {pod.name}
          </div>
          <div className="text-[10px] text-text-muted truncate">
            {pod.namespace ?? "—"}
          </div>
        </div>

        <PodMeta pod={pod} />

        <div className="flex flex-wrap gap-1.5 pt-1">
          <ConfirmButton
            onConfirm={onRestart}
            title={
              isMock
                ? "Demo mode — restart is a no-op"
                : `Delete pod ${pod.name} so its controller recreates it`
            }
            aria-label={`Restart ${pod.name}`}
            label={
              <>
                <RefreshCw className="w-3 h-3" />
                Restart
              </>
            }
            confirmLabel={
              <>
                <RefreshCw className="w-3 h-3" />
                restart?
              </>
            }
            className={CHIP_CLASS_DANGER}
            armedClassName="text-status-crashloop border-status-crashloop/60 bg-bg-panel2"
          />
          <ActionChip
            icon={<ScrollText className="w-3 h-3" />}
            label="Logs"
            onClick={onLogs}
          />
          <ActionChip
            icon={<Terminal className="w-3 h-3" />}
            label="Shell"
            onClick={onShell}
          />
          <ActionChip
            icon={<PinOff className="w-3 h-3" />}
            label="Unpin"
            onClick={onUnpin}
            danger
          />
        </div>
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Pieces
// ---------------------------------------------------------------------------

function PodMeta({ pod }: { pod: Pod }) {
  return (
    <div className="grid grid-cols-[60px_1fr] gap-x-2 gap-y-0.5 text-[11px]">
      <span className="text-text-muted uppercase text-[9px] tracking-wider pt-0.5">
        node
      </span>
      <span className="text-text truncate">{pod.node}</span>
      <span className="text-text-muted uppercase text-[9px] tracking-wider pt-0.5">
        ip
      </span>
      <span className="text-text-muted truncate font-mono">
        {pod.podIP ?? "—"}
      </span>
      <span className="text-text-muted uppercase text-[9px] tracking-wider pt-0.5">
        age
      </span>
      <span className="text-text-muted tabular-nums">{formatAge(pod.ageSec)}</span>
    </div>
  );
}

/** Shared chip class strings so ActionChip + ConfirmButton stay identical. */
const CHIP_BASE =
  "inline-flex items-center gap-1 px-2 py-1 text-[11px] rounded border transition-colors duration-100";
const CHIP_CLASS_NEUTRAL =
  CHIP_BASE +
  " border-border text-text-muted hover:text-text hover:border-border-strong hover:bg-bg-panel2";
const CHIP_CLASS_DANGER =
  CHIP_BASE +
  " border-border text-text-muted hover:text-status-crashloop hover:border-status-crashloop/60";

function ActionChip({
  icon,
  label,
  onClick,
  danger,
}: {
  icon: React.ReactNode;
  label: string;
  onClick: () => void;
  danger?: boolean;
}) {
  return (
    <button
      type="button"
      onClick={(e) => {
        e.stopPropagation();
        onClick();
      }}
      className={danger ? CHIP_CLASS_DANGER : CHIP_CLASS_NEUTRAL}
    >
      {icon}
      {label}
    </button>
  );
}
