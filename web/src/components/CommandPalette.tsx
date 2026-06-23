/**
 * CommandPalette — k9s-style `:` command bar.
 *
 * Centered overlay, JetBrains Mono, gold caret. Fuzzy-matches a fixed command
 * list (pods, deployments, services, nodes, flux, events, overview,
 * kubagachi, quit) plus dynamic `ns <name>` namespace commands. Enter
 * executes; Esc closes; arrows (or Tab) move the cursor.
 */

import { useEffect, useMemo, useRef, useState } from "react";
import { useCluster, usePaletteOpen, workspaceActions, type TabKind } from "../store/workspace";

interface Command {
  name: string;
  hint: string;
  run: () => void;
}

function tabCommand(name: string, hint: string, kind: TabKind): Command {
  return {
    name,
    hint,
    run: () => workspaceActions.openTab(kind),
  };
}

const STATIC_COMMANDS: Command[] = [
  tabCommand("pods", "open the pod table", "Pod"),
  tabCommand("deployments", "open the deployment table", "Deployment"),
  tabCommand("services", "open the service table", "Service"),
  tabCommand("nodes", "open the node table", "Node"),
  tabCommand("flux", "open the flux (gitops) table", "flux"),
  tabCommand("events", "open the event stream", "events"),
  tabCommand("overview", "open the cluster overview", "overview"),
  tabCommand("kubagachi", "open your pinned pets", "kubagachi"),
  {
    name: "quit",
    hint: "close this palette",
    run: () => {
      /* closing is handled by the caller */
    },
  },
];

/** Simple subsequence fuzzy match; returns a score (lower is better) or null. */
function fuzzyScore(query: string, target: string): number | null {
  if (!query) return 0;
  let qi = 0;
  let score = 0;
  let lastHit = -1;
  for (let ti = 0; ti < target.length && qi < query.length; ti++) {
    if (target[ti] === query[qi]) {
      score += lastHit === -1 ? ti : ti - lastHit - 1;
      lastHit = ti;
      qi++;
    }
  }
  return qi === query.length ? score : null;
}

export default function CommandPalette() {
  const open = usePaletteOpen();
  if (!open) return null;
  return <PaletteBody />;
}

