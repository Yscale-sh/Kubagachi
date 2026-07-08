/**
 * YscaleTab — live burst-fleet dashboard for the yscale hyperscaler.
 *
 * Polls /api/yscale every 5 s. Between polls, accrued costs tick upward in
 * real-time via requestAnimationFrame, interpolated from each burst's
 * hourly_usd rate. This makes the "billed by the second" nature viscerally
 * felt — every second the numbers visibly climb.
 *
 * States: loading / not-configured / error / empty-fleet / populated.
 */

import { useEffect, useRef, useState } from "react";
import { AlertTriangle, ServerCrash, Zap } from "lucide-react";
import {
  fetchYscale,
  type YscaleBurst,
  type YscaleResponse,
  type YscaleSpend,
} from "../lib/cluster-api";

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function fmtUsd(v: number): string {
  return v < 0.01 && v > 0
    ? `$${v.toFixed(4)}`
    : `$${v.toFixed(2)}`;
}

function fmtAge(seconds: number): string {
  if (seconds < 60) return `${Math.floor(seconds)}s`;
  const m = Math.floor(seconds / 60);
  const h = Math.floor(m / 60);
  if (h === 0) return `${m}m`;
  const rm = m - h * 60;
  return rm === 0 ? `${h}h` : `${h}h${rm}m`;
}

function shortId(id: string): string {
  return id.length > 12 ? id.slice(0, 12) : id;
}

// ---------------------------------------------------------------------------
// Live cost ticker hook
//
// Takes the latest burst list from the API poll and returns a map of
// burst-id → accrued_usd that ticks up every animation frame.
// Accuracy: interpolates accrued_usd + (elapsed_ms / 3_600_000) * hourly_usd.
// ---------------------------------------------------------------------------

function useLiveCosts(bursts: YscaleBurst[]): Map<string, number> {
  const [costs, setCosts] = useState<Map<string, number>>(new Map());
  // Snapshot: { id -> { baseAccrued, hourlyUsd, snapshotTime } }
  const snapRef = useRef<
    Map<string, { baseAccrued: number; hourlyUsd: number; snapshotTime: number }>
  >(new Map());
  const rafRef = useRef<number>(0);

  // Refresh snapshot whenever bursts update from the poll.
  useEffect(() => {
    const now = performance.now();
    const nextSnap = new Map<
      string,
      { baseAccrued: number; hourlyUsd: number; snapshotTime: number }
    >();
    for (const b of bursts) {
      nextSnap.set(b.id, {
        baseAccrued: b.accrued_usd,
        hourlyUsd: b.hourly_usd,
        snapshotTime: now,
      });
    }
    snapRef.current = nextSnap;
  }, [bursts]);

  // rAF loop — runs while component is mounted.
  useEffect(() => {
    let running = true;

    const tick = (): void => {
      if (!running) return;
      const now = performance.now();
      const next = new Map<string, number>();
      for (const [id, snap] of snapRef.current) {
        const elapsedHrs = (now - snap.snapshotTime) / 3_600_000;
        next.set(id, snap.baseAccrued + elapsedHrs * snap.hourlyUsd);
      }
      setCosts(next);
      rafRef.current = requestAnimationFrame(tick);
    };

    rafRef.current = requestAnimationFrame(tick);
    return () => {
      running = false;
      cancelAnimationFrame(rafRef.current);
    };
  }, []);

  return costs;
}

// ---------------------------------------------------------------------------
// Main component
// ---------------------------------------------------------------------------

