/**
 * Toasts — app-wide action-feedback notifications.
 *
 * Reads the transient toast queue from the workspace store (workspaceActions
 * .toast(msg, kind) pushes; entries auto-dismiss after ~3.8s). Anchored above
 * the hotbar / terminal dock so it never hides behind them, and announced via
 * aria-live. Click a toast to dismiss it early.
 */

import { useToasts, workspaceActions } from "../store/workspace";

const KIND: Record<string, { color: string; glyph: string }> = {
  info: { color: "text-tui-cyan", glyph: "ℹ" },
  success: { color: "text-status-running", glyph: "✓" },
  error: { color: "text-status-error", glyph: "✖" },
};

export default function Toasts() {
  const toasts = useToasts();
  if (toasts.length === 0) return null;
  return (
    <div
      className="fixed left-1/2 -translate-x-1/2 bottom-24 z-[80] flex flex-col items-center gap-2 pointer-events-none"
      aria-live="polite"
    >
      {toasts.map((t) => {
        const k = KIND[t.kind] ?? KIND.info;
        return (
          <button
            key={t.id}
            type="button"
            onClick={() => workspaceActions.dismissToast(t.id)}
            className="pointer-events-auto yscale-overlay-in flex items-center gap-2 max-w-[90vw] border border-border-strong bg-bg-panel/95 backdrop-blur px-3 py-2 text-[12px] text-text shadow-lg shadow-black/50 k9s-square font-mono"
          >
            <span className={`k9s-glyph ${k.color}`} aria-hidden="true">{k.glyph}</span>
            <span className="truncate">{t.message}</span>
          </button>
        );
      })}
    </div>
  );
}
