import { useEffect, useState } from 'react'
import { DownloadSimple, FolderOpen, Star, Trash } from '@phosphor-icons/react'
import { AppService, RuntimesService } from '../../bindings/hyphp/services'
import { Badge } from '../components/Badge'
import { Button } from '../components/Button'
import { Card } from '../components/Card'
import { EmptyState } from '../components/EmptyState'
import { ProgressBar } from '../components/ProgressBar'
import { SectionLabel } from '../components/SectionLabel'
import { Skeleton } from '../components/Skeleton'
import { Toggle } from '../components/Toggle'
import type { ScreenProps } from '../lib/screens'
import type { Extension, Installed, Package, Progress, RuntimeKind } from '../lib/types'
import { useRuntimes } from '../lib/useRuntimes'
import { useSettings } from '../lib/useSettings'

const TABS: Array<{ kind: RuntimeKind; label: string }> = [
  { kind: 'php', label: 'PHP' },
  { kind: 'apache', label: 'Apache' },
  { kind: 'nginx', label: 'nginx' },
  { kind: 'mysql', label: 'MySQL' },
  { kind: 'mailpit', label: 'Mailpit' },
]

/** Espelha as constantes `Phase*` de internal/pkgmgr/manager.go. */
const PHASE_LABELS: Record<string, string> = {
  download: 'Baixando',
  verify: 'Verificando SHA-256',
  extract: 'Extraindo',
  done: 'Concluído',
  error: 'Erro',
}

const KB = 1024
const MB = 1024 * 1024

function errorText(e: unknown): string {
  return e instanceof Error ? e.message : String(e)
}

/**
 * Ordena da versão mais nova para a mais antiga comparando segmento a segmento:
 * a ordem lexicográfica colocaria "8.1.9" acima de "8.1.10".
 */
function compareVersionDesc(a: string, b: string): number {
  const left = a.split('.')
  const right = b.split('.')
  const len = Math.max(left.length, right.length)
  for (let i = 0; i < len; i++) {
    const na = Number.parseInt(left[i] || '0', 10) || 0
    const nb = Number.parseInt(right[i] || '0', 10) || 0
    if (na !== nb) return nb - na
  }
  return 0
}

function formatBytes(n: number): string {
  if (n < MB) return `${Math.round(n / KB)} KB`
  return `${(n / MB).toFixed(1)} MB`
}

type InstalledCardProps = {
  inst: Installed
  isDefault: boolean
  onError: (msg: string) => void
}

