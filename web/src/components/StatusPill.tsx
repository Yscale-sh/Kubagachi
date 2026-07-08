/**
 * StatusPill — k9s-style status glyph + short code.
 *
 * Renders a colored glyph (◉ ◯ ◆ ▲ ▼ ✓ ⚠ ?) leading a 3-4 letter status
 * code so dense tables can show state at a glance. `compact` drops the
 * text and renders only the glyph (used in tight cells / inline icons).
 *
 * Maps an arbitrary status string (PodStatus, WorkloadStatus, PV/PVC
 * phase, NodeStatus, JobStatus, HelmReleaseStatus, etc.) onto the
 * Tailwind `status.*` palette plus a friendly code.
 */

interface StatusPillProps {
  status: string;
  compact?: boolean;
}

interface PillStyle {
  text: string;
  glyph: string;
  code: string;
}

const PALETTE: Record<string, PillStyle> = {
  // pods (canonical, lowercase)
  running:           { text: "text-status-running",     glyph: "◉", code: "RUN"  },
  pending:           { text: "text-status-pending",     glyph: "◯", code: "PND"  },
  completed:         { text: "text-status-completed",   glyph: "✓", code: "DONE" },
  crashloop:         { text: "text-status-crashloop",   glyph: "✖", code: "CRSH" },
  backoff:           { text: "text-status-backoff",     glyph: "▼", code: "BOFF" },
  terminating:       { text: "text-status-terminating", glyph: "⌧", code: "TERM" },
  unknown:           { text: "text-status-unknown",     glyph: "?", code: "UNK"  },
  error:             { text: "text-status-error",       glyph: "◆", code: "ERR"  },

  // workloads
  healthy:           { text: "text-status-running",     glyph: "◉", code: "OK"   },
  progressing:       { text: "text-status-pending",     glyph: "◯", code: "PROG" },
  degraded:          { text: "text-status-crashloop",   glyph: "▼", code: "DGRD" },

  // jobs
  active:            { text: "text-accent",             glyph: "▶", code: "ACT"  },
  suspended:         { text: "text-status-terminating", glyph: "‖", code: "SUSP" },

  // pv / pvc
  bound:             { text: "text-status-running",     glyph: "◉", code: "BND"  },
  available:         { text: "text-accent",             glyph: "○", code: "AVL"  },
  released:          { text: "text-status-terminating", glyph: "↑", code: "RLS"  },
  lost:              { text: "text-status-error",       glyph: "✖", code: "LOST" },

  // nodes
  ready:             { text: "text-status-running",     glyph: "◉", code: "RDY"  },
  notready:          { text: "text-status-crashloop",   glyph: "✖", code: "NRDY" },
  schedulingdisabled:{ text: "text-status-terminating", glyph: "⌧", code: "NSCH" },

  // helm
  deployed:          { text: "text-status-running",     glyph: "◉", code: "DEPL" },
  "pending-install": { text: "text-status-pending",     glyph: "◯", code: "INST" },
  "pending-upgrade": { text: "text-status-pending",     glyph: "◯", code: "UPGD" },
  superseded:        { text: "text-status-terminating", glyph: "↪", code: "SUPS" },
  uninstalled:       { text: "text-status-terminating", glyph: "⌧", code: "UNIN" },
  failed:            { text: "text-status-crashloop",   glyph: "✖", code: "FAIL" },

  // events
  normal:            { text: "text-text-muted",         glyph: "·", code: "NRM"  },
  warning:           { text: "text-status-backoff",     glyph: "⚠", code: "WARN" },
};

function styleFor(status: string): PillStyle {
  const direct = PALETTE[status];
  if (direct) return direct;
  const lower = status.toLowerCase();
  const lowerHit = PALETTE[lower];
  if (lowerHit) return lowerHit;
  // Heuristic fallback for unknown values.
  if (lower.includes("error") || lower.includes("fail") || lower.includes("crash")) {
    return { text: "text-status-error", glyph: "✖", code: status.slice(0, 4).toUpperCase() };
  }
  if (lower.includes("pending") || lower.includes("progress") || lower.includes("init")) {
    return { text: "text-status-pending", glyph: "◯", code: status.slice(0, 4).toUpperCase() };
  }
  if (lower.includes("ok") || lower.includes("ready") || lower.includes("run") || lower.includes("healthy")) {
    return { text: "text-status-running", glyph: "◉", code: status.slice(0, 4).toUpperCase() };
  }
  return { text: "text-text-muted", glyph: "·", code: status.slice(0, 4).toUpperCase() };
}

export default function StatusPill({ status, compact }: StatusPillProps) {
  const s = styleFor(status);
  if (compact) {
    return (
      <span className={`k9s-glyph ${s.text} font-semibold`} aria-label={status} title={status}>
        {s.glyph}
      </span>
    );
  }
  return (
    <span
      className={`inline-flex items-center gap-1.5 ${s.text} font-mono text-[11px] font-semibold tabular-nums leading-5`}
      title={status}
    >
      <span aria-hidden="true" className="k9s-glyph drop-shadow-[0_0_5px_currentColor]">{s.glyph}</span>
      <span className="tracking-wider">{s.code}</span>
    </span>
  );
}
