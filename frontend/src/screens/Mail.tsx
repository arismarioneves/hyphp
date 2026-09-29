import { useState } from 'react'
import { ArrowSquareOut, Envelope, Play } from '@phosphor-icons/react'
import { AppService, ServicesService } from '../../bindings/hyphp/services'
import { Button } from '../components/Button'
import { Card } from '../components/Card'
import { EmptyState } from '../components/EmptyState'
import { SectionLabel } from '../components/SectionLabel'
import { StatusDot, type ServiceState } from '../components/StatusDot'
import type { ScreenProps } from '../lib/screens'
import { useServices } from '../lib/useServices'
import { useSettings } from '../lib/useSettings'
import { useT } from '../i18n'

/** Porta padrão da UI do Mailpit; `State` recém-criado traz 0 no campo. */
const DEFAULT_HTTP_PORT = 8025

export function Mail({ onNavigate }: ScreenProps) {
  const t = useT('mail')
  const { services } = useServices()
  const { settings } = useSettings()
  const [error, setError] = useState<string | null>(null)
  const [starting, setStarting] = useState(false)

  const mailpit = services.find((s) => s.id === 'mailpit')
  // `ServiceStatus.state` é o enum gerado `supervisor.State` (nominal).
  const state = (mailpit?.state ?? 'stopped') as ServiceState
  // `||` e não `??`: o zero value do Go chega como 0 e viraria uma URL com porta 0.
  const url = `http://127.0.0.1:${settings?.mailpitHttpPort || DEFAULT_HTTP_PORT}`

  const start = async () => {
    setStarting(true)
    setError(null)
    try {
      await ServicesService.Start('mailpit')
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setStarting(false)
    }
  }

  if (!mailpit) {
    return (
      <EmptyState
        icon={<Envelope size={24} />}
        title={t('notInstalledTitle')}
        description={t('notInstalledDescription')}
        action={
          <Button variant="secondary" size="sm" onClick={() => onNavigate('runtimes')}>
            {t('goToRuntimes')}
          </Button>
        }
      />
    )
  }

  return (
    <div className="flex h-full min-h-0 flex-col gap-3">
      <div className="flex items-center gap-3">
        <StatusDot state={state} />
        <span className="text-sm text-fg">Mailpit</span>
        <span className="selectable font-mono text-xs text-fg-muted">{url}</span>
        <Button
          variant="ghost"
          size="sm"
          icon={<ArrowSquareOut size={14} />}
          className="ml-auto"
          onClick={() => void AppService.OpenExternal(url)}
        >
          {t('openInNewTab')}
        </Button>
      </div>
      {state === 'ready' ? (
        <iframe title="Mailpit" src={url} className="h-full w-full rounded-card border border-border bg-bg-card" />
      ) : (
        <Card>
          <SectionLabel>MAILPIT</SectionLabel>
          <div className="mt-3 flex items-center gap-3 text-sm">
            <StatusDot state={state} />
            <span className="text-fg">{state}</span>
            {mailpit.lastError && <span className="selectable text-err">{mailpit.lastError}</span>}
            {error && <span className="selectable text-err">{error}</span>}
            <Button
              variant="primary"
              size="sm"
              icon={<Play size={14} weight="fill" />}
              className="ml-auto"
              disabled={starting || state === 'starting' || state === 'stopping'}
              loading={starting}
              onClick={() => void start()}
            >
              {t('start')}
            </Button>
          </div>
        </Card>
      )}
    </div>
  )
}
