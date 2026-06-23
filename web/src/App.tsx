/**
 * App shell: TopBar / TabsBar above, Sidebar + MainView in a flex row.
 *
 * The home ("overview") tab renders the full-bleed HabitatDashboard, which
 * carries its own left rail — so the Freelens resource Sidebar is hidden there
 * to keep the habitat uncluttered. Every other tab shows the Sidebar.
 *
 * Floating layers (DetailDrawer, HotbarDock, TerminalDock, CommandPalette,
 * KeybindingsHelp) and the global KeyboardLayer are mounted once at the root.
 */

import TopBar from "./components/TopBar";
import TabsBar from "./components/TabsBar";
import Sidebar from "./components/Sidebar";
import MainView from "./components/MainView";
import DetailDrawer from "./components/DetailDrawer";
import HotbarDock from "./components/HotbarDock";
import TerminalDock from "./components/TerminalDock";
import CommandPalette from "./components/CommandPalette";
import KeybindingsHelp from "./components/KeybindingsHelp";
import KeyboardLayer from "./components/KeyboardLayer";
import Toasts from "./components/Toasts";
import { useActiveTab, useSidebarState, useTabs, workspaceActions } from "./store/workspace";

export default function App() {
  const sidebar = useSidebarState();
  const tabs = useTabs();
  const activeId = useActiveTab();
  const activeKind = tabs.find((t) => t.id === activeId)?.kind;
  const habitat = activeKind === "overview";

  return (
    <div className="flex flex-col h-screen overflow-hidden bg-bg-base text-text">
      {/* Yscale atmosphere — fixed, pointer-events-none, very low intensity.
          The glow tint tracks cluster mood (set on <body> by the habitat). */}
      <div className="atmo-glow" aria-hidden="true" />
      <div className="atmo-vignette" aria-hidden="true" />
      <div className="atmo-grain" aria-hidden="true" />

      <TopBar />
      <TabsBar />
      <div className="flex flex-1 min-h-0 relative">
        {!habitat && <Sidebar />}

        {/* Mobile scrim — only visible while the sidebar is open on small screens */}
        {!habitat && sidebar.open && (
          <button
            type="button"
            aria-label="Close navigation"
            onClick={() => workspaceActions.toggleSidebar()}
            className="md:hidden fixed inset-0 z-20 bg-black/40 backdrop-blur-[1px]"
          />
        )}

        <MainView />
      </div>

      {/* Floating / global layers. The hotbar (pinned pets) is hidden on the
          habitat home so the cluster view stays clean. */}
      <DetailDrawer />
      {!habitat && <HotbarDock />}
      <TerminalDock />
      <CommandPalette />
      <KeybindingsHelp />
      <Toasts />
      <KeyboardLayer />
    </div>
  );
}