function InstalledCard({ inst, isDefault, onError }: InstalledCardProps) {
  const [confirmRemove, setConfirmRemove] = useState(false)
  const [busy, setBusy] = useState(false)
  const [showExt, setShowExt] = useState(false)
  const [extensions, setExtensions] = useState<Extension[] | null>(null)

  // `Installed.kind` é o enum gerado `runtime.Kind` (nominal no TS): alarga para
  // string antes de comparar com os literais de RuntimeKind.
  const isPhp = (inst.kind as string) === 'php'

  const call = async (fn: () => Promise<unknown>) => {
    setBusy(true)
    try {
      await fn()
    } catch (e) {
      onError(errorText(e))
    } finally {
      setBusy(false)
    }
  }

  useEffect(() => {
    if (!showExt || !isPhp) return
    // a resposta pode chegar depois de fechar o painel/desmontar o card.
    let cancelled = false
    RuntimesService.Extensions(inst.major)
      .then((list) => {
        if (!cancelled) setExtensions(list ?? [])
      })
      .catch((e: unknown) => onError(errorText(e)))
    return () => {
      cancelled = true
    }
  }, [showExt, isPhp, inst.major, onError])

  const toggleExt = (ext: Extension, on: boolean) =>
    call(async () => {
      await RuntimesService.SetExtension(inst.major, ext.name, on)
      // o Go não reemite a lista; atualiza local para o toggle não voltar sozinho.
      setExtensions((prev) => prev?.map((x) => (x.name === ext.name ? { ...x, enabled: on } : x)) ?? null)
    })

  return (
    <Card>
      <div className="flex items-start gap-4">
        <div className="flex flex-col gap-1">
          <div className="flex items-center gap-2">
            <span className="selectable font-mono text-2xl text-fg">{inst.version}</span>
            {isDefault && <Badge tone="accent">padrão</Badge>}
          </div>
          <div className="flex items-center gap-1">
            {inst.compiler && <Badge mono>{inst.compiler}</Badge>}
            {inst.arch && <Badge mono>{inst.arch}</Badge>}
            {inst.threadSafe !== null && <Badge mono>{inst.threadSafe ? 'TS' : 'NTS'}</Badge>}
          </div>
          <button
            type="button"
            onClick={() => void call(() => AppService.OpenFolder(inst.dir))}
            className="mt-1 inline-flex items-center gap-1 font-mono text-xs text-fg-muted hover:text-fg focus-visible:outline-2 focus-visible:outline-accent"
          >
            <FolderOpen size={12} /> {inst.dir}
          </button>
        </div>
        <div className="ml-auto flex items-center gap-1">
          {isPhp && !isDefault && (
            <Button
              variant="secondary"
              size="sm"
              icon={<Star size={14} />}
              disabled={busy}
              onClick={() => void call(() => RuntimesService.SetDefaultPHP(inst.major))}
            >
              Definir como padrão
            </Button>
          )}
          {isPhp && (
            <Button
              variant={showExt ? 'secondary' : 'ghost'}
              size="sm"
              aria-pressed={showExt}
              onClick={() => setShowExt((v) => !v)}
            >
              Extensões
            </Button>
          )}
          {confirmRemove ? (
            <>
              <span className="text-xs text-fg-muted">Remover a pasta?</span>
              <Button
                variant="danger"
                size="sm"
                disabled={busy}
                loading={busy}
                onClick={() => void call(() => RuntimesService.Remove(inst.kind, inst.version))}
              >
                Remover
              </Button>
              <Button variant="ghost" size="sm" onClick={() => setConfirmRemove(false)}>
                Cancelar
              </Button>
            </>
          ) : (
            <Button
              variant="ghost"
              size="sm"
              aria-label={`Remover ${inst.kind} ${inst.version}`}
              icon={<Trash size={14} />}
              disabled={busy}
              onClick={() => setConfirmRemove(true)}
            />
          )}
        </div>
      </div>

      {showExt && isPhp && (
        <div className="mt-4 border-t border-border pt-3">
          <SectionLabel>EXTENSÕES</SectionLabel>
          {extensions === null ? (
            <Skeleton lines={4} className="mt-2" />
          ) : extensions.length === 0 ? (
            <p className="mt-2 text-sm text-fg-faint">Nenhuma DLL em ext/.</p>
          ) : (
            <ul className="mt-2 grid grid-cols-3 gap-x-6 gap-y-1">
              {extensions.map((ext) => (
                <li key={ext.name} className="flex items-center justify-between py-1 text-sm">
                  <span className="selectable font-mono text-fg">{ext.name}</span>
                  <Toggle
                    checked={ext.enabled}
                    disabled={busy}
                    label={`Extensão ${ext.name}`}
                    onChange={(on) => void toggleExt(ext, on)}
                  />
                </li>
              ))}
            </ul>
          )}
        </div>
      )}
    </Card>
  )
}

type AvailableCardProps = {
  pkg: Package
  progress: Progress | undefined
  onError: (msg: string) => void
}

function AvailableCard({ pkg, progress, onError }: AvailableCardProps) {
  const [starting, setStarting] = useState(false)
  const active = progress !== undefined && progress.phase !== 'done' && progress.phase !== 'error'
  // só a fase de download tem tamanho conhecido — e nem sempre (total = -1 sem Content-Length).
  const determinate = progress !== undefined && progress.phase === 'download' && progress.total > 0

  const install = async () => {
    setStarting(true)
    try {
      await RuntimesService.Install(pkg.id)
    } catch (e) {
      onError(errorText(e))
    } finally {
      setStarting(false)
    }
  }

  return (
    <Card>
      <div className="flex items-start gap-4">
        <div className="flex flex-col gap-1">
          <span className="selectable font-mono text-lg text-fg">{pkg.version}</span>
          <div className="flex items-center gap-1">
            {pkg.compiler && <Badge mono>{pkg.compiler}</Badge>}
            {pkg.arch && <Badge mono>{pkg.arch}</Badge>}
          </div>
          {pkg.notes && <span className="text-xs text-fg-muted">{pkg.notes}</span>}
          <span className="selectable truncate font-mono text-xs text-fg-faint" title={pkg.url}>
            {pkg.url}
          </span>
        </div>
        <div className="ml-auto">
          <Button
            variant="primary"
            size="sm"
            icon={<DownloadSimple size={14} />}
            disabled={active || starting}
            loading={starting}
            onClick={() => void install()}
          >
            Baixar
          </Button>
        </div>
      </div>
      {progress && progress.phase !== 'done' && (
        <div className="mt-3">
          <ProgressBar
            value={determinate ? progress.done / progress.total : 0}
            indeterminate={!determinate}
            tone={progress.phase === 'error' ? 'err' : 'accent'}
            label={
              determinate
                ? `${PHASE_LABELS.download} · ${formatBytes(progress.done)} / ${formatBytes(progress.total)}`
                : PHASE_LABELS[progress.phase] ?? progress.phase
            }
          />
          {progress.phase === 'error' && <p className="selectable mt-1 text-xs text-err">{progress.error}</p>}
        </div>
      )}
    </Card>
  )
}

