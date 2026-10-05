import { useEffect, useState } from 'react'
import { ArrowClockwise, DownloadSimple, FolderOpen, Star, Trash, X } from '@phosphor-icons/react'
import { AppService, RuntimesService } from '../../bindings/hyphp/services'
import { Badge } from '../components/Badge'
import { Button } from '../components/Button'
import { Card } from '../components/Card'
import { CopyButton } from '../components/CopyButton'
import { EmptyState } from '../components/EmptyState'
import { ProgressBar } from '../components/ProgressBar'
import { SectionLabel } from '../components/SectionLabel'
import { Skeleton } from '../components/Skeleton'
import { Toggle } from '../components/Toggle'
import type { ScreenProps } from '../lib/screens'
import type { Extension, Installed, Package, Progress, RuntimeKind } from '../lib/types'
import { useRuntimes } from '../lib/useRuntimes'
import { useSettings } from '../lib/useSettings'
import { errorText } from '../lib/errors'
import { compareVersionDesc } from '../lib/versions'
import { useT } from '../i18n'
import type { runtimes } from '../i18n/locales/pt-BR/runtimes'
import { PhpIniPanel } from './PhpIniPanel'

const TABS: Array<{ kind: RuntimeKind; label: string }> = [
  { kind: 'php', label: 'PHP' },
  { kind: 'apache', label: 'Apache' },
  { kind: 'nginx', label: 'nginx' },
  { kind: 'mysql', label: 'MySQL' },
  { kind: 'mariadb', label: 'MariaDB' },
  { kind: 'mailpit', label: 'Mailpit' },
  // mkcert estava no catálogo mas não tinha aba: não havia como instalá-lo
  // pela UI, e o aviso tls-unavailable apontava para uma ação inexistente.
  { kind: 'mkcert', label: 'mkcert' },
  // phpMyAdmin não é runtime nem serviço: é código PHP servido numa porta
  // dedicada. A aba existe porque instalar/remover é a única ação que ele tem.
  { kind: 'phpmyadmin', label: 'phpMyAdmin' },
]

/**
 * Espelha as constantes `Phase*` de internal/pkgmgr/manager.go. Guarda a chave
 * do catálogo: o texto só pode ser resolvido dentro do componente.
 */
const PHASE_LABELS: Record<string, keyof typeof runtimes> = {
  download: 'phaseDownload',
  verify: 'phaseVerify',
  extract: 'phaseExtract',
  done: 'phaseDone',
  error: 'phaseError',
  canceled: 'phaseCanceled',
}

const KB = 1024
const MB = 1024 * 1024

function formatBytes(n: number): string {
  if (n < MB) return `${Math.round(n / KB)} KB`
  return `${(n / MB).toFixed(1)} MB`
}

type InstalledCardProps = {
  inst: Installed
  isDefault: boolean
  /** Mac com Homebrew: muda o texto das extensões (não há DLLs) */
  brew: boolean
  onError: (msg: string) => void
}

