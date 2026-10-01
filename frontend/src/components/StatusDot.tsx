import { useT } from '../i18n';

/** Espelho de supervisor.State (Go). */
export type ServiceState = 'stopped' | 'starting' | 'ready' | 'degraded' | 'stopping' | 'failed';

type StatusDotProps = { state: ServiceState; pulse?: boolean; className?: string };

const COLORS: Record<ServiceState, string> = {
  ready: 'bg-ok',
  starting: 'bg-warn',
  degraded: 'bg-warn',
  stopping: 'bg-warn',
  failed: 'bg-err',
  stopped: 'bg-fg-faint',
};

export function StatusDot({ state, pulse, className = '' }: StatusDotProps) {
  const t = useT('common');
  const animate = pulse ?? (state === 'starting' || state === 'stopping');
  const label = t(`state_${state}`);
  // Sem `role`, leitores de tela ignoram o aria-label de um <span>; em várias
  // listas o ponto é o único indicador do estado.
  return (
    <span
      role="img"
      aria-label={label}
      title={label}
      className={`inline-block h-2 w-2 shrink-0 rounded-full ${COLORS[state]} ${animate ? 'animate-pulse' : ''} ${className}`}
    />
  );
}
