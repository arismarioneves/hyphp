import { useMemo, useState } from 'react'
import {
  ArrowSquareOut,
  Code,
  FilePlus,
  FolderOpen,
  FolderPlus,
  Scroll,
  Terminal,
  Warning as WarningIcon,
} from '@phosphor-icons/react'
import { AppService, ProjectsService } from '../../bindings/hyphp/services'
import { Badge } from '../components/Badge'
import { Button } from '../components/Button'
import { Card } from '../components/Card'
import { EmptyState } from '../components/EmptyState'
import { LogView } from '../components/LogView'
import { SectionLabel } from '../components/SectionLabel'
import { Select, type SelectOption } from '../components/Select'
import { Skeleton } from '../components/Skeleton'
import { SplitPane, SplitPaneItem } from '../components/SplitPane'
import { StatusDot, type ServiceState } from '../components/StatusDot'
import { Toggle } from '../components/Toggle'
import type { ScreenProps } from '../lib/screens'
import { aggregateState } from '../lib/status'
import { useProjects } from '../lib/useProjects'
import { useRuntimes } from '../lib/useRuntimes'
import { useServices } from '../lib/useServices'
import { useSettings } from '../lib/useSettings'
import { useT } from '../i18n'

/** Valor sentinela do Select: não é uma versão, leva para a tela Runtimes. */
const DOWNLOAD_OPTION = '__download__'
/** Opção inerte só para separar as versões instaladas da ação de download. */
const SEPARATOR_OPTION = '__separator__'