export default function YscaleTab() {
  const [data, setData] = useState<YscaleResponse | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;

    const load = (): void => {
      fetchYscale()
        .then((res) => {
          if (!cancelled) {
            setData(res);
            setError(null);
          }
        })
        .catch((err: unknown) => {
          if (!cancelled) {
            setError(err instanceof Error ? err.message : String(err));
          }
        });
    };

    load();
    const id = setInterval(load, 5_000);
    return () => {
      cancelled = true;
      clearInterval(id);
    };
  }, []);

  // --- loading ---
  if (data === null && error === null) {
    return (
      <div className="flex flex-col items-center justify-center py-20 text-text-muted gap-3 font-mono">
        <Zap size={20} className="opacity-40" />
        <span className="text-[12px]">connecting to yscale…</span>
      </div>
    );
  }

  // --- fetch-level error ---
  if (error !== null) {
    return (
      <div className="flex flex-col items-center justify-center py-20 gap-3 font-mono">
        <AlertTriangle size={20} className="text-status-error opacity-70" />
        <span className="text-[12px] text-status-error">{error}</span>
      </div>
    );
  }

  if (data === null) return null;

  // --- not configured ---
  if (!data.configured) {
    return <NotConfiguredState />;
  }

  // --- configured but central unreachable ---
  if ("error" in data) {
    return <ErrorState url={data.url} message={data.error} />;
  }

  // --- happy path ---
  return <FleetView spend={data.spend} bursts={data.bursts} />;
}

// ---------------------------------------------------------------------------
// Not-configured empty state
// ---------------------------------------------------------------------------