export function Runtimes({ onNavigate }: ScreenProps) {
  const { installed, available, progress, loading } = useRuntimes()
  const { settings } = useSettings()
  const [tab, setTab] = useState<RuntimeKind>('php')
  const [error, setError] = useState<string | null>(null)
  const [runtimeRoot, setRuntimeRoot] = useState('')

  useEffect(() => {
    void AppService.RuntimeRoot().then(setRuntimeRoot)
  }, [])

  const installedOfKind = installed
    .filter((i) => (i.kind as string) === tab)
    .sort((a, b) => compareVersionDesc(a.version, b.version))
  const installedVersions = new Set(installedOfKind.map((i) => i.version))
  const availableOfKind = available.filter((p) => (p.kind as string) === tab && !installedVersions.has(p.version))
  const tabLabel = TABS.find((t) => t.kind === tab)?.label ?? tab

  return (
    <div className="flex flex-col gap-4">
      <div role="tablist" className="flex items-center gap-1 border-b border-border">
        {TABS.map((t) => (
          <button
            key={t.kind}
            role="tab"
            type="button"
            aria-selected={tab === t.kind}
            onClick={() => setTab(t.kind)}
            className={`-mb-px border-b-2 px-3 py-2 text-sm focus-visible:outline-2 focus-visible:outline-accent ${
              tab === t.kind ? 'border-accent text-accent-fg' : 'border-transparent text-fg-muted hover:text-fg'
            }`}
          >
            {t.label}
            <span className="ml-1 font-mono text-xs text-fg-faint">
              {installed.filter((i) => (i.kind as string) === t.kind).length}
            </span>
          </button>
        ))}
      </div>

      {error && (
        <div className="flex items-center justify-between rounded-card border border-err bg-bg-card px-3 py-2 text-sm text-err">
          <span className="selectable">{error}</span>
          <Button variant="ghost" size="sm" onClick={() => setError(null)}>
            fechar
          </Button>
        </div>
      )}

      {tab === 'php' && (
        <div className="flex items-center justify-between text-sm text-fg-muted">
          <span>Coloque qualquer build do php.net em bin/php e ela aparece aqui.</span>
          <Button
            variant="ghost"
            size="sm"
            icon={<FolderOpen size={14} />}
            disabled={!runtimeRoot}
            onClick={() =>
              void AppService.OpenFolder(`${runtimeRoot}\\bin\\php`).catch((e: unknown) => setError(errorText(e)))
            }
          >
            Abrir pasta bin/php
          </Button>
        </div>
      )}

      <section className="flex flex-col gap-3">
        <SectionLabel>INSTALADOS</SectionLabel>
        {loading && installed.length === 0 ? (
          <Skeleton lines={3} />
        ) : installedOfKind.length === 0 ? (
          <EmptyState
            title={`Nenhum ${tabLabel} instalado`}
            description="Baixe uma versão do catálogo abaixo; o serviço correspondente só aparece em Serviços depois da instalação."
            action={
              <Button variant="secondary" onClick={() => onNavigate('services')}>
                Ver serviços
              </Button>
            }
          />
        ) : (
          installedOfKind.map((inst) => (
            <InstalledCard
              key={`${inst.kind}-${inst.dir}`}
              inst={inst}
              isDefault={(inst.kind as string) === 'php' && settings?.defaultPhp === inst.major}
              onError={setError}
            />
          ))
        )}
      </section>

      <section className="flex flex-col gap-3">
        <SectionLabel>DISPONÍVEIS</SectionLabel>
        {availableOfKind.length === 0 ? (
          <p className="text-sm text-fg-faint">Nenhum pacote do catálogo pendente para este tipo.</p>
        ) : (
          availableOfKind.map((pkg) => (
            <AvailableCard key={pkg.id} pkg={pkg} progress={progress[pkg.id]} onError={setError} />
          ))
        )}
      </section>
    </div>
  )
}
