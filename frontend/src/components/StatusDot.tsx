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
  const animate = pulse ?? (state === 'starting' || state === 'stopping');
  return (
    <span
      aria-label={state}
      title={state}
      className={`inline-block h-2 w-2 shrink-0 rounded-full ${COLORS[state]} ${animate ? 'animate-pulse' : ''} ${className}`}
    />
  );
}
