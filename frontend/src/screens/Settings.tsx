import { useEffect, useState, type ReactNode } from 'react'
import { FolderPlus, Trash } from '@phosphor-icons/react'
import { AppService, ProjectsService, SettingsService } from '../../bindings/hyphp/services'
// O enum gerado `state.WebServerName` é nominal: o literal 'apache' não é
// atribuível a `State.webServer`, e a união de `lib/types` só serve para leitura.
// Para comparar e gravar o web server, usa-se o enum do binding.
import { WebServerName } from '../../bindings/hyphp/internal/state/models'
import { Badge } from '../components/Badge'
import { Button } from '../components/Button'
import { Card } from '../components/Card'
import { ProgressBar } from '../components/ProgressBar'
import { SectionLabel } from '../components/SectionLabel'
import { Select } from '../components/Select'
import { Skeleton } from '../components/Skeleton'
import { Toggle } from '../components/Toggle'
import type { ScreenProps } from '../lib/screens'
import type { State, Warning } from '../lib/types'
import { useRuntimes } from '../lib/useRuntimes'
import { useSettings } from '../lib/useSettings'
import { useWarnings } from '../lib/useWarnings'

/** Códigos de `stack.Warning` que o card Permissões resolve (C18.42 e C18.45). */
const HOSTS_PENDING = 'hosts-pending'
const CA_PENDING = 'ca-pending'
const WILDCARD_PENDING = 'wildcard-pending'

const WEB_SERVERS: Array<{ name: WebServerName; label: string }> = [
  { name: WebServerName.Apache, label: 'Apache' },
  { name: WebServerName.Nginx, label: 'nginx' },
]

function errorText(e: unknown): string {
  return e instanceof Error ? e.message : String(e)
}

function Section({ label, children }: { label: string; children: ReactNode }) {
  return (
    <Card>
      <SectionLabel>{label}</SectionLabel>
      <div className="mt-3 flex flex-col gap-3">{children}</div>
    </Card>
  )
}