function InstalledCard({ inst, isDefault, brew, onError }: InstalledCardProps) {
  const t = useT('runtimes')
  const tIni = useT('phpini')
  const [confirmRemove, setConfirmRemove] = useState(false)
  const [busy, setBusy] = useState(false)
  const [showExt, setShowExt] = useState(false)
  const [showIni, setShowIni] = useState(false)
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
            {isDefault && <Badge tone="accent">{t('default')}</Badge>}
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
              {t('setDefault')}
            </Button>
          )}
          {isPhp && (
            <Button
              variant={showExt ? 'secondary' : 'ghost'}
              size="sm"
              aria-pressed={showExt}
              onClick={() => setShowExt((v) => !v)}
            >
              {t('extensions')}
            </Button>
          )}
          {isPhp && (
            <Button
              variant={showIni ? 'secondary' : 'ghost'}
              size="sm"
              aria-pressed={showIni}
              onClick={() => setShowIni((v) => !v)}
            >
              {tIni('toggle')}
            </Button>
          )}
          {confirmRemove ? (
            <>
              {/* remover um keg roda `brew uninstall`, que some para qualquer
                  outra ferramenta do Mac que dependa da fórmula: avisa antes. */}
              <span className="text-xs text-fg-muted">
                {inst.formula ? t('removeConfirmBrew', { formula: inst.formula }) : t('removeConfirm')}
              </span>
              <Button
                variant="danger"
                size="sm"
                disabled={busy}
                loading={busy}
                onClick={() => void call(() => RuntimesService.Remove(inst.kind, inst.version))}
              >
                {t('remove')}
              </Button>
              <Button variant="ghost" size="sm" onClick={() => setConfirmRemove(false)}>
                {t('cancel')}
              </Button>
            </>
          ) : (
            <Button
              variant="ghost"
              size="sm"
              aria-label={t('removeAria', { kind: inst.kind, version: inst.version })}
              icon={<Trash size={14} />}
              disabled={busy}
              onClick={() => setConfirmRemove(true)}
            />
          )}
        </div>
      </div>

      {showExt && isPhp && (
        <div className="mt-4 border-t border-border pt-3">
          <SectionLabel>{t('extensionsLabel')}</SectionLabel>
          {extensions === null ? (
            <Skeleton lines={4} className="mt-2" />
          ) : extensions.length === 0 ? (
            <p className="mt-2 text-sm text-fg-faint">{brew ? t('noModules') : t('noDlls')}</p>
          ) : (
            <ul className="mt-2 grid grid-cols-3 gap-x-6 gap-y-1">
              {extensions.map((ext) => (
                <li key={ext.name} className="flex items-center justify-between py-1 text-sm">
                  <span className="flex items-center gap-1">
                    <span className="selectable font-mono text-fg">{ext.name}</span>
                    {ext.builtin && <Badge>{t('builtin')}</Badge>}
                  </span>
                  {/* embutida está compilada no binário: não há linha de ini a ligar/desligar */}
                  <Toggle
                    checked={ext.enabled}
                    disabled={busy || ext.builtin}
                    label={t('extensionAria', { name: ext.name })}
                    onChange={(on) => void toggleExt(ext, on)}
                  />
                </li>
              ))}
            </ul>
          )}
        </div>
      )}
      {showIni && isPhp && <PhpIniPanel major={inst.major} />}
    </Card>
  )
}

type AvailableCardProps = {
  pkg: Package
  progress: Progress | undefined
  /** Mac sem Homebrew: fórmulas não têm como ser instaladas */
  brewMissing: boolean
  onError: (msg: string) => void
}

function AvailableCard({ pkg, progress, brewMissing, onError }: AvailableCardProps) {
  const t = useT('runtimes')
  const [starting, setStarting] = useState(false)
  const [canceling, setCanceling] = useState(false)
  // Cancelado volta ao estado inicial: nem barra nem mensagem, só o botão Baixar.
  const finished = progress === undefined || ['done', 'error', 'canceled'].includes(progress.phase)
  const active = !finished
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

  // Só o download é cancelável: verificar e extrair levam segundos e não
  // olham o cancelamento.
  const cancel = async () => {
    setCanceling(true)
    try {
      await RuntimesService.CancelInstall(pkg.id)
    } catch (e) {
      onError(errorText(e))
    } finally {
      setCanceling(false)
    }
  }

  return (
    <Card>
      <div className="flex items-start gap-4">
        <div className="flex flex-col gap-1">
          {/* fórmula sem série (httpd, nginx...) tem versão vazia: usa o nome curto */}
          <span className="selectable font-mono text-lg text-fg">
            {pkg.version || (pkg.formula ?? '').split('/').pop()}
          </span>
          <div className="flex items-center gap-1">
            {pkg.compiler && <Badge mono>{pkg.compiler}</Badge>}
            {pkg.arch && <Badge mono>{pkg.arch}</Badge>}
          </div>
          {/* `notes` do catálogo é anotação de manutenção — procedência do
              SHA-256, layout do zip, aviso de 404 do php.net. Não é texto de
              usuário: quem vê a lista quer versão, compilador e arquitetura. */}
          {/* fórmula do Homebrew não tem URL: mostra o comando que será rodado */}
          {pkg.formula ? (
            <span className="selectable truncate font-mono text-xs text-fg-faint">
              {t('brewInstallCmd', { formula: pkg.formula })}
            </span>
          ) : (
            <span className="selectable truncate font-mono text-xs text-fg-faint" title={pkg.url}>
              {pkg.url}
            </span>
          )}
        </div>
        <div className="ml-auto flex items-center gap-2">
          {progress?.phase === 'download' && (
            <Button
              variant="ghost"
              size="sm"
              icon={<X size={14} />}
              loading={canceling}
              disabled={canceling}
              onClick={() => void cancel()}
            >
              {t('cancel')}
            </Button>
          )}
          <Button
            variant="primary"
            size="sm"
            icon={<DownloadSimple size={14} />}
            disabled={active || starting || (brewMissing && !!pkg.formula)}
            loading={starting}
            onClick={() => void install()}
          >
            {pkg.formula ? t('install') : t('download')}
          </Button>
        </div>
      </div>
      {progress && progress.phase !== 'done' && progress.phase !== 'canceled' && (
        <div className="mt-3">
          <ProgressBar
            value={determinate ? progress.done / progress.total : 0}
            indeterminate={!determinate}
            tone={progress.phase === 'error' ? 'err' : 'accent'}
            label={
              determinate
                ? `${t(PHASE_LABELS.download)} · ${formatBytes(progress.done)} / ${formatBytes(progress.total)}`
                : // sem total (brew, ou download sem Content-Length): fase + última linha de saída
                  [PHASE_LABELS[progress.phase] ? t(PHASE_LABELS[progress.phase]) : progress.phase, progress.message]
                    .filter(Boolean)
                    .join(' · ')
            }
          />
          {progress.phase === 'error' && <p className="selectable mt-1 text-xs text-err">{progress.error}</p>}
        </div>
      )}
    </Card>
  )
}

