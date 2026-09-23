import type { ReactNode } from 'react';

type SectionLabelProps = { children: ReactNode; className?: string };

/** Rótulo uppercase estilo "YOUR STACK" (spec §12.2, --fg-faint). */
export function SectionLabel({ children, className = '' }: SectionLabelProps) {
  return (
    <div className={`text-[11px] font-medium uppercase tracking-wider text-fg-faint ${className}`}>
      {children}
    </div>
  );
}
