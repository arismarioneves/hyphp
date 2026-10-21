import { useEffect, useState, type ReactNode } from 'react'
import { ArrowSquareOut, Copy, FolderOpen } from '@phosphor-icons/react'
import { AppService, SettingsService } from '../../bindings/hyphp/services'
import { Button } from '../components/Button'
import { Logo } from '../components/Logo'
import { ProgressBar } from '../components/ProgressBar'
import { Row, Section } from '../components/Section'
import { Skeleton } from '../components/Skeleton'
import { Toggle } from '../components/Toggle'
import { errorText } from '../lib/errors'
import type { AboutInfo, UpdateStatus } from '../lib/types'
import { useRuntimes } from '../lib/useRuntimes'
import { useSettings } from '../lib/useSettings'
import { useUpdate } from '../lib/useUpdate'
import { activeDbEngine, compareVersionDesc, newestVersions } from '../lib/versions'
import { useLang, useT } from '../i18n'
import type { Messages } from '../i18n/types'
import type { Vars } from '../i18n/format'

// Tradutor de `about` recebido por parâmetro: updateLine fica fora do componente.
type AboutT = (key: keyof Messages['about'], vars?: Vars) => string

// Nomes de produto dos valores de state.WebServer e state.DBEngine.
const PRODUCT_NAMES: Record<string, string> = {
  apache: 'Apache',
  nginx: 'nginx',
  mysql: 'MySQL',
  mariadb: 'MariaDB',
}

// Texto de estado do auto-update; os estados vêm de internal/update.State.
function updateLine(s: UpdateStatus, t: AboutT, lang: string) {
  switch (s.state as string) {
    case 'inativo':
      return t('updateInactive')
    case 'ocioso':
      return t('updateIdle')
    case 'verificando':
      return t('updateChecking')
    case 'em-dia': {
      const at = s.checkedAt ? new Date(s.checkedAt).toLocaleTimeString(lang, { hour: '2-digit', minute: '2-digit' }) : ''
      return at ? t('updateUpToDateAt', { at }) : t('updateUpToDate')
    }
    case 'baixando':
      return t('updateDownloading', { version: s.available })
    case 'pronto':
      return t('updateReady', { version: s.available })
    case 'aplicando':
      return t('updateApplying', { version: s.available })
    default:
      return t('updateFailed', { error: s.error })
  }
}

function UpdateRow({ version }: { version: string }) {
  const t = useT('about')
  const { lang } = useLang()
  const { status, busy, error, check, apply } = useUpdate()
  const state = status?.state as string | undefined
  return (
    <div className="flex flex-col gap-2">
      <div className="flex items-center justify-between gap-4 text-sm">
        <div className="flex flex-col">
          <span className="text-fg">
            {t('version')} <span className="selectable font-mono">{version}</span>
          </span>
          {status && (
            <span className={`selectable text-xs ${state === 'falhou' ? 'text-err' : 'text-fg-muted'}`}>
              {updateLine(status, t, lang)}
            </span>
          )}
        </div>
        {!status ? (
          <Skeleton className="h-7 w-40" />
        ) : state === 'pronto' ? (
          <Button variant="primary" size="sm" loading={busy} onClick={() => void apply()}>
            {t('updateAndRestart')}
          </Button>
        ) : (
          <Button
            variant="secondary"
            size="sm"
            loading={busy || state === 'verificando' || state === 'baixando'}
            disabled={state === 'inativo' || state === 'aplicando'}
            onClick={() => void check()}
          >
            {t('checkNow')}
          </Button>
        )}
      </div>
      {status && state === 'baixando' && status.total > 0 && (
        <ProgressBar value={status.done / status.total} label={`${Math.round((status.done / status.total) * 100)}%`} />
      )}
      {error && <span className="selectable text-sm text-err">{error}</span>}
    </div>
  )
}

/** Link que abre no navegador padrão; a falha sobe para a tela mostrar. */
function ExternalLink({ href, onError, children }: { href: string; onError: (msg: string) => void; children: ReactNode }) {
  return (
    <button
      type="button"
      onClick={() => void AppService.OpenExternal(href).catch((e: unknown) => onError(errorText(e)))}
      className="inline-flex items-center gap-1 font-mono text-sm text-accent-fg hover:underline focus-visible:outline-2 focus-visible:outline-accent"
    >
      {children}
      <ArrowSquareOut size={12} />
    </button>
  )
}

