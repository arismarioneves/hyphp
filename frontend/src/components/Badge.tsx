import type { ReactNode } from 'react';

type Tone = 'neutral' | 'accent' | 'ok' | 'warn' | 'err';

type BadgeProps = { tone?: Tone; mono?: boolean; children: ReactNode };

const TONES: Record<Tone, string> = {
  neutral: 'bg-bg-card-hover text-fg-muted',
  accent: 'bg-accent-soft text-accent-fg',
  ok: 'bg-ok/15 text-ok',
  warn: 'bg-warn/15 text-warn',
  err: 'bg-err/15 text-err',
};

export function Badge({ tone = 'neutral', mono = false, children }: BadgeProps) {
  return (
    <span
      className={`inline-flex items-center rounded-pill px-2 py-0.5 text-[11px] font-medium ${TONES[tone]} ${
        mono ? 'font-mono' : ''
      }`}
    >
      {children}
    </span>
  );
}
