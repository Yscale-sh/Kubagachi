/**
 * SettingsPanel — plug in a kubeconfig without leaving the cockpit.
 *
 * A centered overlay (matching the command palette / keybindings help) with two
 * tabs:
 *   - raw:  paste kubeconfig YAML; the server loads it in-memory, no file needed
 *   - path: point at a kubeconfig file the server can read
 *
 * On connect the backend validates + switches the live cluster to the config's
 * current-context (leaving demo mode); a bad config leaves the running cockpit
 * untouched and the error is shown inline.
 */

import { useEffect, useRef, useState } from "react";
import { FileText, Loader2, ClipboardPaste } from "lucide-react";
import { useSettingsOpen, workspaceActions } from "../store/workspace";

type Tab = "raw" | "path";

export default function SettingsPanel() {
  const open = useSettingsOpen();
  const [tab, setTab] = useState<Tab>("raw");
  const [raw, setRaw] = useState("");
  const [path, setPath] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const firstFieldRef = useRef<HTMLTextAreaElement | HTMLInputElement | null>(null);

  const close = (): void => {
    if (busy) return;
    workspaceActions.setSettingsOpen(false);
  };

  // Local Escape handling: KeyboardLayer ignores keys while a field is focused,
  // so the overlay must close itself. Capture phase so it wins over the field.
  useEffect(() => {
    if (!open) return;
    const onKey = (e: KeyboardEvent): void => {
      if (e.key === "Escape") {
        e.stopPropagation();
        close();
      }
    };
    window.addEventListener("keydown", onKey, true);
    return () => window.removeEventListener("keydown", onKey, true);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, busy]);

  // Reset transient state each time the panel opens; focus the active field.
  useEffect(() => {
    if (!open) return;
    setError(null);
    setBusy(false);
    const id = window.setTimeout(() => firstFieldRef.current?.focus(), 0);
    return () => window.clearTimeout(id);
  }, [open, tab]);

  if (!open) return null;

  const value = tab === "raw" ? raw : path;
  const canSubmit = value.trim().length > 0 && !busy;

  const submit = async (): Promise<void> => {
    if (!canSubmit) return;
    setError(null);
    setBusy(true);
    try {
      await workspaceActions.applyKubeconfig(
        tab === "raw" ? { mode: "raw", raw } : { mode: "path", path: path.trim() },
      );
      workspaceActions.setSettingsOpen(false);
    } catch (e) {
      setError(e instanceof Error ? e.message : "failed to apply kubeconfig");
    } finally {
      setBusy(false);
    }
  };

  return (
    <>
      <div
        className="fixed inset-0 z-50 bg-black/50 backdrop-blur-[1px]"
        onClick={close}
        aria-hidden="true"
      />
      <div className="fixed inset-0 z-50 flex items-center justify-center px-4 pointer-events-none">
        <div
          role="dialog"
          aria-label="Settings"
          className="pointer-events-auto w-full max-w-[520px] bg-bg-panel border border-border-strong shadow-2xl shadow-black/60 font-mono yscale-overlay-in k9s-square"
        >
          <div className="flex items-center px-3 h-9 border-b border-border text-[12px]">
            <span className="k9s-bracket text-accent">
              <span className="text-text">kubeconfig</span>
            </span>
            <button
              type="button"
              onClick={close}
              className="ml-auto text-[10px] text-text-muted hover:text-text transition-colors"
            >
              esc to close
            </button>
          </div>

          {/* Tabs */}
          <div className="flex items-stretch border-b border-border text-[11px]">
            <TabButton active={tab === "raw"} onClick={() => setTab("raw")} icon={<ClipboardPaste size={13} />}>
              raw
            </TabButton>
            <TabButton active={tab === "path"} onClick={() => setTab("path")} icon={<FileText size={13} />}>
              file path
            </TabButton>
          </div>

          <div className="p-3 flex flex-col gap-3">
            <p className="text-[11px] leading-relaxed text-text-muted/80">
              {tab === "raw"
                ? "Paste kubeconfig YAML. It's loaded in memory and connects to its current-context — no file written to disk."
                : "Absolute path to a kubeconfig file the server can read (e.g. ~/.kube/config or /etc/rancher/k3s/k3s.yaml)."}
            </p>

            {tab === "raw" ? (
              <textarea
                ref={firstFieldRef as React.RefObject<HTMLTextAreaElement>}
                value={raw}
                onChange={(e) => setRaw(e.target.value)}
                spellCheck={false}
                placeholder={"apiVersion: v1\nkind: Config\nclusters:\n- ..."}
                className="w-full h-52 resize-none bg-bg-base border border-border focus:border-accent outline-none px-2.5 py-2 text-[12px] leading-relaxed text-text placeholder:text-text-muted/40 k9s-square"
              />
            ) : (
              <input
                ref={firstFieldRef as React.RefObject<HTMLInputElement>}
                type="text"
                value={path}
                onChange={(e) => setPath(e.target.value)}
                spellCheck={false}
                onKeyDown={(e) => {
                  if (e.key === "Enter") void submit();
                }}
                placeholder="/home/you/.kube/config"
                className="w-full bg-bg-base border border-border focus:border-accent outline-none px-2.5 h-9 text-[12px] text-text placeholder:text-text-muted/40 k9s-square"
              />
            )}

            {error && (
              <div className="text-[11px] leading-relaxed text-status-error border border-status-error/40 bg-status-error/10 px-2.5 py-1.5 k9s-square break-words">
                {error}
              </div>
            )}

            <div className="flex items-center gap-2">
              <span className="text-[10px] text-text-muted/60">
                connecting switches the live cluster
              </span>
              <button
                type="button"
                onClick={() => void submit()}
                disabled={!canSubmit}
                className="ml-auto inline-flex items-center gap-1.5 h-8 px-3 text-[11px] uppercase tracking-wide bg-accent/15 text-accent border border-accent/50 hover:bg-accent/25 disabled:opacity-40 disabled:cursor-not-allowed transition-colors k9s-square"
              >
                {busy && <Loader2 size={13} className="animate-spin" />}
                {busy ? "connecting…" : "connect"}
              </button>
            </div>
          </div>
        </div>
      </div>
    </>
  );
}

function TabButton({
  active,
  onClick,
  icon,
  children,
}: {
  active: boolean;
  onClick: () => void;
  icon: React.ReactNode;
  children: React.ReactNode;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={
        "inline-flex items-center gap-1.5 px-3 h-8 uppercase tracking-wide transition-colors border-b-2 " +
        (active
          ? "border-accent text-accent bg-accent/5"
          : "border-transparent text-text-muted hover:text-text")
      }
    >
      {icon}
      {children}
    </button>
  );
}
