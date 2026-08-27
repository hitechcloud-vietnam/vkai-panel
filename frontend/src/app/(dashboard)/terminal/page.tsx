'use client';

import { useState, useEffect, useRef, useCallback } from 'react';
import {
  Terminal as TerminalIcon,
  Plus,
  X,
  Wifi,
  WifiOff,
  Loader2,
  ChevronLeft,
  ChevronRight,
  Monitor,
  Maximize2,
  Minimize2,
} from 'lucide-react';
import { Button } from '@/components/ui/button';
import { cn } from '@/lib/utils';

// ---------------------------------------------------------------------------
// Dynamic import for xterm (SSR-safe)
// ---------------------------------------------------------------------------
let XTerminal: any = null;
let FitAddon: any = null;
let WebLinksAddon: any = null;

async function loadXterm() {
  if (typeof window === 'undefined') return;
  const xterm = await import('xterm');
  const fit = await import('xterm-addon-fit');
  const webLinks = await import('xterm-addon-web-links');
  await import('xterm/css/xterm.css');
  XTerminal = xterm.Terminal;
  FitAddon = fit.FitAddon;
  WebLinksAddon = webLinks.WebLinksAddon;
}

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------
type ConnectionStatus = 'connecting' | 'connected' | 'disconnected';

interface TerminalSession {
  id: string;
  name: string;
  terminal: any | null;
  fitAddon: any | null;
  ws: WebSocket | null;
  status: ConnectionStatus;
  createdAt: Date;
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------
function generateId(): string {
  return `term-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;
}

function getWsUrl(sessionId: string): string {
  if (typeof window === 'undefined') return '';
  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
  const host = window.location.host;
  const token = localStorage.getItem('access_token') || '';
  return `${protocol}//${host}/api/ws?token=${encodeURIComponent(token)}&session=${sessionId}`;
}

function formatTime(date: Date): string {
  return date.toLocaleTimeString('en-US', {
    hour: '2-digit',
    minute: '2-digit',
    hour12: false,
  });
}

// ---------------------------------------------------------------------------
// TerminalTheme — matches the dark-950 / dark-900 palette
// ---------------------------------------------------------------------------
const TERMINAL_THEME = {
  background: '#020617',
  foreground: '#e2e8f0',
  cursor: '#3b82f6',
  cursorAccent: '#020617',
  selectionBackground: '#1e40af55',
  selectionForeground: '#f8fafc',
  black: '#1e293b',
  red: '#ef4444',
  green: '#22c55e',
  yellow: '#eab308',
  blue: '#3b82f6',
  magenta: '#a855f7',
  cyan: '#06b6d4',
  white: '#e2e8f0',
  brightBlack: '#475569',
  brightRed: '#f87171',
  brightGreen: '#4ade80',
  brightYellow: '#facc15',
  brightBlue: '#60a5fa',
  brightMagenta: '#c084fc',
  brightCyan: '#22d3ee',
  brightWhite: '#f8fafc',
};

// ---------------------------------------------------------------------------
// StatusIndicator
// ---------------------------------------------------------------------------
function StatusIndicator({ status }: { status: ConnectionStatus }) {
  const config = {
    connecting: {
      dot: 'bg-yellow-500',
      icon: <Loader2 className="h-3 w-3 animate-spin text-yellow-500" />,
      label: 'Connecting',
      text: 'text-yellow-500',
    },
    connected: {
      dot: 'bg-green-500',
      icon: <Wifi className="h-3 w-3 text-green-500" />,
      label: 'Connected',
      text: 'text-green-500',
    },
    disconnected: {
      dot: 'bg-red-500',
      icon: <WifiOff className="h-3 w-3 text-red-500" />,
      label: 'Disconnected',
      text: 'text-red-500',
    },
  };

  const c = config[status];

  return (
    <div className="flex items-center gap-1.5">
      <span className={cn('h-2 w-2 rounded-full', c.dot)} />
      <span className={cn('text-xs font-medium', c.text)}>{c.label}</span>
    </div>
  );
}

// ---------------------------------------------------------------------------
// TerminalPage
// ---------------------------------------------------------------------------
export default function TerminalPage() {
  const [sessions, setSessions] = useState<TerminalSession[]>([]);
  const [activeSessionId, setActiveSessionId] = useState<string | null>(null);
  const [sidebarOpen, setSidebarOpen] = useState(true);
  const [xtermLoaded, setXtermLoaded] = useState(false);
  const [isFullscreen, setIsFullscreen] = useState(false);

  const terminalContainerRefs = useRef<Map<string, HTMLDivElement>>(new Map());
  const sessionCounterRef = useRef(0);
  const sessionsRef = useRef<TerminalSession[]>([]);

  // Keep ref in sync
  useEffect(() => {
    sessionsRef.current = sessions;
  }, [sessions]);

  // Load xterm.js on mount
  useEffect(() => {
    loadXterm().then(() => setXtermLoaded(true));
  }, []);

  // ── Create a new terminal session ──────────────────────────────────────
  const createSession = useCallback(
    (autoConnect = true) => {
      if (!xtermLoaded || !XTerminal) return;

      sessionCounterRef.current += 1;
      const id = generateId();
      const name = `Session ${sessionCounterRef.current}`;

      const term = new XTerminal({
        theme: TERMINAL_THEME,
        fontFamily: "'JetBrains Mono', 'Fira Code', 'Cascadia Code', Menlo, monospace",
        fontSize: 14,
        lineHeight: 1.2,
        cursorBlink: true,
        cursorStyle: 'bar',
        cursorWidth: 2,
        scrollback: 10000,
        tabStopWidth: 4,
        bellStyle: 'none',
        allowTransparency: true,
        convertEol: true,
      });

      const fitAddonInstance = new FitAddon();
      const webLinksAddonInstance = new WebLinksAddon();
      term.loadAddon(fitAddonInstance);
      term.loadAddon(webLinksAddonInstance);

      const session: TerminalSession = {
        id,
        name,
        terminal: term,
        fitAddon: fitAddonInstance,
        ws: null,
        status: 'disconnected',
        createdAt: new Date(),
      };

      setSessions((prev) => [...prev, session]);
      setActiveSessionId(id);

      if (autoConnect) {
        setTimeout(() => connectSession(id), 50);
      }
    },
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [xtermLoaded],
  );

  // ── Connect a session's WebSocket ──────────────────────────────────────
  const connectSession = useCallback((sessionId: string) => {
    const session = sessionsRef.current.find((s) => s.id === sessionId);
    if (!session || !session.terminal) return;

    // Mark connecting
    setSessions((prev) =>
      prev.map((s) =>
        s.id === sessionId ? { ...s, status: 'connecting' as ConnectionStatus } : s,
      ),
    );

    const wsUrl = getWsUrl(sessionId);
    const ws = new WebSocket(wsUrl);

    ws.onopen = () => {
      setSessions((prev) =>
        prev.map((s) =>
          s.id === sessionId ? { ...s, status: 'connected' as ConnectionStatus, ws } : s,
        ),
      );

      // Join terminal room
      ws.send(
        JSON.stringify({
          type: 'join_room',
          payload: { room: `terminal:${sessionId}` },
        }),
      );

      session.terminal.writeln('\x1b[1;34m╔══════════════════════════════════════════════╗\x1b[0m');
      session.terminal.writeln('\x1b[1;34m║\x1b[0m  \x1b[1;36mvKAI Terminal\x1b[0m — Session Connected            \x1b[1;34m║\x1b[0m');
      session.terminal.writeln('\x1b[1;34m╚══════════════════════════════════════════════╝\x1b[0m');
      session.terminal.writeln('');
    };

    ws.onmessage = (event: MessageEvent) => {
      try {
        const msg = JSON.parse(event.data);
        if (msg.type === 'terminal_output' && msg.payload?.data) {
          session.terminal.write(msg.payload.data);
        } else if (msg.type === 'error') {
          session.terminal.writeln(
            `\x1b[1;31mError: ${msg.payload?.message || 'Unknown error'}\x1b[0m`,
          );
        }
      } catch {
        // Raw terminal data
        session.terminal.write(event.data);
      }
    };

    ws.onclose = () => {
      setSessions((prev) =>
        prev.map((s) =>
          s.id === sessionId ? { ...s, status: 'disconnected' as ConnectionStatus, ws: null } : s,
        ),
      );
      session.terminal.writeln('');
      session.terminal.writeln('\x1b[1;31m[Connection closed]\x1b[0m');
    };

    ws.onerror = () => {
      setSessions((prev) =>
        prev.map((s) =>
          s.id === sessionId ? { ...s, status: 'disconnected' as ConnectionStatus } : s,
        ),
      );
      session.terminal.writeln('\x1b[1;31m[Connection error]\x1b[0m');
    };

    // Wire terminal input → WebSocket
    session.terminal.onData((data: string) => {
      if (ws.readyState === WebSocket.OPEN) {
        ws.send(
          JSON.stringify({
            type: 'terminal_input',
            room: `terminal:${sessionId}`,
            payload: { data },
          }),
        );
      }
    });

    // Wire terminal resize → WebSocket
    session.terminal.onResize(({ cols, rows }: { cols: number; rows: number }) => {
      if (ws.readyState === WebSocket.OPEN) {
        ws.send(
          JSON.stringify({
            type: 'terminal_resize',
            room: `terminal:${sessionId}`,
            payload: { cols, rows },
          }),
        );
      }
    });

    setSessions((prev) =>
      prev.map((s) => (s.id === sessionId ? { ...s, ws } : s)),
    );
  }, []);

  // ── Mount terminal to DOM ──────────────────────────────────────────────
  useEffect(() => {
    if (!activeSessionId) return;
    const session = sessions.find((s) => s.id === activeSessionId);
    if (!session || !session.terminal) return;

    const container = terminalContainerRefs.current.get(activeSessionId);
    if (!container) return;

    // Only open if not already attached
    if (container.childElementCount === 0) {
      session.terminal.open(container);
      requestAnimationFrame(() => {
        try {
          session.fitAddon?.fit();
        } catch {
          /* ignore */
        }
        session.terminal.focus();
      });
    }
  }, [activeSessionId, sessions]);

  // ── Handle window resize ───────────────────────────────────────────────
  useEffect(() => {
    const handleResize = () => {
      if (!activeSessionId) return;
      const session = sessionsRef.current.find((s) => s.id === activeSessionId);
      if (session?.fitAddon) {
        try {
          session.fitAddon.fit();
        } catch {
          /* ignore fit errors during transitions */
        }
      }
    };

    window.addEventListener('resize', handleResize);
    return () => window.removeEventListener('resize', handleResize);
  }, [activeSessionId]);

  // ── Close a session ────────────────────────────────────────────────────
  const closeSession = useCallback(
    (sessionId: string) => {
      const session = sessionsRef.current.find((s) => s.id === sessionId);
      if (!session) return;

      if (session.ws) session.ws.close();
      if (session.terminal) session.terminal.dispose();

      setSessions((prev) => {
        const remaining = prev.filter((s) => s.id !== sessionId);
        if (activeSessionId === sessionId) {
          setActiveSessionId(remaining.length > 0 ? remaining[remaining.length - 1].id : null);
        }
        return remaining;
      });
    },
    [activeSessionId],
  );

  // ── Reconnect a session ────────────────────────────────────────────────
  const reconnectSession = useCallback(
    (sessionId: string) => {
      const session = sessionsRef.current.find((s) => s.id === sessionId);
      if (!session) return;

      if (session.ws) session.ws.close();
      session.terminal.clear();
      connectSession(sessionId);
    },
    [connectSession],
  );

  // ── Create first session on xterm load ─────────────────────────────────
  useEffect(() => {
    if (xtermLoaded && sessions.length === 0) {
      createSession();
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [xtermLoaded]);

  // ── Fullscreen toggle ──────────────────────────────────────────────────
  const toggleFullscreen = useCallback(() => {
    if (!document.fullscreenElement) {
      document.documentElement.requestFullscreen();
      setIsFullscreen(true);
    } else {
      document.exitFullscreen();
      setIsFullscreen(false);
    }
  }, []);

  useEffect(() => {
    const handler = () => setIsFullscreen(!!document.fullscreenElement);
    document.addEventListener('fullscreenchange', handler);
    return () => document.removeEventListener('fullscreenchange', handler);
  }, []);

  const activeSession = sessions.find((s) => s.id === activeSessionId);

  // ── Loading state ──────────────────────────────────────────────────────
  if (!xtermLoaded) {
    return (
      <div className="flex items-center justify-center h-[calc(100vh-8rem)]">
        <div className="flex flex-col items-center gap-4">
          <Loader2 className="h-8 w-8 animate-spin text-primary-500" />
          <p className="text-dark-400 text-sm">Loading terminal...</p>
        </div>
      </div>
    );
  }

  // ── Render ─────────────────────────────────────────────────────────────
  return (
    <div
      className={cn(
        'flex flex-col',
        isFullscreen
          ? 'fixed inset-0 z-50 bg-dark-950'
          : 'h-[calc(100vh-8rem)] -m-6',
      )}
    >
      {/* ── Top Bar ─────────────────────────────────────────────────────── */}
      <div className="flex items-center justify-between border-b border-dark-700 bg-dark-900 px-4 py-2">
        <div className="flex items-center gap-3">
          <TerminalIcon className="h-5 w-5 text-primary-400" />
          <h1 className="text-sm font-semibold text-dark-100">Web Terminal</h1>
          {activeSession && <StatusIndicator status={activeSession.status} />}
        </div>

        <div className="flex items-center gap-2">
          <Button
            variant="ghost"
            size="sm"
            onClick={() => createSession()}
            className="h-7 gap-1.5 text-xs text-dark-300 hover:text-dark-100 hover:bg-dark-700"
          >
            <Plus className="h-3.5 w-3.5" />
            New Session
          </Button>
          <div className="h-4 w-px bg-dark-700" />
          <Button
            variant="ghost"
            size="sm"
            onClick={() => setSidebarOpen(!sidebarOpen)}
            className="h-7 w-7 p-0 text-dark-400 hover:text-dark-100 hover:bg-dark-700"
            title={sidebarOpen ? 'Hide sidebar' : 'Show sidebar'}
          >
            {sidebarOpen ? (
              <ChevronRight className="h-3.5 w-3.5" />
            ) : (
              <ChevronLeft className="h-3.5 w-3.5" />
            )}
          </Button>
          <Button
            variant="ghost"
            size="sm"
            onClick={toggleFullscreen}
            className="h-7 w-7 p-0 text-dark-400 hover:text-dark-100 hover:bg-dark-700"
            title={isFullscreen ? 'Exit fullscreen' : 'Fullscreen'}
          >
            {isFullscreen ? (
              <Minimize2 className="h-3.5 w-3.5" />
            ) : (
              <Maximize2 className="h-3.5 w-3.5" />
            )}
          </Button>
        </div>
      </div>

      <div className="flex flex-1 overflow-hidden">
        {/* ── Sidebar ──────────────────────────────────────────────────── */}
        {sidebarOpen && (
          <div className="w-64 flex-shrink-0 border-r border-dark-700 bg-dark-900/50 flex flex-col">
            <div className="p-3 border-b border-dark-700/50">
              <div className="flex items-center justify-between">
                <span className="text-xs font-medium text-dark-400 uppercase tracking-wider">
                  Sessions
                </span>
                <span className="text-xs text-dark-500">{sessions.length}</span>
              </div>
            </div>

            <div className="flex-1 overflow-y-auto p-2 space-y-1">
              {sessions.length === 0 ? (
                <div className="flex flex-col items-center justify-center py-8 text-dark-500">
                  <Monitor className="h-8 w-8 mb-2 opacity-50" />
                  <p className="text-xs">No active sessions</p>
                </div>
              ) : (
                sessions.map((session) => (
                  <div
                    key={session.id}
                    onClick={() => setActiveSessionId(session.id)}
                    className={cn(
                      'group flex items-center gap-2 rounded-md px-3 py-2 cursor-pointer transition-colors',
                      session.id === activeSessionId
                        ? 'bg-primary-600/20 border border-primary-500/30 text-dark-100'
                        : 'hover:bg-dark-700/50 text-dark-300 border border-transparent',
                    )}
                  >
                    <span
                      className={cn(
                        'h-2 w-2 rounded-full flex-shrink-0',
                        session.status === 'connected'
                          ? 'bg-green-500'
                          : session.status === 'connecting'
                            ? 'bg-yellow-500 animate-pulse'
                            : 'bg-red-500',
                      )}
                    />
                    <div className="flex-1 min-w-0">
                      <p className="text-xs font-medium truncate">{session.name}</p>
                      <p className="text-[10px] text-dark-500">
                        {formatTime(session.createdAt)}
                      </p>
                    </div>
                    <div className="flex items-center gap-1 opacity-0 group-hover:opacity-100 transition-opacity">
                      {session.status === 'disconnected' && (
                        <button
                          onClick={(e) => {
                            e.stopPropagation();
                            reconnectSession(session.id);
                          }}
                          className="p-0.5 rounded hover:bg-dark-600 text-dark-400 hover:text-green-400"
                          title="Reconnect"
                        >
                          <Wifi className="h-3 w-3" />
                        </button>
                      )}
                      <button
                        onClick={(e) => {
                          e.stopPropagation();
                          closeSession(session.id);
                        }}
                        className="p-0.5 rounded hover:bg-dark-600 text-dark-400 hover:text-red-400"
                        title="Close session"
                      >
                        <X className="h-3 w-3" />
                      </button>
                    </div>
                  </div>
                ))
              )}
            </div>

            <div className="p-3 border-t border-dark-700/50">
              <Button
                variant="outline"
                size="sm"
                onClick={() => createSession()}
                className="w-full h-8 text-xs border-dark-600 bg-dark-800 text-dark-300 hover:bg-dark-700 hover:text-dark-100"
              >
                <Plus className="h-3.5 w-3.5 mr-1.5" />
                New Session
              </Button>
            </div>
          </div>
        )}

        {/* ── Terminal Area ────────────────────────────────────────────── */}
        <div className="flex-1 flex flex-col overflow-hidden bg-dark-950">
          {/* Session Tabs (only when multiple sessions) */}
          {sessions.length > 1 && (
            <div className="flex items-center border-b border-dark-700 bg-dark-900/80 overflow-x-auto">
              {sessions.map((session) => (
                <div
                  key={session.id}
                  onClick={() => setActiveSessionId(session.id)}
                  className={cn(
                    'group flex items-center gap-2 px-4 py-2 text-xs cursor-pointer border-r border-dark-700/50 transition-colors min-w-[120px]',
                    session.id === activeSessionId
                      ? 'bg-dark-950 text-dark-100 border-b-2 border-b-primary-500'
                      : 'text-dark-400 hover:bg-dark-800 hover:text-dark-200',
                  )}
                >
                  <span
                    className={cn(
                      'h-1.5 w-1.5 rounded-full flex-shrink-0',
                      session.status === 'connected'
                        ? 'bg-green-500'
                        : session.status === 'connecting'
                          ? 'bg-yellow-500 animate-pulse'
                          : 'bg-red-500',
                    )}
                  />
                  <span className="truncate flex-1">{session.name}</span>
                  <button
                    onClick={(e) => {
                      e.stopPropagation();
                      closeSession(session.id);
                    }}
                    className="p-0.5 rounded opacity-0 group-hover:opacity-100 hover:bg-dark-600 text-dark-500 hover:text-red-400 transition-all"
                  >
                    <X className="h-3 w-3" />
                  </button>
                </div>
              ))}
            </div>
          )}

          {/* Terminal containers */}
          <div className="flex-1 relative overflow-hidden">
            {sessions.length === 0 ? (
              <div className="flex flex-col items-center justify-center h-full text-dark-500">
                <TerminalIcon className="h-16 w-16 mb-4 opacity-20" />
                <p className="text-lg font-medium mb-2">No Terminal Sessions</p>
                <p className="text-sm mb-6">Create a new session to get started</p>
                <Button
                  onClick={() => createSession()}
                  className="gap-2 bg-primary-600 hover:bg-primary-700 text-white"
                >
                  <Plus className="h-4 w-4" />
                  New Session
                </Button>
              </div>
            ) : (
              sessions.map((session) => (
                <div
                  key={session.id}
                  ref={(el) => {
                    if (el) {
                      terminalContainerRefs.current.set(session.id, el);
                    } else {
                      terminalContainerRefs.current.delete(session.id);
                    }
                  }}
                  className={cn(
                    'absolute inset-0 p-1',
                    session.id === activeSessionId ? 'visible' : 'invisible',
                  )}
                  style={{ height: '100%', width: '100%' }}
                />
              ))
            )}
          </div>

          {/* Bottom Status Bar */}
          {activeSession && (
            <div className="flex items-center justify-between border-t border-dark-700 bg-dark-900 px-4 py-1.5">
              <div className="flex items-center gap-4 text-[11px] text-dark-500">
                <span className="flex items-center gap-1.5">
                  <TerminalIcon className="h-3 w-3" />
                  {activeSession.name}
                </span>
                <span>
                  {activeSession.terminal
                    ? `${activeSession.terminal.cols}×${activeSession.terminal.rows}`
                    : '—'}
                </span>
              </div>
              <div className="flex items-center gap-4 text-[11px] text-dark-500">
                {activeSession.status === 'disconnected' && (
                  <button
                    onClick={() => reconnectSession(activeSession.id)}
                    className="flex items-center gap-1 text-yellow-500 hover:text-yellow-400 transition-colors"
                  >
                    <Wifi className="h-3 w-3" />
                    Reconnect
                  </button>
                )}
                <span>vKAI Terminal</span>
              </div>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
