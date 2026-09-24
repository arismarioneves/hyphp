import { useEffect, useRef, useState } from 'react';
import { Stream, type WailsSocket } from '@wailsio/runtime';
import { ArrowLineDown, Broom } from '@phosphor-icons/react';
import { Button } from './Button';

type LogViewProps = {
  /** id do serviço supervisionado (ex.: "web:apache", "proc:acme:queue") */
  id: string;
  className?: string;
  maxLines?: number;
};

/** O que Stream() devolve: socket do runtime no app, WebSocket em build de servidor. */
type StreamHandle = WailsSocket | WebSocket;

const ERROR_PREFIX = '!error:';
const RECONNECT_MS = 1000;

/**
 * Log ao vivo de um serviço via Stream("logs").
 * Handshake: primeiro frame enviado = {"id":"<id>"}; cada frame recebido = uma linha.
 */
export function LogView({ id, className = '', maxLines = 2000 }: LogViewProps) {
  const [lines, setLines] = useState<string[]>([]);
  const [filter, setFilter] = useState('');
  const [follow, setFollow] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [connected, setConnected] = useState(false);

  const boxRef = useRef<HTMLDivElement>(null);
  const pendingRef = useRef<string[]>([]);
  const flushRef = useRef<number | null>(null);
  const mountedRef = useRef(true);

  // conexão + reconexão
  useEffect(() => {
    mountedRef.current = true;
    setLines([]);
    setError(null);
    let stream: StreamHandle | null = null;
    let timer = 0;
    // os dois frames de erro do servidor (id desconhecido, hello inválido) são
    // permanentes: reconectar só repetiria o mesmo erro a cada segundo.
    let fatal = false;
    const decoder = new TextDecoder();

    const flush = () => {
      flushRef.current = null;
      const batch = pendingRef.current;
      if (batch.length === 0) return;
      pendingRef.current = [];
      setLines((prev) => {
        const next = prev.concat(batch);
        return next.length > maxLines ? next.slice(next.length - maxLines) : next;
      });
    };

    const connect = () => {
      if (!mountedRef.current) return;
      const s = Stream('logs');
      stream = s;
      s.onopen = () => {
        setConnected(true);
        s.send(new TextEncoder().encode(JSON.stringify({ id })));
      };
      s.onmessage = (ev: MessageEvent<ArrayBuffer>) => {
        const line = decoder.decode(new Uint8Array(ev.data));
        if (line.startsWith(ERROR_PREFIX)) {
          fatal = true;
          setError(line.slice(ERROR_PREFIX.length).trim());
          return;
        }
        pendingRef.current.push(line);
        if (flushRef.current === null) flushRef.current = requestAnimationFrame(flush);
      };
      s.onerror = () => {
        setConnected(false);
      };
      s.onclose = () => {
        setConnected(false);
        if (mountedRef.current && !fatal) timer = window.setTimeout(connect, RECONNECT_MS);
      };
    };

    connect();

    return () => {
      mountedRef.current = false;
      clearTimeout(timer);
      if (flushRef.current !== null) cancelAnimationFrame(flushRef.current);
      pendingRef.current = [];
      if (stream) {
        stream.onclose = null;
        stream.close();
      }
    };
  }, [id, maxLines]);

  // auto-scroll enquanto "seguir" estiver ativo
  useEffect(() => {
    if (!follow || !boxRef.current) return;
    boxRef.current.scrollTop = boxRef.current.scrollHeight;
  }, [lines, follow]);

  const onScroll = () => {
    const el = boxRef.current;
    if (!el) return;
    const atBottom = el.scrollHeight - el.scrollTop - el.clientHeight < 8;
    setFollow(atBottom);
  };

  const visible = filter ? lines.filter((l) => l.toLowerCase().includes(filter.toLowerCase())) : lines;

  return (
    <div className={`flex min-h-0 flex-col overflow-hidden rounded-card border border-border bg-bg-sidebar ${className}`}>
      <div className="flex items-center gap-2 border-b border-border px-3 py-2">
        <span className={`h-2 w-2 rounded-full ${connected ? 'bg-ok' : 'bg-fg-faint'}`} aria-hidden />
        <span className="font-mono text-xs text-fg-muted">{id}</span>
        <input
          type="search"
          value={filter}
          onChange={(e) => setFilter(e.target.value)}
          placeholder="filtrar…"
          aria-label="Filtrar linhas do log"
          className="ml-auto h-7 w-48 rounded-pill border border-border bg-bg-card px-2 text-xs text-fg outline-none focus-visible:outline-2 focus-visible:outline-accent"
        />
        <Button variant="ghost" size="sm" aria-label="Limpar log" icon={<Broom size={14} />} onClick={() => setLines([])} />
        <Button
          variant={follow ? 'secondary' : 'ghost'}
          size="sm"
          aria-label="Seguir o fim do log"
          aria-pressed={follow}
          icon={<ArrowLineDown size={14} />}
          onClick={() => {
            setFollow(true);
            if (boxRef.current) boxRef.current.scrollTop = boxRef.current.scrollHeight;
          }}
        />
      </div>
      {error && <div className="border-b border-border px-3 py-2 text-xs text-err">{error}</div>}
      <div
        ref={boxRef}
        onScroll={onScroll}
        className="selectable min-h-0 flex-1 overflow-auto p-3 font-mono text-xs leading-5 text-fg"
      >
        {visible.length === 0 ? (
          <span className="text-fg-faint">{connected ? 'Sem linhas ainda.' : 'Conectando…'}</span>
        ) : (
          visible.map((l, i) => (
            <div key={i} className="whitespace-pre-wrap break-all">
              {l}
            </div>
          ))
        )}
      </div>
    </div>
  );
}
