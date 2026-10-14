import type { ReactNode } from 'react'
import { Card } from './Card'
import { SectionLabel } from './SectionLabel'

/** Card de linhas empilhadas, com rótulo opcional no topo. */
export function Section({ label, children }: { label?: string; children: ReactNode }) {
  return (
    <Card>
      {label && <SectionLabel className="mb-3">{label}</SectionLabel>}
      <div className="flex flex-col gap-3">{children}</div>
    </Card>
  )
}

/** Rótulo e dica à esquerda, controle à direita. */
export function Row({ label, hint, children }: { label: string; hint?: string; children: ReactNode }) {
  return (
    <div className="flex items-center justify-between gap-4 text-sm">
      <div className="flex flex-col">
        <span className="text-fg">{label}</span>
        {hint && <span className="text-xs text-fg-muted">{hint}</span>}
      </div>
      {children}
    </div>
  )
}