function PaletteBody() {
  const cluster = useCluster();
  const [query, setQuery] = useState("");
  const [cursor, setCursor] = useState(0);
  const inputRef = useRef<HTMLInputElement | null>(null);

  useEffect(() => {
    inputRef.current?.focus();
  }, []);

  const close = (): void => workspaceActions.setPaletteOpen(false);

  const commands = useMemo<Command[]>(() => {
    const q = query.trim().toLowerCase();

    // `ns <name>` namespace commands (dynamic).
    const nsCommands: Command[] = (cluster?.namespaces ?? []).map((n) => ({
      name: `ns ${n.name}`,
      hint: "switch namespace",
      run: () => workspaceActions.setNamespace(n.name),
    }));
    nsCommands.unshift({
      name: "ns all",
      hint: "show all namespaces",
      run: () => workspaceActions.setNamespace("all"),
    });

    const all = [...STATIC_COMMANDS, ...nsCommands];
    if (!q) return all.slice(0, 12);

    const scored = all
      .map((c) => ({ c, s: fuzzyScore(q, c.name.toLowerCase()) }))
      .filter((x): x is { c: Command; s: number } => x.s !== null)
      .sort((a, b) => a.s - b.s || a.c.name.localeCompare(b.c.name));
    return scored.map((x) => x.c).slice(0, 12);
  }, [query, cluster]);

  const clamped = Math.min(cursor, Math.max(0, commands.length - 1));

  /** Group label for a command — drives the section headers in the list. */
  const groupOf = (cmd: Command): string =>
    cmd.name === "ns all" || cmd.name.startsWith("ns ") ? "NAMESPACE" : "NAVIGATE";

  const execute = (cmd: Command | undefined): void => {
    close();
    cmd?.run();
  };

  const onKeyDown = (e: React.KeyboardEvent<HTMLInputElement>): void => {
    if (e.key === "Escape") {
      e.preventDefault();
      close();
    } else if (e.key === "ArrowDown" || (e.key === "Tab" && !e.shiftKey)) {
      e.preventDefault();
      setCursor((c) => Math.min(commands.length - 1, c + 1));
    } else if (e.key === "ArrowUp" || (e.key === "Tab" && e.shiftKey)) {
      e.preventDefault();
      setCursor((c) => Math.max(0, c - 1));
    } else if (e.key === "Enter") {
      e.preventDefault();
      execute(commands[clamped]);
    }
  };

  return (
    <>
      <div
        className="fixed inset-0 z-50 bg-black/50 backdrop-blur-[1px]"
        onClick={close}
        aria-hidden="true"
      />
      <div className="fixed inset-x-0 top-[18vh] z-50 flex justify-center px-4 pointer-events-none">
        <div
          role="dialog"
          aria-label="Command palette"
          className="pointer-events-auto w-full max-w-[520px] bg-bg-panel border border-border-strong shadow-2xl shadow-black/60 font-mono yscale-overlay-in k9s-square"
        >
          {/* Input row */}
          <div className="flex items-center gap-2.5 px-3 h-12 border-b border-border-strong">
            <span
              className="text-accent text-[22px] leading-none font-bold select-none -mt-0.5"
              aria-hidden="true"
            >
              :
            </span>
            <input
              ref={inputRef}
              type="text"
              value={query}
              onChange={(e) => {
                setQuery(e.target.value);
                setCursor(0);
              }}
              onKeyDown={onKeyDown}
              placeholder="command… (pods, flux, ns <name>)"
              spellCheck={false}
              autoCapitalize="off"
              autoComplete="off"
              className="flex-1 bg-transparent text-[14px] text-text placeholder:text-text-muted/60 outline-none caret-accent"
            />
            <kbd className="text-[10px] text-text-muted/70 border border-border px-1 py-px k9s-square">
              esc
            </kbd>
          </div>

          {/* Results */}
          <div className="max-h-[40vh] overflow-y-auto scrollbar-thin py-1">
            {commands.length === 0 && (
              <div className="px-3 py-2 text-[12px] text-text-muted">no matching command.</div>
            )}
            {commands.map((c, i) => {
              const active = i === clamped;
              const group = groupOf(c);
              const showHeader = i === 0 || groupOf(commands[i - 1]) !== group;
              return (
                <div key={c.name}>
                  {showHeader && (
                    <div className="px-3 pt-2 pb-1 text-[10px] font-semibold tracking-[0.18em] text-text-muted/70 select-none">
                      {group}
                    </div>
                  )}
                  <button
                    type="button"
                    onMouseEnter={() => setCursor(i)}
                    onClick={() => execute(c)}
                    className={
                      "w-full flex items-center gap-2 px-3 py-1.5 text-left text-[12px] border-l-2 transition-colors duration-100 " +
                      (active
                        ? "bg-accent-dim border-accent text-text"
                        : "border-transparent text-text-muted hover:text-text")
                    }
                  >
                    <span
                      aria-hidden="true"
                      className={active ? "text-accent" : "text-transparent"}
                    >
                      ▍
                    </span>
                    <span className={"flex-1" + (active ? " text-accent-bright" : "")}>
                      {c.name}
                    </span>
                    <span className="text-[10px] text-text-muted/70">{c.hint}</span>
                  </button>
                </div>
              );
            })}
          </div>

          {/* Footer strip */}
          <div className="flex items-center justify-between gap-2 px-3 h-7 border-t border-border-strong text-[10px] text-text-muted/70 select-none">
            <span>
              <span className="text-accent">{commands.length}</span>{" "}
              {commands.length === 1 ? "result" : "results"}
            </span>
            <span className="flex items-center gap-1.5" aria-hidden="true">
              <span>↑↓ move</span>
              <span className="text-border-strong">·</span>
              <span>↵ run</span>
              <span className="text-border-strong">·</span>
              <span>esc</span>
            </span>
          </div>
        </div>
      </div>
    </>
  );
}
