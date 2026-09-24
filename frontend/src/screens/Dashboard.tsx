import { useState } from 'react'
import { ArrowSquareOut, Play, Stop, Warning as WarningIcon } from '@phosphor-icons/react'
import { AppService, ServicesService, SettingsService } from '../../bindings/hyphp/services'
import { Badge } from '../components/Badge'
import { Button } from '../components/Button'
import { Card } from '../components/Card'
import { EmptyState } from '../components/EmptyState'
import { SectionLabel } from '../components/SectionLabel'
import { Skeleton } from '../components/Skeleton'
import { StatusDot, type ServiceState } from '../components/StatusDot'
import type { Screen, ScreenProps } from '../lib/screens'
import { phpPools } from '../lib/status'
import type { Installed, RuntimeKind, ServiceStatus } from '../lib/types'
import { useProjects } from '../lib/useProjects'
import { useRuntimes } from '../lib/useRuntimes'
import { useServices } from '../lib/useServices'
import { useSettings } from '../lib/useSettings'
import { useWarnings } from '../lib/useWarnings'

type StackRow = { key: string; state: ServiceState; name: string; version: string; extra?: string }

// Como resolver cada pendência. As que pedem UAC executam aqui mesmo: mandar o
// usuário para outra tela para clicar num botão equivalente é um salto sem
// motivo. As que dependem de download levam a Runtimes, onde a escolha existe.
const ONDE_RESOLVER: Record<string, { tela?: Screen; run?: () => Promise<void>; acao: string }> = {
  'web-missing': { tela: 'runtimes', acao: 'Instalar web server' },
  'tls-unavailable': { tela: 'runtimes', acao: 'Instalar mkcert' },
  'php-missing': { tela: 'runtimes', acao: 'Instalar PHP' },
  'hosts-pending': { run: SettingsService.ApplyHosts, acao: 'Aplicar domínios' },
  'ca-pending': { run: SettingsService.InstallCA, acao: 'Instalar certificado' },
  'wildcard-pending': { run: SettingsService.ApplyWildcardDNS, acao: 'Registrar regra de DNS' },
}

// `Installed.kind` é o enum gerado `runtime.Kind`; comparar com os literais de
// RuntimeKind exige alargar para string (enum de string do TS é nominal).
function phpVersionFor(installed: Installed[], major: string): string {
  const builds = installed.filter((i) => (i.kind as string) === 'php' && i.major === major)
  if (builds.length === 0) return major
  // maior versão da série vence: é a que o supervisor sobe para o pool.
  const sorted = builds.map((b) => b.version).sort()
  return sorted[sorted.length - 1] ?? major
}

function stackRows(services: ServiceStatus[], installed: Installed[]): StackRow[] {
  const versions: Record<string, string> = {}
  for (const i of installed) versions[i.kind as string] ??= i.version

  const rows: StackRow[] = []
  const web = services.find((s) => s.group === 'web')
  if (web) {
    const kind: RuntimeKind = web.id === 'web:nginx' ? 'nginx' : 'apache'
    rows.push({
      key: web.id,
      state: web.state as ServiceState,
      name: kind === 'nginx' ? 'nginx' : 'Apache',
      version: versions[kind] ?? '—',
    })
  }
  for (const pool of phpPools(services)) {
    rows.push({
      key: `php:${pool.major}`,
      state: pool.state,
      name: `PHP ${pool.major}`,
      version: phpVersionFor(installed, pool.major),
      extra: `${pool.ready}/${pool.total} workers`,
    })
  }
  const db = services.find((s) => s.id === 'mysql')
  if (db) rows.push({ key: db.id, state: db.state as ServiceState, name: 'MySQL', version: versions.mysql ?? '—' })
  const mail = services.find((s) => s.id === 'mailpit')
  if (mail) rows.push({ key: mail.id, state: mail.state as ServiceState, name: 'Mailpit', version: versions.mailpit ?? '—' })
  return rows
}

