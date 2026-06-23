/**
 * TerminalDock — bottom dock hosting a real xterm.js terminal connected to
 * the Go server's pod-exec WebSocket.
 *
 * Protocol (JSON text frames):
 *   send    {type:"stdin",data}  {type:"resize",cols,rows}  {type:"ping"}
 *   receive {type:"stdout",data} {type:"connected"} {type:"exit"} {type:"error",...}
 *
 * The dock takes layout space (~40vh) below the main view; it unmounts (and
 * tears the socket down) when the store's terminalSession is cleared. In demo
 * mode the server answers with an error frame, which we print gracefully.
 */

import { useEffect, useRef } from "react";
import { Terminal as TerminalIcon, X } from "lucide-react";
import { Terminal } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import "@xterm/xterm/css/xterm.css";
import {
  useTerminalSession,
  workspaceActions,
  type TerminalSession,
} from "../store/workspace";

const PING_INTERVAL_MS = 30_000;

export default function TerminalDock() {
  const session = useTerminalSession();
  if (!session) return null;
  const key = `${session.namespace}/${session.pod}/${session.container}`;
  return <DockSession key={key} session={session} />;
}

function DockSession({ session }: { session: TerminalSession }) {
  const hostRef = useRef<HTMLDivElement | null>(null);

  useEffect(() => {
    const host = hostRef.current;
    if (!host) return;

    const term = new Terminal({
      cursorBlink: true,
      fontFamily: '"JetBrains Mono", ui-monospace, Menlo, monospace',
      fontSize: 12,
      lineHeight: 1.25,
      theme: {
        background: "#0a0a0a",
        foreground: "#e8e6e0",
        cursor: "#c9b88a",
        cursorAccent: "#0a0a0a",
        selectionBackground: "rgba(201,184,138,0.25)",
        black: "#0a0a0a",
        red: "#d88a8a",
        green: "#7eb87e",
        yellow: "#d4b46a",
        blue: "#8aa3c9",
        magenta: "#c9a3c0",
        cyan: "#8ac0b8",
        white: "#e8e6e0",
        brightBlack: "#55514a",
        brightRed: "#e0a0a0",
        brightGreen: "#9ec79a",
        brightYellow: "#d8c89a",
        brightBlue: "#a0b8d8",
        brightMagenta: "#d8b8d0",
        brightCyan: "#a0d0c8",
        brightWhite: "#f4f2ec",
      },
    });
    const fit = new FitAddon();
    term.loadAddon(fit);
    term.open(host);

    let ws: WebSocket | null = null;
    let closed = false;
    let lastCols = -1;
    let lastRows = -1;
    let pingTimer: number | null = null;
    let resizeTimer: number | null = null;

    const sendResize = (): void => {
      if (!ws || ws.readyState !== WebSocket.OPEN) return;
      const { cols, rows } = term;
      if (cols === lastCols && rows === lastRows) return;
      lastCols = cols;
      lastRows = rows;
      ws.send(JSON.stringify({ type: "resize", cols, rows }));
    };

    const doFit = (): void => {
      try {
        fit.fit();
      } catch {
        /* host may be 0-sized mid-layout */
      }
      sendResize();
    };

    const onWindowResize = (): void => {
      if (resizeTimer !== null) window.clearTimeout(resizeTimer);
      resizeTimer = window.setTimeout(doFit, 120);
    };

    // Initial fit before connecting so we can report a real size.
    try {
      fit.fit();
    } catch {
      /* ignore */
    }

    const proto = window.location.protocol === "https:" ? "wss:" : "ws:";
    const qs = new URLSearchParams({
      namespace: session.namespace,
      pod: session.pod,
      container: session.container,
    });
    const url = `${proto}//${window.location.host}/api/exec?${qs.toString()}`;

    term.writeln(`\x1b[90mconnecting to ${session.namespace}/${session.pod} (${session.container})…\x1b[0m`);

    try {
      ws = new WebSocket(url);
    } catch {
      term.writeln("\x1b[31mfailed to open exec socket\x1b[0m");
      term.writeln("\x1b[90m[session ended]\x1b[0m");
    }

    if (ws) {
      const socket = ws;
      socket.onopen = () => {
        sendResize();
        pingTimer = window.setInterval(() => {
          if (socket.readyState === WebSocket.OPEN) {
            socket.send(JSON.stringify({ type: "ping" }));
          }
        }, PING_INTERVAL_MS);
      };
      socket.onmessage = (ev: MessageEvent<string>) => {
        let frame: { type?: string; data?: string; message?: string; error?: string };
        try {
          frame = JSON.parse(ev.data) as typeof frame;
        } catch {
          return;
        }
        switch (frame.type) {
          case "stdout":
            if (frame.data) term.write(frame.data);
            break;
          case "connected":
            term.writeln("\x1b[90mconnected.\x1b[0m");
            break;
          case "error":
            term.writeln(`\x1b[31m${frame.message ?? frame.error ?? frame.data ?? "exec error"}\x1b[0m`);
            break;
          case "exit":
            term.writeln("\r\n\x1b[90m[session ended]\x1b[0m");
            closed = true;
            break;
          default:
            break;
        }
      };
      socket.onerror = () => {
        if (!closed) term.writeln("\r\n\x1b[31mexec socket error\x1b[0m");
      };
      socket.onclose = () => {
        if (!closed) {
          term.writeln("\r\n\x1b[90m[session ended]\x1b[0m");
          closed = true;
        }
      };
    }

    const dataSub = term.onData((data) => {
      if (ws && ws.readyState === WebSocket.OPEN) {
        ws.send(JSON.stringify({ type: "stdin", data }));
      }
    });

    window.addEventListener("resize", onWindowResize);
    // One more fit after fonts settle / layout stabilizes.
    const settleTimer = window.setTimeout(doFit, 50);
    term.focus();

    return () => {
      window.removeEventListener("resize", onWindowResize);
      window.clearTimeout(settleTimer);
      if (pingTimer !== null) window.clearInterval(pingTimer);
      if (resizeTimer !== null) window.clearTimeout(resizeTimer);
      dataSub.dispose();
      try {
        ws?.close();
      } catch {
        /* ignore */
      }
      term.dispose();
    };
  }, [session.namespace, session.pod, session.container]);

  return (
    <div className="shrink-0 h-[40vh] min-h-[200px] border-t border-border-strong bg-bg-base flex flex-col z-20">
      {/* Header */}
      <div className="shrink-0 h-8 flex items-center gap-2 px-3 border-b border-border bg-bg-panel font-mono text-[11px]">
        <TerminalIcon size={12} className="text-accent" />
        <span className="text-text-muted">shell</span>
        <span className="text-border select-none" aria-hidden="true">│</span>
        <span className="text-text truncate">
          {session.namespace}/{session.pod}
        </span>
        <span className="text-text-muted truncate">· {session.container}</span>
        <button
          type="button"
          aria-label="Close terminal"
          onClick={() => workspaceActions.closeTerminal()}
          className="ml-auto inline-flex items-center justify-center h-6 w-6 text-text-muted hover:text-text hover:bg-bg-panel2 transition-colors k9s-square"
        >
          <X size={13} />
        </button>
      </div>
      {/* xterm host */}
      <div ref={hostRef} className="flex-1 min-h-0 px-2 py-1 overflow-hidden" />
    </div>
  );
}
