import { useState } from 'react'
import { ArrowCircleUp, CheckCircle, X } from '@phosphor-icons/react'
import { Button } from './Button'
import { useUpdate } from '../lib/useUpdate'

// Faixa acima do conteúdo, em qualquer tela: versão pronta para instalar, ou
// o resultado do update que acabou de ser aplicado.
export function UpdateBanner() {
  const { status, busy, error, apply } = useUpdate()
  const [dismissed, setDismissed] = useState(false)
  if (!status) return null

  const state = status.state as string
  if (state === 'pronto' || state === 'aplicando') {
    return (
      <div className="flex items-center gap-3 border-b border-border bg-accent-soft px-6 py-2 text-sm">
        <ArrowCircleUp size={18} className="shrink-0 text-accent-fg" />
        <span className="text-fg">
          HyPHP <span className="font-mono">{status.available}</span> pronto para instalar.
          <span className="text-fg-muted"> Os serviços param e o Windows pede permissão.</span>
        </span>
        {(error || status.error) && <span className="selectable text-err">{error || status.error}</span>}
        <Button
          variant="primary"
          size="sm"
          className="ml-auto shrink-0"
          loading={busy || state === 'aplicando'}
          onClick={() => void apply()}
        >
          Atualizar e reiniciar
        </Button>
      </div>
    )
  }

  if (status.message && !dismissed) {
    return (
      <div className="flex items-center gap-3 border-b border-border bg-bg-card px-6 py-2 text-sm">
        <CheckCircle size={18} className="shrink-0 text-ok" />
        <span className="text-fg">{status.message}</span>
        <button
          type="button"
          className="ml-auto text-fg-muted hover:text-fg"
          aria-label="Dispensar"
          onClick={() => setDismissed(true)}
        >
          <X size={16} />
        </button>
      </div>
    )
  }
  return null
}
