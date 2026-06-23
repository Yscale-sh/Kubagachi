import type { PodStatus } from "./types";

/**
 * Format a duration in seconds as a compact "k8s style" age string.
 * Examples: 5s, 42s, 7m, 3h, 5d, 12d3h.
 */
export function formatAge(sec: number): string {
  if (!Number.isFinite(sec) || sec < 0) return "0s";
  const s = Math.floor(sec);
  if (s < 60) return `${s}s`;
  const m = Math.floor(s / 60);
  if (m < 60) return `${m}m`;
  const h = Math.floor(m / 60);
  if (h < 24) {
    const remM = m % 60;
    return remM > 0 ? `${h}h${remM}m` : `${h}h`;
  }
  const d = Math.floor(h / 24);
  const remH = h % 24;
  return remH > 0 ? `${d}d${remH}h` : `${d}d`;
}

/**
 * Format an absolute timestamp as ISO-ish YYYY-MM-DD HH:MM:SS (UTC).
 * Accepts Date, number (ms epoch) or ISO string.
 */
export function formatTimestamp(date: Date | number | string): string {
  const d = typeof date === "object" ? date : new Date(date);
  if (Number.isNaN(d.getTime())) return "—";
  const pad = (n: number) => n.toString().padStart(2, "0");
  return (
    `${d.getUTCFullYear()}-${pad(d.getUTCMonth() + 1)}-${pad(d.getUTCDate())} ` +
    `${pad(d.getUTCHours())}:${pad(d.getUTCMinutes())}:${pad(d.getUTCSeconds())}`
  );
}

const BYTE_UNITS = ["B", "KiB", "MiB", "GiB", "TiB", "PiB"] as const;

/** Format raw byte count to the largest binary unit that keeps n < 1024. */
export function formatBytes(n: number): string {
  if (!Number.isFinite(n)) return "—";
  const sign = n < 0 ? "-" : "";
  let v = Math.abs(n);
  let u = 0;
  while (v >= 1024 && u < BYTE_UNITS.length - 1) {
    v /= 1024;
    u += 1;
  }
  const precision = v >= 100 || u === 0 ? 0 : v >= 10 ? 1 : 2;
  return `${sign}${v.toFixed(precision)} ${BYTE_UNITS[u]}`;
}

/**
 * Format a CPU value where the input is "cores" (e.g. 0.25, 1, 2.5).
 * Sub-cores are rendered in millicores ("250m").
 */
export function formatCPU(n: number): string {
  if (!Number.isFinite(n)) return "—";
  if (n === 0) return "0";
  if (Math.abs(n) < 1) {
    return `${Math.round(n * 1000)}m`;
  }
  return Number.isInteger(n) ? `${n}` : n.toFixed(2);
}

/**
 * Render a label map as a stable, comma-joined string ("k=v,k=v").
 * Keys are sorted to make the output deterministic.
 */
export function parseLabels(labels: Record<string, string> | undefined): string {
  if (!labels) return "";
  const keys = Object.keys(labels).sort();
  return keys.map((k) => `${k}=${labels[k]}`).join(",");
}

/** Human-friendly text for a pod status (also used by badges/tooltips). */
export function humanStatus(status: PodStatus): string {
  switch (status) {
    case "running":
      return "Running";
    case "pending":
      return "Pending";
    case "completed":
      return "Completed";
    case "error":
      return "Error";
    case "unknown":
      return "Unknown";
    case "crashloop":
      return "Crash loop";
    case "backoff":
      return "Backoff";
    case "terminating":
      return "Terminating";
  }
}