export function Projects({ onNavigate }: ScreenProps) {
  const t = useT('projects')
  const { projects, loading } = useProjects()
  const { services } = useServices()
  const { installed } = useRuntimes()
  const { settings } = useSettings()
  const [selectedId, setSelectedId] = useState<string | null>(null)
  const [openLog, setOpenLog] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  // a seleção segue o id; se o projeto sumiu num rescan, cai no primeiro.
  const selected = projects.find((p) => p.id === selectedId) ?? projects[0] ?? null

  // processos declarados em `processes:` viram specs `proc:<projeto>:<nome>`.
  const processes = selected ? services.filter((s) => s.id.startsWith(`proc:${selected.id}:`)) : []

  // `Installed.kind` é o enum gerado `runtime.Kind`: enum de string do TS é
  // nominal, comparar com o literal exige alargar para string.
  const phpMajors = useMemo(() => {
    const seen: Record<string, true> = {}
    for (const i of installed) if ((i.kind as string) === 'php') seen[i.major] = true
    return Object.keys(seen).sort()
  }, [installed])

  const phpOptions: SelectOption[] = [
    ...phpMajors.map((m) => ({ value: m, label: `PHP ${m}${settings?.defaultPhp === m ? t('defaultSuffix') : ''}` })),
    { value: SEPARATOR_OPTION, label: '────────', disabled: true },
    { value: DOWNLOAD_OPTION, label: t('downloadOther') },
  ]

  const call = async (fn: () => Promise<unknown>) => {
    setBusy(true)
    setError(null)
    try {
      await fn()
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setBusy(false)
    }
  }

  // PickRoot devolve "" quando o usuário cancela o diálogo nativo.
  const addRoot = () =>
    call(async () => {
      const dir = await ProjectsService.PickRoot()
      if (dir) await ProjectsService.AddRoot(dir)
    })

  if (loading && projects.length === 0) {
    return <Skeleton lines={6} />
  }

  if (projects.length === 0) {
    return (
      <EmptyState
        icon={<FolderOpen size={32} />}
        title={t('noProjectsTitle')}
        description={t('noProjectsDesc')}
        action={
          <Button variant="primary" icon={<FolderPlus size={16} />} onClick={() => void addRoot()} disabled={busy}>
            {t('addDirectory')}
          </Button>
        }
      />
    )
  }

  const list = projects.map((p) => (
    <SplitPaneItem
      key={p.id}
      selected={selected?.id === p.id}
      onClick={() => {
        setSelectedId(p.id)
        setOpenLog(null)
      }}
    >
      <StatusDot
        state={aggregateState(
          services.filter((s) => s.id.startsWith(`proc:${p.id}:`)).map((s) => s.state as ServiceState),
        )}
      />
      <div className="flex min-w-0 flex-1 flex-col">
        <span className="truncate">{p.name}</span>
        <span className="truncate font-mono text-xs text-fg-muted">{p.domain}</span>
      </div>
      <Badge mono>{p.phpEffective || p.php || '—'}</Badge>
    </SplitPaneItem>
  ))

  const footer = (
    <Button
      variant="secondary"
      size="sm"
      icon={<FolderPlus size={14} />}
      onClick={() => void addRoot()}
      disabled={busy}
      className="w-full"
    >
      {t('addDirectory')}
    </Button>
  )

  // `State.webServer` é o enum gerado `state.WebServerName` (nominal).
  const isNginx = (settings?.webServer as string) === 'nginx'

  return (
    <SplitPane list={list} footer={footer}>
      {selected && (
        <div className="flex flex-col gap-4">
          {error && (
            <div className="selectable rounded-card border border-err bg-bg-card px-3 py-2 text-sm text-err">
              {error}
            </div>
          )}

          <Card
            title={selected.name}
            actions={
              <div className="flex items-center gap-1">
                <Button
                  variant="ghost"
                  size="sm"
                  aria-label={t('openBrowser')}
                  icon={<ArrowSquareOut size={16} />}
                  onClick={() => void call(() => AppService.OpenExternal(`https://${selected.domain}`))}
                />
                <Button
                  variant="ghost"
                  size="sm"
                  aria-label={t('openFolder')}
                  icon={<FolderOpen size={16} />}
                  onClick={() => void call(() => AppService.OpenFolder(selected.root))}
                />
                <Button
                  variant="ghost"
                  size="sm"
                  aria-label={t('openEditor')}
                  icon={<Code size={16} />}
                  onClick={() => void call(() => AppService.OpenInEditor(selected.root))}
                />
                <Button
                  variant="ghost"
                  size="sm"
                  aria-label={t('openTerminal')}
                  icon={<Terminal size={16} />}
                  onClick={() => void call(() => AppService.OpenTerminal(selected.root))}
                />
              </div>
            }
          >
            <dl className="grid grid-cols-[140px_1fr] gap-y-2 text-sm">
              <dt className="text-fg-muted">{t('domain')}</dt>
              <dd className="selectable font-mono text-fg">{selected.domain}</dd>
              <dt className="text-fg-muted">{t('path')}</dt>
              <dd className="selectable break-all font-mono text-fg">{selected.root}</dd>
              <dt className="text-fg-muted">{t('docroot')}</dt>
              <dd className="selectable break-all font-mono text-fg">{selected.docrootAbs}</dd>
              <dt className="text-fg-muted">{t('webServer')}</dt>
              <dd className="flex items-center gap-2 text-fg">
                <span>{isNginx ? 'nginx' : 'Apache'}</span>
                {selected.hasHtaccess && isNginx && (
                  <span className="inline-flex items-center gap-1 text-xs text-warn">
                    <WarningIcon size={14} /> {t('htaccessNginx')}
                  </span>
                )}
              </dd>
              <dt className="text-fg-muted">{t('manifest')}</dt>
              <dd>
                {selected.hasManifest ? (
                  <Badge tone="ok" mono>
                    hyphp.yaml
                  </Badge>
                ) : (
                  <Button
                    variant="secondary"
                    size="sm"
                    icon={<FilePlus size={14} />}
                    disabled={busy}
                    onClick={() => void call(() => ProjectsService.CreateManifest(selected.id))}
                  >
                    {t('createManifest')}
                  </Button>
                )}
              </dd>
            </dl>
          </Card>

          <Card>
            <SectionLabel>PHP</SectionLabel>
            <div className="mt-3 flex items-center gap-4">
              <Select
                aria-label={t('phpVersionAria')}
                value={selected.phpEffective || selected.php || ''}
                options={phpOptions}
                disabled={busy}
                onChange={(v) => {
                  if (v === DOWNLOAD_OPTION) {
                    onNavigate('runtimes')
                    return
                  }
                  void call(() => ProjectsService.SetPHP(selected.id, v))
                }}
                className="w-56"
              />
              {selected.php && !phpMajors.includes(selected.php) && (
                <span className="inline-flex items-center gap-1 text-sm text-warn">
                  <WarningIcon size={14} /> {t('phpNotInstalled', { version: selected.php })}
                  <Button variant="ghost" size="sm" onClick={() => onNavigate('runtimes')}>
                    {t('download')}
                  </Button>
                </span>
              )}
            </div>
            {selected.extensions && selected.extensions.length > 0 && (
              <div className="mt-3 flex flex-wrap gap-1">
                {selected.extensions.map((ext) => (
                  <Badge key={ext} mono>
                    {ext}
                  </Badge>
                ))}
              </div>
            )}
          </Card>

          <Card>
            <SectionLabel>{t('network')}</SectionLabel>
            <div className="mt-3 flex items-center justify-between text-sm">
              <div className="flex flex-col">
                <span className="text-fg">Wildcard DNS (*.{selected.domain})</span>
                <span className="text-xs text-fg-muted">
                  {t('wildcardDesc')}
                </span>
              </div>
              <Toggle
                checked={Boolean(selected.wildcard)}
                disabled={busy}
                label="Wildcard DNS"
                onChange={(on) => void call(() => ProjectsService.SetWildcard(selected.id, on))}
              />
            </div>
          </Card>

          <Card>
            <SectionLabel>{t('processes')}</SectionLabel>
            {processes.length === 0 ? (
              <p className="mt-3 text-sm text-fg-faint">{t('noProcesses')}</p>
            ) : (
              <ul className="mt-3 divide-y divide-border">
                {processes.map((s) => (
                  <li key={s.id} className="flex flex-col gap-2 py-2">
                    <div className="flex items-center gap-3">
                      <StatusDot state={s.state as ServiceState} />
                      <span className="font-mono text-sm text-fg">{s.id.slice(`proc:${selected.id}:`.length)}</span>
                      <span className="text-xs text-fg-faint">
                        {s.state}
                        {s.pid ? ` · PID ${s.pid}` : ''}
                        {s.restarts ? t('restarts', { count: s.restarts }) : ''}
                      </span>
                      <Button
                        variant={openLog === s.id ? 'secondary' : 'ghost'}
                        size="sm"
                        icon={<Scroll size={14} />}
                        aria-pressed={openLog === s.id}
                        onClick={() => setOpenLog(openLog === s.id ? null : s.id)}
                        className="ml-auto"
                      >
                        log
                      </Button>
                    </div>
                    {s.lastError && <div className="selectable text-xs text-err">{s.lastError}</div>}
                    {openLog === s.id && <LogView id={s.id} className="h-64" />}
                  </li>
                ))}
              </ul>
            )}
          </Card>
        </div>
      )}
    </SplitPane>
  )
}