function NotConfiguredState() {
  return (
    <div className="flex flex-col items-center justify-center py-20 gap-4 font-mono px-6 text-center">
      <Zap size={24} className="text-accent opacity-50" />
      <div className="text-[13px] text-text-muted max-w-[440px]">
        <span className="text-text font-semibold">yscale not connected.</span>
        <br />
        Run kubagachi with{" "}
        <code className="text-accent bg-bg-panel2 border border-border px-1 py-0.5 k9s-square text-[11px]">
          --yscale-url &lt;central&gt;
        </code>{" "}
        and set{" "}
        <code className="text-accent bg-bg-panel2 border border-border px-1 py-0.5 k9s-square text-[11px]">
          YSCALE_TOKEN=&lt;token&gt;
        </code>{" "}
        to enable burst-fleet visibility.
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Error state (configured but central unreachable)
// ---------------------------------------------------------------------------

function ErrorState({ url, message }: { url: string; message: string }) {
  return (
    <div className="flex flex-col items-start gap-3 px-4 py-6 font-mono">
      <div className="flex items-center gap-2">
        <ServerCrash size={16} className="text-status-error shrink-0" />
        <span className="text-[12px] text-status-error font-semibold">central unreachable</span>
      </div>
      <div className="text-[11px] text-text-muted">
        <span className="uppercase tracking-wider opacity-70">url </span>
        <span className="text-text">{url}</span>
      </div>
      <div className="text-[12px] text-text bg-bg-panel2 border border-border p-2 k9s-square max-w-[600px] break-words">
        {message}
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Fleet view — spend header + burst table
// ---------------------------------------------------------------------------

function FleetView({ spend, bursts }: { spend: YscaleSpend; bursts: YscaleBurst[] }) {
  const liveCosts = useLiveCosts(bursts);

  // Sum live accrued across all bursts for the total ticker.
  const totalAccrued = bursts.reduce((sum, b) => {
    return sum + (liveCosts.get(b.id) ?? b.accrued_usd);
  }, 0);

  return (
    <div className="font-mono flex flex-col">
      {/* Spend summary header */}
      <SpendHeader spend={spend} totalAccrued={totalAccrued} />

      {/* Burst table */}
      {bursts.length === 0 ? (
        <EmptyFleet />
      ) : (
        <div className="overflow-x-auto scrollbar-thin">
          <table className="w-full text-[12px] border-separate border-spacing-0">
            <thead className="sticky top-0 z-10 bg-bg-panel">
              <tr>
                <Th className="w-32">id</Th>
                <Th className="w-20">backend</Th>
                <Th className="w-24">status</Th>
                <Th className="min-w-[120px]">node</Th>
                <Th className="w-28">sku</Th>
                <Th className="w-20 text-right">$/hr</Th>
                <Th className="w-24 text-right">accrued</Th>
                <Th className="w-16 text-right">age</Th>
                <Th className="w-24">mesh</Th>
                <Th className="min-w-[120px]">pod cidr</Th>
              </tr>
            </thead>
            <tbody>
              {bursts.map((b) => (
                <BurstRow key={b.id} burst={b} liveCost={liveCosts.get(b.id) ?? b.accrued_usd} />
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Spend summary header
// ---------------------------------------------------------------------------

function SpendHeader({ spend, totalAccrued }: { spend: YscaleSpend; totalAccrued: number }) {
  const maxBursts = spend.limits.max_concurrent_bursts;
  const maxHourly = spend.limits.max_hourly_usd;

  const burstPct =
    maxBursts > 0 ? Math.min(1, spend.running_bursts / maxBursts) : 0;
  const hourlyPct =
    maxHourly > 0 ? Math.min(1, spend.hourly_usd / maxHourly) : 0;

  return (
    <div className="flex items-start gap-0 flex-wrap border-b border-border bg-bg-panel">
      <SpendTile
        label="running bursts"
        value={
          maxBursts > 0
            ? `${spend.running_bursts} / ${maxBursts}`
            : `${spend.running_bursts}`
        }
        pct={burstPct}
        accent={burstPct > 0.85}
      />
      <SpendTile
        label="hourly rate"
        value={
          maxHourly > 0
            ? `${fmtUsd(spend.hourly_usd)} / ${fmtUsd(maxHourly)}`
            : fmtUsd(spend.hourly_usd)
        }
        pct={hourlyPct}
        accent={hourlyPct > 0.85}
      />
      <SpendTile
        label="projected daily"
        value={fmtUsd(spend.projected_daily_usd)}
        pct={null}
        accent={false}
      />
      {/* The signature moment: live total accrued ticking up in real-time */}
      <LiveTotalTile totalAccrued={totalAccrued} />
    </div>
  );
}

function SpendTile({
  label,
  value,
  pct,
  accent,
}: {
  label: string;
  value: string;
  pct: number | null;
  accent: boolean;
}) {
  return (
    <div className="flex flex-col gap-1 px-4 py-3 border-r border-border min-w-[150px]">
      <div className="text-[10px] uppercase tracking-wider text-text-muted">{label}</div>
      <div
        className={
          "text-[16px] tabular-nums font-semibold " +
          (accent ? "text-status-error" : "text-text")
        }
      >
        {value}
      </div>
      {pct !== null && (
        <div className="h-[2px] bg-border w-full k9s-square overflow-hidden mt-0.5">
          <div
            className={
              "h-full transition-all duration-500 " +
              (accent ? "bg-status-error" : "bg-accent")
            }
            style={{ width: `${(pct * 100).toFixed(1)}%` }}
          />
        </div>
      )}
    </div>
  );
}

/**
 * Signature moment: the total accrued cost tile ticks up live via rAF.
 * The number is always moving — every second costs money.
 */
function LiveTotalTile({ totalAccrued }: { totalAccrued: number }) {
  return (
    <div className="flex flex-col gap-1 px-4 py-3 min-w-[170px] relative">
      {/* Subtle gold pulse behind the live number */}
      <div
        className="absolute inset-0 pointer-events-none"
        aria-hidden="true"
        style={{
          background:
            "radial-gradient(ellipse 80% 60% at 30% 50%, rgba(201,184,138,0.06), transparent 70%)",
        }}
      />
      <div className="text-[10px] uppercase tracking-wider text-accent/70 relative z-10">
        total accrued
        <span
          className="yscale-live-blink ml-1.5 inline-block w-1.5 h-1.5 rounded-full bg-accent align-middle"
          aria-label="live"
        />
      </div>
      <div className="text-[18px] tabular-nums font-semibold text-accent relative z-10 tracking-tight">
        {fmtUsd(totalAccrued)}
      </div>
      <style>{`
        .yscale-live-blink {
          animation: yscale-blink 1.4s ease-in-out infinite;
        }
        @keyframes yscale-blink {
          0%, 100% { opacity: 1; }
          50% { opacity: 0.2; }
        }
        @media (prefers-reduced-motion: reduce) {
          .yscale-live-blink { animation: none !important; }
        }
      `}</style>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Empty fleet
// ---------------------------------------------------------------------------

function EmptyFleet() {
  return (
    <div className="flex flex-col items-center justify-center py-16 text-text-muted gap-2">
      <Zap size={20} className="opacity-30" />
      <div className="text-[12px]">no active bursts.</div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Burst row
// ---------------------------------------------------------------------------

function BurstRow({ burst: b, liveCost }: { burst: YscaleBurst; liveCost: number }) {
  return (
    <tr
      className="hover:bg-bg-panel2 transition-colors duration-100"
      style={{ height: 32 }}
    >
      <td className="pl-2 pr-3 py-1 border-b border-border/70 align-middle">
        <span className="text-text-muted/60 mr-1.5" aria-hidden="true">▍</span>
        <code className="text-text text-[11px]">{shortId(b.id)}</code>
      </td>
      <td className="px-3 py-1 border-b border-border/70 align-middle">
        <BackendBadge backend={b.backend} />
      </td>
      <td className="px-3 py-1 border-b border-border/70 align-middle">
        <StatusPill status={b.status} />
      </td>
      <td className="px-3 py-1 border-b border-border/70 align-middle text-text-muted truncate max-w-[180px]">
        {b.node_name}
      </td>
      <td className="px-3 py-1 border-b border-border/70 align-middle text-text-muted">
        {b.sku}
      </td>
      <td className="px-3 py-1 border-b border-border/70 align-middle text-right tabular-nums text-text-muted">
        {fmtUsd(b.hourly_usd)}
      </td>
      <td className="px-3 py-1 border-b border-border/70 align-middle text-right tabular-nums text-accent font-semibold">
        {fmtUsd(liveCost)}
      </td>
      <td className="px-3 py-1 border-b border-border/70 align-middle text-right tabular-nums text-text-muted">
        {fmtAge(b.age_seconds)}
      </td>
      <td className="px-3 py-1 border-b border-border/70 align-middle text-text-muted text-[11px]">
        {b.mesh_provider || "—"}
      </td>
      <td className="px-3 py-1 border-b border-border/70 align-middle text-text-muted text-[11px] truncate max-w-[140px]">
        <code>{b.pod_cidr}</code>
      </td>
    </tr>
  );
}

// ---------------------------------------------------------------------------
// Pieces
// ---------------------------------------------------------------------------

function Th({ children, className }: { children: React.ReactNode; className?: string }) {
  return (
    <th
      className={`text-left font-normal text-text-muted/80 uppercase tracking-wider text-[10px] px-3 py-1.5 border-b border-border ${className ?? ""}`}
    >
      {children}
    </th>
  );
}

function BackendBadge({ backend }: { backend: "flyio" | "linode" | "aws" }) {
  const map: Record<"flyio" | "linode" | "aws", { label: string; cls: string }> = {
    flyio: { label: "fly.io", cls: "text-tui-pink border-tui-pink/40" },
    linode: { label: "linode", cls: "text-tui-cyan border-tui-cyan/40" },
    aws: { label: "aws", cls: "text-status-pending border-status-pending/40" },
  };
  const { label, cls } = map[backend] ?? { label: backend, cls: "text-text-muted border-border" };
  return (
    <span
      className={`inline-flex items-center px-1.5 py-0.5 text-[10px] border k9s-square uppercase tracking-wider ${cls}`}
    >
      {label}
    </span>
  );
}

function StatusPill({ status }: { status: string }) {
  const s = status.toLowerCase();
  let cls = "text-text-muted border-border/60";
  let glyph = "·";

  if (s === "running") {
    cls = "text-status-running border-status-running/40";
    glyph = "◉";
  } else if (s === "provisioning") {
    cls = "text-status-pending border-status-pending/40";
    glyph = "◌";
  } else if (s === "error" || s === "failed") {
    cls = "text-status-error border-status-error/40";
    glyph = "✖";
  } else if (s === "terminating") {
    cls = "text-status-terminating border-border/60";
    glyph = "⊘";
  }

  return (
    <span
      className={`inline-flex items-center gap-1 px-1.5 py-0.5 text-[10px] border k9s-square uppercase tracking-wider ${cls}`}
    >
      <span className="k9s-glyph">{glyph}</span>
      {status}
    </span>
  );
}
