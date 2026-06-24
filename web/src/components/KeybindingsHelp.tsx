/**
 * KeybindingsHelp — `?` overlay styled like a TUI help box.
 */

import { Fragment } from "react";
import { useHelpOpen, workspaceActions } from "../store/workspace";

const GROUPS: ReadonlyArray<{
  title: string;
  bindings: ReadonlyArray<readonly [string, string]>;
}> = [
  {
    title: "navigation",
    bindings: [
      ["j / k", "move row selection"],
      ["g / G", "jump to top / bottom"],
      ["enter", "open detail for selection"],
      ["esc", "back / close layer"],
    ],
  },
  {
    title: "workspace",
    bindings: [
      ["/", "focus search"],
      ["⌘K / Ctrl-K", "focus search"],
      [":", "command palette"],
      ["[ / ]", "previous / next tab"],
      ["v", "toggle grid / ranch habitat view"],
    ],
  },
  {
    title: "help",
    bindings: [["?", "this help"]],
  },
];

export default function KeybindingsHelp() {
  const open = useHelpOpen();
  if (!open) return null;

  const close = (): void => workspaceActions.setHelpOpen(false);

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
          aria-label="Keybindings"
          className="pointer-events-auto w-full max-w-[380px] bg-bg-panel border border-border-strong shadow-2xl shadow-black/60 font-mono yscale-overlay-in k9s-square"
        >
          <div className="flex items-center px-3 h-9 border-b border-border text-[12px]">
            <span className="k9s-bracket text-accent">
              <span className="text-text">keys</span>
            </span>
            <button
              type="button"
              onClick={close}
              className="ml-auto text-[10px] text-text-muted hover:text-text transition-colors"
            >
              esc to close
            </button>
          </div>
          <div className="p-3">
            <table className="w-full text-[12px]">
              <tbody>
                {GROUPS.map((group) => (
                  <Fragment key={group.title}>
                    <tr style={{ height: 22 }}>
                      <td
                        colSpan={2}
                        className="pt-1 text-[10px] uppercase tracking-wider text-text-muted/70"
                      >
                        {group.title}
                      </td>
                    </tr>
                    {group.bindings.map(([key, desc]) => (
                      <tr key={key} style={{ height: 26 }}>
                        <td className="w-28 align-middle">
                          <kbd className="px-1.5 py-px border border-border bg-bg-base text-accent text-[11px] k9s-square">
                            {key}
                          </kbd>
                        </td>
                        <td className="align-middle text-text-muted">{desc}</td>
                      </tr>
                    ))}
                  </Fragment>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      </div>
    </>
  );
}