export function About() {
  const t = useT('about')
  const { settings } = useSettings()
  const { installed, loading } = useRuntimes()
  const [about, setAbout] = useState<AboutInfo | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [copied, setCopied] = useState(false)

  useEffect(() => {
    void AppService.About()
      .then(setAbout)
      .catch((e: unknown) => setError(errorText(e)))
  }, [])

  if (!about || !settings) return error ? <p className="selectable text-sm text-err">{error}</p> : <Skeleton lines={8} />

  // O diagnóstico descreve o que está no ar: o web server e o banco ativos,
  // na versão que a stack sobe (a maior instalada), e todo PHP instalado.
  const versions = newestVersions(installed)
  const web = settings.webServer as string
  const db = activeDbEngine(settings.dbEngine, versions)
  const php = installed
    .filter((i) => (i.kind as string) === 'php')
    .map((i) => i.version)
    .sort(compareVersionDesc)
  const diagnostics = [
    `HyPHP ${about.version}`,
    `${t('system')}: ${about.os} · ${about.arch}`,
    `${t('diagWebServer')}: ${versions[web] ? `${PRODUCT_NAMES[web]} ${versions[web]}` : t('diagNone')}`,
    `${t('diagDatabase')}: ${versions[db] ? `${PRODUCT_NAMES[db]} ${versions[db]}` : t('diagNone')}`,
    `PHP: ${php.length > 0 ? php.join(', ') : t('diagNone')}`,
    `${t('dataRoot')}: ${about.root}`,
  ].join('\n')

  const contributors = about.contributors ?? []

  const copyDiagnostics = () => {
    setError(null)
    navigator.clipboard
      .writeText(diagnostics)
      .then(() => {
        setCopied(true)
        setTimeout(() => setCopied(false), 1200)
      })
      .catch((e: unknown) => setError(errorText(e)))
  }

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-col items-center gap-2 py-4 text-center">
        <Logo size={56} className="text-accent" />
        <h2 className="mt-2 font-mono text-xl font-semibold text-fg">HyPHP</h2>
        <p className="text-sm text-fg-muted">{t('tagline')}</p>
      </div>

      {error && <p className="selectable text-sm text-err">{error}</p>}

      <Section>
        <UpdateRow version={about.version} />
        <Row label={t('autoUpdate')} hint={t('autoUpdateHint')}>
          <Toggle
            checked={!settings.autoUpdateOff}
            label={t('autoUpdate')}
            onChange={(on) => {
              setError(null)
              void SettingsService.SetAutoUpdate(on).catch((e: unknown) => setError(errorText(e)))
            }}
          />
        </Row>
        <Row label={t('releaseNotes')}>
          <ExternalLink href={about.releaseNotes} onError={setError}>
            v{about.version}
          </ExternalLink>
        </Row>
      </Section>

      <Section>
        <Row label={t('system')} hint={t('copyDiagnosticsHint')}>
          <div className="flex items-center gap-3">
            <span className="selectable font-mono text-sm text-fg">
              {about.os} · {about.arch}
            </span>
            {/* Antes da varredura dos runtimes o diagnóstico sairia sem web server, banco e PHP. */}
            <Button variant="secondary" size="sm" icon={<Copy size={14} />} disabled={loading} onClick={copyDiagnostics}>
              {copied ? t('copied') : t('copyDiagnostics')}
            </Button>
          </div>
        </Row>
        <Row label={t('dataRoot')}>
          <div className="flex items-center gap-3">
            <span className="selectable font-mono text-sm text-fg">{about.root}</span>
            <Button
              variant="secondary"
              size="sm"
              icon={<FolderOpen size={14} />}
              onClick={() => void AppService.OpenFolder(about.root).catch((e: unknown) => setError(errorText(e)))}
            >
              {t('openFolder')}
            </Button>
          </div>
        </Row>
      </Section>

      <Section>
        <Row label={t('links')}>
          <div className="flex flex-wrap items-center justify-end gap-4">
            <ExternalLink href={about.site} onError={setError}>
              {t('site')}
            </ExternalLink>
            <ExternalLink href={about.repository} onError={setError}>
              {t('repository')}
            </ExternalLink>
            <ExternalLink href={about.newIssue} onError={setError}>
              {t('reportIssue')}
            </ExternalLink>
          </div>
        </Row>
        <Row label={t('license')}>
          <span className="selectable font-mono text-sm text-fg">
            {about.license} · {about.copyright}
          </span>
        </Row>
        <Row label={t('madeBy')}>
          <div className="flex items-center gap-3">
            <span className="text-sm text-fg">{about.author}</span>
            <ExternalLink href={`https://github.com/${about.authorLogin}`} onError={setError}>
              @{about.authorLogin}
            </ExternalLink>
          </div>
        </Row>
        {/* A lista já traz o autor, que está em "Feito por": sozinho, ele
            apareceria duas vezes. */}
        {contributors.length > 1 && (
          <div className="flex flex-col gap-2 text-sm">
            <span className="text-fg">{t('contributors', { count: contributors.length })}</span>
            <div className="flex flex-wrap gap-x-4 gap-y-1">
              {contributors.map((login) => (
                <ExternalLink key={login} href={`https://github.com/${login}`} onError={setError}>
                  @{login}
                </ExternalLink>
              ))}
            </div>
          </div>
        )}
        <Row label={t('support')}>
          <ExternalLink href={about.support} onError={setError}>
            {about.support.replace(/^https:\/\//, '')}
          </ExternalLink>
        </Row>
      </Section>
    </div>
  )
}