export function Runtimes({ onNavigate }: ScreenProps) {
  const t = useT('runtimes')
  const { installed, available, progress, loading, homebrew, rescan } = useRuntimes()
  const { settings } = useSettings()
  const [tab, setTab] = useState<RuntimeKind>('php')
  const [error, setError] = useState<string | null>(null)
  const [runtimeRoot, setRuntimeRoot] = useState('')
  const [importing, setImporting] = useState(false)
  const [checking, setChecking] = useState(false)
  const brewMissing = homebrew.supported && !homebrew.found

  useEffect(() => {
    void AppService.RuntimeRoot().then(setRuntimeRoot)
  }, [])

  const installedOfKind = installed
    .filter((i) => (i.kind as string) === tab)
    .sort((a, b) => compareVersionDesc(a.version, b.version))
  const installedVersions = new Set(installedOfKind.map((i) => i.version))
  // Ordenar também os disponíveis: eles vinham na ordem do catálogo, onde o
  // MySQL 8.4 aparece antes do 8.0, e a lista ficava fora de ordem só nesse kind.
  const availableOfKind = available
    .filter((p) => (p.kind as string) === tab && !installedVersions.has(p.version))
    .sort((a, b) => compareVersionDesc(a.version, b.version))
  const tabLabel = TABS.find((x) => x.kind === tab)?.label ?? tab

  return (
    <div className="flex flex-col gap-4">
      <div role="tablist" className="flex items-center gap-1 border-b border-border">
        {TABS.map((x) => (
          <button
            key={x.kind}
            role="tab"
            type="button"
            aria-selected={tab === x.kind}
            onClick={() => setTab(x.kind)}
            className={`-mb-px border-b-2 px-3 py-2 text-sm focus-visible:outline-2 focus-visible:outline-accent ${
              tab === x.kind ? 'border-accent text-accent-fg' : 'border-transparent text-fg-muted hover:text-fg'
            }`}
          >
            {x.label}
            <span className="ml-1 font-mono text-xs text-fg-faint">
              {installed.filter((i) => (i.kind as string) === x.kind).length}
            </span>
          </button>
        ))}
      </div>

      {error && (
        <div className="flex items-center justify-between rounded-card border border-err bg-bg-card px-3 py-2 text-sm text-err">
          <span className="selectable">{error}</span>
          <Button variant="ghost" size="sm" onClick={() => setError(null)}>
            {t('close')}
          </Button>
        </div>
      )}

      {brewMissing && (
        <Card>
          <div className="flex flex-col gap-2 text-sm">
            <span className="text-fg">{t('brewMissingTitle')}</span>
            <span className="text-fg-muted">{t('brewMissingBody')}</span>
            <div className="flex items-center gap-2">
              <code className="selectable flex-1 truncate rounded-card bg-bg-card-hover px-2 py-1 font-mono text-xs text-fg">
                {homebrew.installCommand}
              </code>
              <CopyButton text={homebrew.installCommand} label={t('brewCopyAria')} />
              <Button
                variant="secondary"
                size="sm"
                icon={<ArrowClockwise size={14} />}
                disabled={checking}
                loading={checking}
                onClick={() => {
                  setChecking(true)
                  void rescan()
                    .catch((e: unknown) => setError(errorText(e)))
                    .finally(() => setChecking(false))
                }}
              >
                {t('brewCheckAgain')}
              </Button>
            </div>
          </div>
        </Card>
      )}

      {tab === 'php' && (
        <div className="flex items-center justify-between gap-2 text-sm text-fg-muted">
          <span>{homebrew.supported ? t('phpHintBrew') : t('phpHint')}</span>
          {/* no Mac o PHP vem do Homebrew: não há bin/php nem pasta a importar */}
          {!homebrew.supported && (
            <div className="flex items-center gap-1">
              <Button
                variant="ghost"
                size="sm"
                icon={<FolderOpen size={14} />}
                disabled={!runtimeRoot}
                onClick={() =>
                  void AppService.OpenFolder(`${runtimeRoot}\\bin\\php`).catch((e: unknown) => setError(errorText(e)))
                }
              >
                {t('openBinPhp')}
              </Button>
              <Button
                variant="ghost"
                size="sm"
                icon={<FolderOpen size={14} />}
                disabled={importing}
                onClick={() => {
                  setImporting(true)
                  setError(null)
                  // PickImportDir devolve "" quando o usuário cancela: nesse caso
                  // não há nada a importar.
                  void RuntimesService.PickImportDir()
                    .then((dir) => (dir === '' ? undefined : RuntimesService.ImportFrom(dir)))
                    .catch((e: unknown) => setError(errorText(e)))
                    .finally(() => setImporting(false))
                }}
              >
                {importing ? t('importing') : t('importFrom')}
              </Button>
            </div>
          )}
        </div>
      )}

      <section className="flex flex-col gap-3">
        <SectionLabel>{t('installedLabel')}</SectionLabel>
        {loading && installed.length === 0 ? (
          <Skeleton lines={3} />
        ) : installedOfKind.length === 0 ? (
          <EmptyState
            title={t('noneInstalled', { name: tabLabel })}
            description={
              // phpMyAdmin não vira serviço: depois de instalado ele é servido
              // numa porta dedicada e se abre pela tela Banco. Mandar o usuário
              // a Serviços aqui seria apontar para uma tela onde nada aparece.
              tab === 'phpmyadmin'
                ? t('noneInstalledPma')
                : t('noneInstalledDescription')
            }
            action={
              <Button variant="secondary" onClick={() => onNavigate(tab === 'phpmyadmin' ? 'database' : 'services')}>
                {tab === 'phpmyadmin' ? t('goToDatabase') : t('viewServices')}
              </Button>
            }
          />
        ) : (
          installedOfKind.map((inst) => (
            <InstalledCard
              key={`${inst.kind}-${inst.dir}`}
              inst={inst}
              isDefault={(inst.kind as string) === 'php' && settings?.defaultPhp === inst.major}
              brew={homebrew.supported}
              onError={setError}
            />
          ))
        )}
      </section>

      <section className="flex flex-col gap-3">
        <SectionLabel>{t('availableLabel')}</SectionLabel>
        {availableOfKind.length === 0 ? (
          <p className="text-sm text-fg-faint">{t('noneAvailable')}</p>
        ) : (
          availableOfKind.map((pkg) => (
            <AvailableCard
              key={pkg.id}
              pkg={pkg}
              progress={progress[pkg.id]}
              brewMissing={brewMissing}
              onError={setError}
            />
          ))
        )}
      </section>
    </div>
  )
}
