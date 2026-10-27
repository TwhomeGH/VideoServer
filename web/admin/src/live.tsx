import { createContext, useContext, useEffect, useRef, useState, type ReactNode } from 'react';

type LiveKind = 'jobs' | 'scan' | 'config';
type LiveVersions = Record<LiveKind, number>;

const LiveContext = createContext<{ versions: LiveVersions; connected: boolean } | null>(null);

/** LiveProvider 訂閱 SSE，事件只推進版本號；各頁用版本號觸發自己的快照更新。 */
export function LiveProvider({ children }: { children: ReactNode }) {
  const [versions, setVersions] = useState<LiveVersions>({ jobs: 0, scan: 0, config: 0 });
  const [connected, setConnected] = useState(false);
  const firstReady = useRef(true);

  useEffect(() => {
    let source: EventSource | null = null;
    let closed = false;
    const bump = (kind: LiveKind) => {
      setVersions(current => ({ ...current, [kind]: current[kind] + 1 }));
    };
    const bumpAll = () => {
      setVersions(current => ({ jobs: current.jobs + 1, scan: current.scan + 1, config: current.config + 1 }));
    };
    const connect = () => {
      if (closed) return;
      source = new EventSource('/api/v1/events');
      source.onopen = () => setConnected(true);
      source.onerror = () => setConnected(false);
      source.addEventListener('ready', () => {
        setConnected(true);
        if (firstReady.current) {
          firstReady.current = false;
          bumpAll();
        }
      });
      source.addEventListener('reset', () => bumpAll());
      source.addEventListener('jobs', () => bump('jobs'));
      source.addEventListener('scan', () => bump('scan'));
      source.addEventListener('config', () => bump('config'));
    };
    connect();
    return () => {
      closed = true;
      source?.close();
      setConnected(false);
    };
  }, []);

  return <LiveContext.Provider value={{ versions, connected }}>{children}</LiveContext.Provider>;
}

export function useLiveVersion(kind: LiveKind): number {
  const context = useContext(LiveContext);
  return context?.versions[kind] ?? 0;
}

export function useLiveConnected(): boolean {
  return useContext(LiveContext)?.connected ?? false;
}
