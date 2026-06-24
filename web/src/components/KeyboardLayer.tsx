/**
 * KeyboardLayer — global k9s-style keybindings. Renders nothing.
 *
 *   /        focus the global search
 *   :        open the command palette
 *   j / k    move row selection in the active table view
 *   enter    open detail for the selected row
 *   g / G    jump to top / bottom
 *   esc      close drawer / palette / help
 *   ?        keybindings help overlay
 *   [ / ]    switch to previous / next tab
 *
 * Keys are ignored while typing in inputs/selects/textareas (including the
 * xterm terminal, which uses a hidden textarea).
 */

import { useEffect } from "react";
import {
  useActiveTab,
  useHelpOpen,
  usePaletteOpen,
  useSelectedRow,
  useSelection,
  useTabs,
  workspaceActions,
} from "../store/workspace";
import { getRowNav, rowNavCount, scrollSelectedRowIntoView } from "../lib/row-nav";

function isTypingTarget(target: EventTarget | null): boolean {
  const el = target as HTMLElement | null;
  if (!el || !el.tagName) return false;
  const tag = el.tagName;
  if (tag === "INPUT" || tag === "TEXTAREA" || tag === "SELECT") return true;
  if (el.isContentEditable) return true;
  // xterm captures keys through a hidden textarea, but belt-and-braces:
  if (typeof el.closest === "function" && el.closest(".xterm")) return true;
  return false;
}

export default function KeyboardLayer() {
  const paletteOpen = usePaletteOpen();
  const helpOpen = useHelpOpen();
  const selection = useSelection();
  const selectedRow = useSelectedRow();
  const tabs = useTabs();
  const activeTabId = useActiveTab();

  useEffect(() => {
    const onKey = (e: KeyboardEvent): void => {
      if (e.defaultPrevented) return;
      if (e.metaKey || e.ctrlKey || e.altKey) return;
      if (isTypingTarget(e.target)) return;

      switch (e.key) {
        case "/": {
          e.preventDefault();
          const input = document.querySelector<HTMLInputElement>("[data-global-search]");
          input?.focus();
          input?.select();
          return;
        }
        case ":": {
          e.preventDefault();
          workspaceActions.setPaletteOpen(true);
          return;
        }
        case "?": {
          e.preventDefault();
          workspaceActions.setHelpOpen(!helpOpen);
          return;
        }
        case "[":
        case "]": {
          if (tabs.length <= 1) return;
          e.preventDefault();
          const activeIndex = tabs.findIndex((tab) => tab.id === activeTabId);
          const currentIndex = activeIndex >= 0 ? activeIndex : 0;
          const direction = e.key === "]" ? 1 : -1;
          const nextIndex = (currentIndex + direction + tabs.length) % tabs.length;
          workspaceActions.setActiveTab(tabs[nextIndex].id);
          return;
        }
        case "Escape": {
          if (helpOpen) {
            workspaceActions.setHelpOpen(false);
            return;
          }
          if (paletteOpen) {
            workspaceActions.setPaletteOpen(false);
            return;
          }
          if (selection !== null) {
            workspaceActions.selectResource(null);
          }
          return;
        }
        default:
          break;
      }

      // Row navigation only applies when no modal layer is up.
      if (paletteOpen || helpOpen) return;

      switch (e.key) {
        case "j": {
          e.preventDefault();
          workspaceActions.moveSelectedRow(1, Math.max(0, rowNavCount() - 1));
          scrollSelectedRowIntoView();
          return;
        }
        case "k": {
          e.preventDefault();
          workspaceActions.moveSelectedRow(-1, Math.max(0, rowNavCount() - 1));
          scrollSelectedRowIntoView();
          return;
        }
        case "g": {
          e.preventDefault();
          workspaceActions.setSelectedRow(0);
          scrollSelectedRowIntoView();
          return;
        }
        case "G": {
          e.preventDefault();
          workspaceActions.setSelectedRow(Math.max(0, rowNavCount() - 1));
          scrollSelectedRowIntoView();
          return;
        }
        case "Enter": {
          if (selection !== null) return; // drawer already open
          const nav = getRowNav();
          if (!nav || nav.ids.length === 0) return;
          e.preventDefault();
          const idx = Math.min(selectedRow, nav.ids.length - 1);
          nav.onEnter(nav.ids[idx], idx);
          return;
        }
        default:
          return;
      }
    };

    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [paletteOpen, helpOpen, selection, selectedRow, tabs, activeTabId]);

  return null;
}
