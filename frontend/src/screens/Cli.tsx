import { useEffect, useState } from 'react'
import { TerminalWindow } from '@phosphor-icons/react'
import { CLIService } from '../../bindings/hyphp/services'
import type { CLIActivity, CLICommand, CLIInfo } from '../../bindings/hyphp/services/models'
import { Badge } from '../components/Badge'
import { Button } from '../components/Button'
import { Card } from '../components/Card'
import { CopyButton } from '../components/CopyButton'
import { EmptyState } from '../components/EmptyState'
import { SectionLabel } from '../components/SectionLabel'
import { Skeleton } from '../components/Skeleton'
import { StatusDot } from '../components/StatusDot'
import { errorText } from '../lib/errors'
import { EVENTS, useEvent } from '../lib/events'
import type { ScreenProps } from '../lib/screens'
import { useLang, useT } from '../i18n'

/** O Go guarda as últimas 100 chamadas; a tela mantém o mesmo teto ao vivo. */
const ACTIVITY_MAX = 100

export function Cli(_: ScreenProps) {
  const t = useT('cli')
  const { lang } = useLang()
  const [info, setInfo] = useState<CLIInfo | null>(null)
  const [commands, setCommands] = useState<CLICommand[] | null>(null)
  const [activity, setActivity] = useState<CLIActivity[] | null>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    void CLIService.Info().then(setInfo)
    void CLIService.Activity().then((a) => setActivity(a ?? []))
  }, [])

  // As descrições vêm do Go no idioma atual: trocar o idioma pede de novo.
  useEffect(() => {
    void CLIService.Commands().then((c) => setCommands(c ?? []))
  }, [lang])

  useEvent<CLIActivity>(EVENTS.cliActivity, (a) => {
    setActivity((prev) => [a, ...(prev ?? [])].slice(0, ACTIVITY_MAX))
  })

  const addToPath = async () => {
    setBusy(true)
    setError(null)
    try {
      await CLIService.AddToPath()
      setInfo(await CLIService.Info())
    } catch (e) {
      setError(errorText(e))
    } finally {
      setBusy(false)
    }
  }

  const time = new Intl.DateTimeFormat(lang, { hour: '2-digit', minute: '2-digit', second: '2-digit' })

  return (
    <div className="flex flex-col gap-4">
      <Card>
        <SectionLabel>{t('label')}</SectionLabel>
        <p className="mt-2 text-sm text-fg-muted">{t('intro')}</p>
        {info === null ? (
          <Skeleton lines={3} className="mt-3" />
        ) : (
          <dl className="mt-3 grid grid-cols-[120px_1fr_auto] items-center gap-y-1 text-sm">
            <dt className="text-fg-muted">{t('address')}</dt>
            <dd className="flex min-w-0 items-center gap-2">
              <StatusDot state={info.listening ? 'ready' : 'failed'} />
              <span className="text-fg">{info.listening ? t('listening') : t('notListening')}</span>
              <span className="selectable truncate font-mono text-xs text-fg-muted" title={info.address}>
                {info.address}
              </span>
            </dd>
            <dd>
              <CopyButton text={info.address} label={t('copyAddress')} />
            </dd>
            <dt className="text-fg-muted">{t('executable')}</dt>
            <dd className="selectable truncate font-mono text-fg" title={info.exe}>
              {info.exe}
            </dd>
            <dd>
              <CopyButton text={info.exe} label={t('copyExecutable')} />
            </dd>
            <dt className="text-fg-muted">{t('path')}</dt>
            <dd className="flex items-center gap-3">
              {info.onPath ? (
                <Badge tone="ok">{t('onPath')}</Badge>
              ) : (
                <>
                  <Button
                    variant="secondary"
                    size="sm"
                    loading={busy}
                    disabled={busy || !info.exeExists}
                    onClick={() => void addToPath()}
                  >
                    {t('addToPath')}
                  </Button>
                  <span className="text-xs text-fg-muted">{t('addToPathHint')}</span>
                </>
              )}
            </dd>
            <dd />
          </dl>
        )}
        {info?.error && <p className="selectable mt-2 text-xs text-err">{info.error}</p>}
        {info && !info.exeExists && <p className="mt-2 text-xs text-warn">{t('exeMissing', { exe: info.exe })}</p>}
        {error && <p className="selectable mt-2 text-xs text-err">{error}</p>}
        <p className="mt-3 text-xs text-fg-muted">{t('aiHint')}</p>
      </Card>

      <Card>
        <div className="flex items-baseline justify-between gap-3">
          <SectionLabel>{t('activity')}</SectionLabel>
          <span className="text-xs text-fg-faint">{t('activityHint')}</span>
        </div>
        {activity === null ? (
          <Skeleton lines={3} className="mt-3" />
        ) : activity.length === 0 ? (
          <EmptyState
            icon={<TerminalWindow size={24} />}
            title={t('noActivityTitle')}
            description={t('noActivityDescription')}
          />
        ) : (
          <ul className="mt-3 divide-y divide-border">
            {activity.map((a, i) => (
              <li key={`${a.at}-${i}`} className="flex items-center gap-3 py-2 text-sm">
                <span className="shrink-0 font-mono text-xs text-fg-faint">{time.format(new Date(a.at))}</span>
                <span className="selectable min-w-0 truncate font-mono text-fg" title={a.error || undefined}>
                  hyphp {a.command}
                </span>
                {a.caller && (
                  <span className="shrink-0 truncate text-xs text-fg-muted" title={a.caller}>
                    {a.caller}
                  </span>
                )}
                <span className="ml-auto flex shrink-0 items-center gap-2">
                  <span className="font-mono text-xs text-fg-faint">{a.durationMs} ms</span>
                  <Badge tone={a.ok ? 'ok' : 'err'}>{a.ok ? t('ok') : t('failed')}</Badge>
                </span>
              </li>
            ))}
          </ul>
        )}
      </Card>

      <Card>
        <SectionLabel>{t('commands')}</SectionLabel>
        {commands === null ? (
          <Skeleton lines={6} className="mt-3" />
        ) : (
          <table className="mt-3 w-full text-sm">
            <tbody className="divide-y divide-border">
              {commands.map((c) => (
                <tr key={c.usage}>
                  <td className="selectable whitespace-nowrap py-1.5 pr-4 font-mono text-fg">{c.usage}</td>
                  <td className="py-1.5 text-fg-muted">{c.description}</td>
                  <td className="py-1.5 text-right">
                    <CopyButton text={c.usage} label={t('copyCommand', { usage: c.usage })} />
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </Card>
    </div>
  )
}