function Row({ label, hint, children }: { label: string; hint?: string; children: ReactNode }) {
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

// `selectable` porque o body tem user-select: none — portas e caminhos precisam
// ser copiáveis para colar em .env, cliente de banco, etc.
const inputClass =
  'selectable h-9 rounded-pill border border-border bg-bg-card px-3 font-mono text-sm text-fg outline-none focus-visible:outline-2 focus-visible:outline-accent'

function PortInput({ label, value, onChange }: { label: string; value: number; onChange: (n: number) => void }) {
  return (
    <Row label={label}>
      <input
        type="number"
        min={1}
        max={65535}
        value={value}
        aria-label={`Porta ${label}`}
        onChange={(e) => onChange(Number(e.target.value))}
        className={`${inputClass} w-28 text-right`}
      />
    </Row>
  )
}

type WebServerCardProps = {
  name: WebServerName
  label: string
  version: string | null
  active: boolean
  switching: boolean
  onSelect: () => void
}

function WebServerCard({ name, label, version, active, switching, onSelect }: WebServerCardProps) {
  const installed = version !== null
  return (
    <button
      type="button"
      disabled={!installed || switching || active}
      aria-pressed={active}
      aria-label={`Usar ${label}`}
      onClick={onSelect}
      className={`flex flex-1 flex-col items-start gap-1 rounded-card border p-4 text-left focus-visible:outline-2 focus-visible:outline-accent disabled:cursor-not-allowed ${
        active ? 'border-accent bg-accent-soft' : 'border-border bg-bg-card hover:bg-bg-card-hover'
      } ${!installed ? 'opacity-50' : ''}`}
    >
      <div className="flex w-full items-center gap-2">
        <span className="font-mono text-lg text-fg">{label}</span>
        {active && <Badge tone="accent">ativo</Badge>}
      </div>
      <span className="selectable font-mono text-sm text-fg-muted">{installed ? version : 'não instalado'}</span>
      <span className="font-mono text-xs text-fg-faint">{name}</span>
    </button>
  )
}

type ElevatedActionProps = { warning: Warning; label: string; run: () => Promise<void> }

/**
 * Uma pendência que só o administrador resolve. A mensagem vem pronta do
 * Reconcile (lista de domínios, caminho da CA) — reconstruí-la aqui duplicaria
 * a regra do backend. Recusar o UAC rejeita a promessa: sem o `catch` o erro
 * morreria no console do WebView e o botão pareceria não ter feito nada.
 */
function ElevatedAction({ warning, label, run }: ElevatedActionProps) {
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  const trigger = () => {
    setBusy(true)
    setError('')
    run()
      .catch((e: unknown) => setError(errorText(e)))
      .finally(() => setBusy(false))
  }

  return (
    <div className="flex flex-col gap-2">
      <p className="selectable text-sm text-fg-muted">{warning.message}</p>
      {error !== '' && <p className="selectable text-sm text-err">{error}</p>}
      <div>
        <Button variant="primary" size="sm" loading={busy} disabled={busy} onClick={trigger}>
          {busy ? 'Aplicando…' : label}
        </Button>
      </div>
    </div>
  )
}

/**
 * Único ponto do produto que dispara UAC. O Reconcile nunca eleva — só reporta
 * as pendências —, então sem pendência não há o que aplicar e o card some
 * inteiro: botão que não faz nada é pior que botão nenhum.
 */
function PermissionsCard() {
  const { warnings } = useWarnings()
  const hosts = warnings.find((w) => w.code === HOSTS_PENDING)
  const ca = warnings.find((w) => w.code === CA_PENDING)
  const wildcard = warnings.find((w) => w.code === WILDCARD_PENDING)
  if (!hosts && !ca && !wildcard) return null
  return (
    <Section label="PERMISSÕES">
      <p className="text-xs text-fg-faint">
        Estas ações exigem permissão de administrador e pedem confirmação do Windows. Nenhuma outra parte do HyPHP
        eleva privilégio.
      </p>
      {hosts && <ElevatedAction warning={hosts} label="Aplicar domínios" run={SettingsService.ApplyHosts} />}
      {ca && <ElevatedAction warning={ca} label="Instalar certificado" run={SettingsService.InstallCA} />}
      {wildcard && (
        <ElevatedAction warning={wildcard} label="Registrar regra de DNS" run={SettingsService.ApplyWildcardDNS} />
      )}
    </Section>
  )
}

export function Settings({ onNavigate }: ScreenProps) {
  const { settings, save } = useSettings()
  const { installed } = useRuntimes()
  const [draft, setDraft] = useState(settings)
  const [synced, setSynced] = useState(settings)
  const [saving, setSaving] = useState(false)
  const [saveError, setSaveError] = useState<string | null>(null)
  const [switching, setSwitching] = useState(false)
  const [switchError, setSwitchError] = useState<string | null>(null)
  const [roots, setRoots] = useState<string[]>([])
  const [rootsError, setRootsError] = useState<string | null>(null)
  const [version, setVersion] = useState('')
  const [runtimeRoot, setRuntimeRoot] = useState('')
  const [pathBusy, setPathBusy] = useState(false)
  const [pathMsg, setPathMsg] = useState('')
  const [pathError, setPathError] = useState<string | null>(null)

  // O backend reemite o state inteiro (settings:changed) a cada Reconcile e a
  // cada troca de web server; quando isso acontece o rascunho volta a espelhar
  // o que está gravado. Ajuste na própria renderização (padrão do React para
  // estado derivado) em vez de um efeito, que custaria um frame de skeleton.
  if (settings !== synced) {
    setSynced(settings)
    setDraft(settings)
  }

  useEffect(() => {
    void ProjectsService.Roots().then((r) => setRoots(r ?? []))
    void AppService.Version().then(setVersion)
    void AppService.RuntimeRoot().then(setRuntimeRoot)
  }, [])

  if (!settings || !draft) return <Skeleton lines={10} />

  const set = <K extends keyof State>(key: K, value: State[K]) => setDraft({ ...draft, [key]: value })
  const dirty = JSON.stringify(draft) !== JSON.stringify(settings)

  // `Installed.kind` é o enum gerado `runtime.Kind` (nominal): comparar com o
  // literal exige alargar para string.
  const webVersions: Record<string, string> = {}
  const majorSeen: Record<string, true> = {}
  for (const i of installed) {
    const kind = i.kind as string
    if (kind === 'apache' || kind === 'nginx') webVersions[kind] = i.version
    if (kind === 'php') majorSeen[i.major] = true
  }
  const phpMajors = Object.keys(majorSeen).sort()
  const missingWebServer = WEB_SERVERS.some((w) => webVersions[w.name] === undefined)

  const doSave = async () => {
    setSaving(true)
    setSaveError(null)
    try {
      await save(draft)
    } catch (e) {
      setSaveError(errorText(e))
    } finally {
      setSaving(false)
    }
  }

  const switchWeb = async (name: WebServerName) => {
    setSwitching(true)
    setSwitchError(null)
    try {
      await SettingsService.SwitchWebServer(name)
    } catch (e) {
      setSwitchError(errorText(e))
    } finally {
      setSwitching(false)
    }
  }

  // PickRoot devolve "" quando o usuário cancela o diálogo nativo.
  const addRoot = async () => {
    setRootsError(null)
    try {
      const dir = await ProjectsService.PickRoot()
      if (!dir) return
      await ProjectsService.AddRoot(dir)
      setRoots((await ProjectsService.Roots()) ?? [])
    } catch (e) {
      setRootsError(errorText(e))
    }
  }

  const removeRoot = async (dir: string) => {
    setRootsError(null)
    try {
      await ProjectsService.RemoveRoot(dir)
      setRoots((await ProjectsService.Roots()) ?? [])
    } catch (e) {
      setRootsError(errorText(e))
    }
  }

  // O aviso do terminal novo é obrigatório: processos já em execução herdaram
  // o ambiente antigo e continuariam achando o php de outra instalação.
  const addPHPToPath = async () => {
    setPathBusy(true)
    setPathMsg('')
    setPathError(null)
    try {
      await AppService.AddDefaultPHPToUserPath()
      setPathMsg('PHP adicionado ao PATH do usuário. Abra um terminal novo para valer.')
    } catch (e) {
      setPathError(errorText(e))
    } finally {
      setPathBusy(false)
    }
  }

  return (
    <div className="flex flex-col gap-4 pb-16">
      <Section label="WEB SERVER">
        <div className="flex gap-3">
          {WEB_SERVERS.map((w) => (
            <WebServerCard
              key={w.name}
              name={w.name}
              label={w.label}
              version={webVersions[w.name] ?? null}
              active={settings.webServer === w.name}
              switching={switching}
              onSelect={() => void switchWeb(w.name)}
            />
          ))}
        </div>
        {switching && <ProgressBar indeterminate label="Trocando web server: validando config e subindo o novo…" />}
        {switchError && <p className="selectable text-sm text-err">{switchError}</p>}
        <div className="flex items-center justify-between gap-3">
          <p className="text-xs text-fg-muted">
            Um servidor ativo por vez (ambos usam 80/443). A troca valida a nova config antes de parar o atual; se
            falhar, o anterior continua no ar.
          </p>
          {missingWebServer && (
            <Button variant="ghost" size="sm" onClick={() => onNavigate('runtimes')}>
              Baixar em Runtimes
            </Button>
          )}
        </div>
      </Section>

      <Section label="PHP">
        <Row label="Versão padrão" hint="Usada por projetos sem `php:` no hyphp.yaml.">
          {phpMajors.length === 0 ? (
            <Button variant="secondary" size="sm" onClick={() => onNavigate('runtimes')}>
              Instalar PHP
            </Button>
          ) : (
            <Select
              aria-label="Versão padrão de PHP"
              value={draft.defaultPhp}
              options={[
                { value: '', label: 'Maior instalada' },
                ...phpMajors.map((m) => ({ value: m, label: `PHP ${m}` })),
              ]}
              onChange={(v) => set('defaultPhp', v)}
              className="w-48"
            />
          )}
        </Row>
      </Section>

      <Section label="POOL">
        <Row label="Workers php-cgi por versão" hint="Cada worker atende um request por vez. 4 é o default medido.">
          <Select
            aria-label="Tamanho do pool"
            value={String(draft.poolSize)}
            options={[1, 2, 3, 4, 5, 6, 7, 8].map((n) => ({ value: String(n), label: String(n) }))}
            onChange={(v) => set('poolSize', Number(v))}
            className="w-24"
          />
        </Row>
      </Section>

      <Section label="PORTAS">
        <PortInput label="HTTP" value={draft.httpPort} onChange={(n) => set('httpPort', n)} />
        <PortInput label="HTTPS" value={draft.httpsPort} onChange={(n) => set('httpsPort', n)} />
        <PortInput label="MySQL" value={draft.mysqlPort} onChange={(n) => set('mysqlPort', n)} />
        <PortInput label="SMTP (Mailpit)" value={draft.mailpitSmtpPort} onChange={(n) => set('mailpitSmtpPort', n)} />
        <PortInput label="Mailpit UI" value={draft.mailpitHttpPort} onChange={(n) => set('mailpitHttpPort', n)} />
      </Section>

      <PermissionsCard />

      <Section label="DIRETÓRIOS">
        {roots.length === 0 ? (
          <p className="text-sm text-fg-faint">Nenhum diretório-raiz.</p>
        ) : (
          <ul className="divide-y divide-border">
            {roots.map((r) => (
              <li key={r} className="flex items-center justify-between py-2">
                <span className="selectable font-mono text-sm text-fg">{r}</span>
                <Button
                  variant="ghost"
                  size="sm"
                  aria-label={`Remover ${r}`}
                  icon={<Trash size={14} />}
                  onClick={() => void removeRoot(r)}
                />
              </li>
            ))}
          </ul>
        )}
        {rootsError && <p className="selectable text-sm text-err">{rootsError}</p>}
        <div>
          <Button variant="secondary" size="sm" icon={<FolderPlus size={14} />} onClick={() => void addRoot()}>
            Adicionar diretório
          </Button>
        </div>
      </Section>

      <Section label="FERRAMENTAS">
        <Row label="Editor" hint="Caminho do executável. Vazio = `code` no PATH.">
          <input
            value={draft.editor}
            aria-label="Editor"
            placeholder="C:\Users\...\Code.exe"
            onChange={(e) => set('editor', e.target.value)}
            className={`${inputClass} w-96`}
          />
        </Row>
        <Row label="Terminal" hint="Vazio = wt.exe se existir, senão cmd.">
          <input
            value={draft.terminal}
            aria-label="Terminal"
            placeholder="wt.exe"
            onChange={(e) => set('terminal', e.target.value)}
            className={`${inputClass} w-96`}
          />
        </Row>
        <Row
          label="PHP no PATH"
          hint="Deixa `php` e `composer` do terminal na mesma versão que o HyPHP serve."
        >
          <div className="flex items-center gap-3">
            <Button variant="secondary" size="sm" loading={pathBusy} onClick={() => void addPHPToPath()}>
              Adicionar PHP ao PATH
            </Button>
            {pathMsg && <span className="text-sm text-fg-muted">{pathMsg}</span>}
            {pathError && <span className="selectable text-sm text-err">{pathError}</span>}
          </div>
        </Row>
      </Section>

      <Section label="INICIAR COM O WINDOWS">
        <Row label="Iniciar o HyPHP no login" hint="Abre minimizado no tray.">
          <Toggle checked={draft.autostart} label="Iniciar com o Windows" onChange={(on) => set('autostart', on)} />
        </Row>
      </Section>

      <Section label="SOBRE">
        <Row label="Versão">
          <span className="selectable font-mono text-sm text-fg">{version || '—'}</span>
        </Row>
        <Row label="Raiz de runtime">
          <span className="selectable font-mono text-sm text-fg">{runtimeRoot || '—'}</span>
        </Row>
        <Row label="Tema">
          <span className="text-sm text-fg-muted">Escuro (fixo na v1)</span>
        </Row>
        <div>
          <Button variant="danger" size="sm" onClick={() => void AppService.Quit()}>
            Sair do HyPHP
          </Button>
        </div>
      </Section>

      {dirty && (
        <div className="fixed bottom-4 right-6 flex items-center gap-3 rounded-card border border-border bg-bg-card px-4 py-2 shadow-lg">
          <span className="text-sm text-fg-muted">Alterações não salvas</span>
          {saveError && <span className="selectable text-sm text-err">{saveError}</span>}
          <Button variant="ghost" size="sm" onClick={() => setDraft(settings)} disabled={saving}>
            Descartar
          </Button>
          <Button variant="primary" size="sm" onClick={() => void doSave()} loading={saving} disabled={saving}>
            Salvar
          </Button>
        </div>
      )}
    </div>
  )
}