export function Dashboard({ onNavigate }: ScreenProps) {
  const { services, loading } = useServices()
  const { projects } = useProjects()
  const { installed } = useRuntimes()
  const { settings } = useSettings()
  const { warnings } = useWarnings()
  const [busy, setBusy] = useState<'start' | 'stop' | null>(null)
  const [error, setError] = useState<string | null>(null)

  const run = async (kind: 'start' | 'stop') => {
    setBusy(kind)
    setError(null)
    try {
      if (kind === 'start') await ServicesService.StartAll()
      else await ServicesService.StopAll()
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setBusy(null)
    }
  }

  const [resolvendo, setResolvendo] = useState<string | null>(null)
  const resolver = async (code: string) => {
    const alvo = ONDE_RESOLVER[code]
    if (!alvo) return
    if (alvo.tela) {
      onNavigate(alvo.tela)
      return
    }
    if (!alvo.run) return
    setResolvendo(code)
    setError(null)
    try {
      await alvo.run()
    } catch (e) {
      // Recusar o UAC chega como rejeição; sem isto o erro só existiria no
      // console do WebView e o aviso continuaria na tela sem explicação.
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setResolvendo(null)
    }
  }

  const showSkeleton = services.length === 0 && loading
  const rows = stackRows(services, installed)
  const anyRunning = services.some((s) => (s.state as string) !== 'stopped')
  // "Iniciar tudo" com tudo no ar não tem o que fazer; deixar o botão ativo
  // convida ao clique que antes devolvia "serviço X já está ready".
  const allRunning = services.length > 0 && services.every((s) => (s.state as string) !== 'stopped')

  const ports: Array<{ label: string; port: number }> = settings
    ? [
        { label: 'HTTP', port: settings.httpPort },
        { label: 'HTTPS', port: settings.httpsPort },
        { label: 'MySQL', port: settings.mysqlPort },
        { label: 'SMTP (Mailpit)', port: settings.mailpitSmtpPort },
        { label: 'Mailpit UI', port: settings.mailpitHttpPort },
        ...services
          .filter((s) => s.group === 'php' && s.port > 0)
          .map((s) => ({ label: s.id, port: s.port })),
      ]
    : []

  return (
    <div className="flex flex-col gap-4">
      <div className="flex items-center gap-2">
        <Button
          variant="primary"
          icon={<Play size={16} weight="fill" />}
          loading={busy === 'start'}
          disabled={busy !== null || allRunning}
          onClick={() => void run('start')}
        >
          Iniciar tudo
        </Button>
        <Button
          variant="secondary"
          icon={<Stop size={16} weight="fill" />}
          loading={busy === 'stop'}
          disabled={busy !== null || !anyRunning}
          onClick={() => void run('stop')}
        >
          Parar tudo
        </Button>
        {error && <span className="selectable text-sm text-err">{error}</span>}
      </div>

      <Card>
        <SectionLabel>YOUR STACK</SectionLabel>
        {showSkeleton ? (
          <Skeleton lines={4} className="mt-3" />
        ) : rows.length === 0 ? (
          <EmptyState
            title="Nenhum serviço configurado"
            description="Instale um web server e uma versão de PHP em Runtimes."
            action={
              <Button variant="secondary" onClick={() => onNavigate('runtimes')}>
                Ir para Runtimes
              </Button>
            }
          />
        ) : (
          <ul className="mt-3 divide-y divide-border">
            {rows.map((r) => (
              <li key={r.key} className="flex items-center gap-3 py-2">
                <StatusDot state={r.state} />
                <span className="text-sm text-fg">{r.name}</span>
                <span className="selectable font-mono text-sm text-fg-muted">{r.version}</span>
                {r.extra && <Badge mono>{r.extra}</Badge>}
                <span className="ml-auto text-xs text-fg-faint">{r.state}</span>
              </li>
            ))}
          </ul>
        )}
      </Card>

      <div className="grid grid-cols-2 gap-4">
        <Card>
          <SectionLabel>PROJETOS</SectionLabel>
          {projects.length === 0 ? (
            <EmptyState
              title="Nenhum projeto"
              description="Adicione um diretório-raiz em Projetos."
              action={
                <Button variant="secondary" onClick={() => onNavigate('projects')}>
                  Adicionar diretório
                </Button>
              }
            />
          ) : (
            <ul className="mt-3 divide-y divide-border">
              {projects.map((p) => (
                <li key={p.id} className="flex items-center gap-3 py-2">
                  <span className="text-sm text-fg">{p.name}</span>
                  {/* phpEffective já vem resolvido pelo Go (manifesto →
                      padrão → maior instalada); repetir a regra aqui foi o que
                      fazia todo projeto sem `php:` aparecer como "PHP —". */}
                  <Badge mono>PHP {p.phpEffective || p.php || '—'}</Badge>
                  <button
                    type="button"
                    onClick={() => void AppService.OpenExternal(`https://${p.domain}`)}
                    className="ml-auto inline-flex items-center gap-1 font-mono text-sm text-accent-fg hover:underline focus-visible:outline-2 focus-visible:outline-accent"
                  >
                    {p.domain}
                    <ArrowSquareOut size={12} />
                  </button>
                </li>
              ))}
            </ul>
          )}
        </Card>

        <Card>
          <SectionLabel>PORTAS</SectionLabel>
          {ports.length === 0 ? (
            <Skeleton lines={5} className="mt-3" />
          ) : (
            <ul className="mt-3 grid grid-cols-2 gap-x-4">
              {ports.map((p) => (
                <li key={p.label} className="flex justify-between py-1 text-sm">
                  <span className="text-fg-muted">{p.label}</span>
                  <span className="selectable font-mono text-fg">{p.port}</span>
                </li>
              ))}
            </ul>
          )}
        </Card>
      </div>

      {/* Sem nenhum runtime instalado, todo aviso é consequência disso: o card
          "YOUR STACK" acima já convida a instalar, e repetir a falta como três
          alertas amarelos faz o app parecer quebrado logo na primeira abertura. */}
      {installed.length > 0 && (
        <Card>
          <SectionLabel>AVISOS</SectionLabel>
          {warnings.length === 0 ? (
            <p className="mt-3 text-sm text-fg-faint">Nenhum aviso.</p>
          ) : (
            <ul className="mt-3 flex flex-col gap-2">
              {warnings.map((w, i) => (
                <li key={`${w.code}-${w.projectId}-${i}`} className="flex items-start gap-2 text-sm">
                  <WarningIcon size={16} className="mt-0.5 shrink-0 text-warn" />
                  <div className="flex flex-col items-start gap-1">
                    <span className="selectable text-fg">{w.message}</span>
                    <span className="selectable font-mono text-xs text-fg-faint">
                      {w.code}
                      {w.projectId ? ` · ${w.projectId}` : ''}
                    </span>
                    {ONDE_RESOLVER[w.code] && (
                      <Button
                        variant="ghost"
                        size="sm"
                        loading={resolvendo === w.code}
                        disabled={resolvendo !== null}
                        onClick={() => void resolver(w.code)}
                      >
                        {ONDE_RESOLVER[w.code].acao}
                      </Button>
                    )}
                  </div>
                </li>
              ))}
            </ul>
          )}
        </Card>
      )}
    </div>
  )
}
