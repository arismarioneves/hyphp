import { useState } from 'react';
import { ArrowClockwise, Play, Pulse, Scroll, Stop, X } from '@phosphor-icons/react';
import { ServicesService } from '../../bindings/hyphp/services';
import { Button } from '../components/Button';
import { Card } from '../components/Card';
import { EmptyState } from '../components/EmptyState';
import { LogView } from '../components/LogView';
import { SectionLabel } from '../components/SectionLabel';
import { Skeleton } from '../components/Skeleton';
import { StatusDot, type ServiceState } from '../components/StatusDot';
import type { ScreenProps } from '../lib/screens';
import { formatUptime } from '../lib/status';
import type { ServiceStatus } from '../lib/types';
import { useNow } from '../lib/useNow';
import { useServices } from '../lib/useServices';

const GROUP_LABELS: Record<string, string> = {
  web: 'WEB SERVER',
  php: 'PHP',
  db: 'BANCO DE DADOS',
  mail: 'MAIL',
  proc: 'PROCESSOS DE PROJETO',
};
/** Ordem de exibição dos grupos; grupo desconhecido do Go cai para o fim. */
const GROUP_RANK: Record<string, number> = { web: 0, php: 1, db: 2, mail: 3, proc: 4 };
const LAST_RANK = 99;

type Action = 'start' | 'stop' | 'restart';

function groupServices(services: ServiceStatus[]): Array<{ group: string; items: ServiceStatus[] }> {
  const byGroup = new Map<string, ServiceStatus[]>();
  for (const s of services) {
    const arr = byGroup.get(s.group) ?? [];
    arr.push(s);
    byGroup.set(s.group, arr);
  }
  return [...byGroup.entries()]
    .sort(([a], [b]) => (GROUP_RANK[a] ?? LAST_RANK) - (GROUP_RANK[b] ?? LAST_RANK))
    .map(([group, items]) => ({ group, items }));
}

export function Services({ onNavigate }: ScreenProps) {
  const { services, loading } = useServices();
  const now = useNow(1000);
  const [openLog, setOpenLog] = useState<string | null>(null);
  const [pending, setPending] = useState<Record<string, Action>>({});
  const [errors, setErrors] = useState<Record<string, string>>({});

  const act = async (id: string, action: Action) => {
    setPending((p) => ({ ...p, [id]: action }));
    setErrors((e) => {
      const next = { ...e };
      delete next[id];
      return next;
    });
    try {
      if (action === 'start') await ServicesService.Start(id);
      else if (action === 'stop') await ServicesService.Stop(id);
      else await ServicesService.Restart(id);
    } catch (e) {
      setErrors((prev) => ({ ...prev, [id]: e instanceof Error ? e.message : String(e) }));
    } finally {
      setPending((p) => {
        const next = { ...p };
        delete next[id];
        return next;
      });
    }
  };

  if (loading && services.length === 0) return <Skeleton lines={8} />;
  if (services.length === 0) {
    return (
      <EmptyState
        icon={<Pulse size={24} />}
        title="Nenhum serviço"
        description="Instale um web server e PHP em Runtimes e adicione um projeto."
        action={
          <Button variant="primary" size="sm" onClick={() => onNavigate('runtimes')}>
            Abrir Runtimes
          </Button>
        }
      />
    );
  }

  const groups = groupServices(services);

  return (
    <div className={`grid h-full min-h-0 gap-4 ${openLog ? 'grid-cols-[1fr_480px]' : 'grid-cols-1'}`}>
      <div className="flex min-h-0 flex-col gap-4 overflow-y-auto">
        {groups.map(({ group, items }) => (
          <Card key={group}>
            <SectionLabel>{GROUP_LABELS[group] ?? group.toUpperCase()}</SectionLabel>
            <table className="mt-3 w-full text-sm">
              <thead className="text-left text-xs text-fg-faint">
                <tr>
                  <th className="w-8 py-1" />
                  <th className="py-1 font-normal">Serviço</th>
                  <th className="py-1 font-normal">PID</th>
                  <th className="py-1 font-normal">Porta</th>
                  <th className="py-1 font-normal">Uptime</th>
                  <th className="py-1 font-normal">Restarts</th>
                  <th className="py-1 text-right font-normal">Ações</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-border">
                {items.map((s) => {
                  const state = s.state as ServiceState;
                  const startable = state === 'stopped' || state === 'failed';
                  const running = state === 'ready' || state === 'degraded' || state === 'starting';
                  const busy = pending[s.id] !== undefined;
                  return (
                    <tr key={s.id} className="hover:bg-bg-card-hover">
                      <td className="py-2">
                        <StatusDot state={state} />
                      </td>
                      <td className="py-2">
                        <div className="flex flex-col">
                          <span className="text-fg">{s.name}</span>
                          <span className="selectable font-mono text-xs text-fg-faint">
                            {s.id} · {s.state}
                          </span>
                          {s.lastError && <span className="selectable text-xs text-err">{s.lastError}</span>}
                          {errors[s.id] && <span className="selectable text-xs text-err">{errors[s.id]}</span>}
                        </div>
                      </td>
                      <td className="py-2 font-mono text-fg-muted">{s.pid || '—'}</td>
                      <td className="py-2 font-mono text-fg-muted">{s.port || '—'}</td>
                      <td className="py-2 font-mono text-fg-muted">
                        {state === 'ready' || state === 'degraded' ? formatUptime(s.startedAt, now) : '—'}
                      </td>
                      <td className="py-2 font-mono text-fg-muted">{s.restarts}</td>
                      <td className="py-2">
                        <div className="flex items-center justify-end gap-1">
                          <Button
                            variant="ghost"
                            size="sm"
                            aria-label={`Iniciar ${s.name}`}
                            icon={<Play size={14} weight="fill" />}
                            disabled={busy || !startable}
                            loading={pending[s.id] === 'start'}
                            onClick={() => void act(s.id, 'start')}
                          />
                          <Button
                            variant="ghost"
                            size="sm"
                            aria-label={`Parar ${s.name}`}
                            icon={<Stop size={14} weight="fill" />}
                            disabled={busy || !running}
                            loading={pending[s.id] === 'stop'}
                            onClick={() => void act(s.id, 'stop')}
                          />
                          <Button
                            variant="ghost"
                            size="sm"
                            aria-label={`Reiniciar ${s.name}`}
                            icon={<ArrowClockwise size={14} />}
                            disabled={busy || !running}
                            loading={pending[s.id] === 'restart'}
                            onClick={() => void act(s.id, 'restart')}
                          />
                          <Button
                            variant={openLog === s.id ? 'secondary' : 'ghost'}
                            size="sm"
                            aria-label={`Log de ${s.name}`}
                            aria-pressed={openLog === s.id}
                            icon={<Scroll size={14} />}
                            onClick={() => setOpenLog(openLog === s.id ? null : s.id)}
                          />
                        </div>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </Card>
        ))}
      </div>

      {openLog && (
        <div className="flex min-h-0 flex-col gap-2">
          <div className="flex items-center justify-between">
            <SectionLabel className="selectable font-mono">{openLog}</SectionLabel>
            <Button variant="ghost" size="sm" aria-label="Fechar log" icon={<X size={14} />} onClick={() => setOpenLog(null)} />
          </div>
          <LogView id={openLog} className="min-h-0 flex-1" />
        </div>
      )}
    </div>
  );
}
