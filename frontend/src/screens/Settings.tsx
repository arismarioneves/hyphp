import { useCallback, useEffect, useState } from 'react'
import { FolderPlus, Trash } from '@phosphor-icons/react'
import { AppService, ProjectsService, SettingsService } from '../../bindings/hyphp/services'
// O enum gerado `state.WebServerName` é nominal: o literal 'apache' não é
// atribuível a `State.webServer`. Para comparar e gravar o web server, usa-se
// o enum do binding.
import { WebServerName } from '../../bindings/hyphp/internal/state/models'
import { Badge } from '../components/Badge'
import { Button } from '../components/Button'
import { ProgressBar } from '../components/ProgressBar'
import { Row, Section } from '../components/Section'
import { Select } from '../components/Select'
import { Skeleton } from '../components/Skeleton'
import { Toggle } from '../components/Toggle'
import type { ScreenProps } from '../lib/screens'
import type { State, SystemChanges, Warning } from '../lib/types'
import { useRuntimes } from '../lib/useRuntimes'
import { useSettings } from '../lib/useSettings'
import { useWarnings } from '../lib/useWarnings'
import { errorText } from '../lib/errors'
import { mergeDraft } from '../lib/draft'
import { LANGS, useLang, useT } from '../i18n'
import { activeDbEngine, newestVersions } from '../lib/versions'
import { byOS, usePlatform } from '../lib/platform'

/** Códigos de `stack.Warning` que o card Permissões resolve (C18.42 e C18.45). */
const HOSTS_PENDING = 'hosts-pending'
const CA_PENDING = 'ca-pending'
const WILDCARD_PENDING = 'wildcard-pending'

const WEB_SERVERS: Array<{ name: WebServerName; label: string }> = [
  { name: WebServerName.Apache, label: 'Apache' },
  { name: WebServerName.Nginx, label: 'nginx' },
]

// Valores de state.DBEngine (internal/state).
const DB_ENGINES = [
  { name: 'mysql', label: 'MySQL' },
  { name: 'mariadb', label: 'MariaDB' },
]

// `selectable` porque o body tem user-select: none — portas e caminhos precisam
// ser copiáveis para colar em .env, cliente de banco, etc.
const inputClass =
  'selectable h-9 rounded-pill border border-border bg-bg-card px-3 font-mono text-sm text-fg outline-none focus-visible:outline-2 focus-visible:outline-accent'

function PortInput({ label, value, onChange }: { label: string; value: number; onChange: (n: number) => void }) {
  const t = useT('settings')
  return (
    <Row label={label}>
      <input
        type="number"
        min={1}
        max={65535}
        value={value}
        aria-label={t('portAria', { label })}
        onChange={(e) => onChange(Number(e.target.value))}
        className={`${inputClass} w-28 text-right`}
      />
    </Row>
  )
}

// Serve para os web servers e para os motores de banco: nos dois, um card por
// opção, e o clique troca na hora.
type WebServerCardProps = {
  name: string
  label: string
  version: string | null
  active: boolean
  switching: boolean
  onSelect: () => void
}

function WebServerCard({ name, label, version, active, switching, onSelect }: WebServerCardProps) {
  const t = useT('settings')
  const installed = version !== null
  return (
    <button
      type="button"
      disabled={!installed || switching || active}
      aria-pressed={active}
      aria-label={t('useWebServer', { label })}
      onClick={onSelect}
      className={`flex flex-1 flex-col items-start gap-1 rounded-card border p-4 text-left focus-visible:outline-2 focus-visible:outline-accent disabled:cursor-not-allowed ${
        active ? 'border-accent bg-accent-soft' : 'border-border bg-bg-card hover:bg-bg-card-hover'
      } ${!installed ? 'opacity-50' : ''}`}
    >
      <div className="flex w-full items-center gap-2">
        <span className="font-mono text-lg text-fg">{label}</span>
        {active && <Badge tone="accent">{t('active')}</Badge>}
      </div>
      <span className="selectable font-mono text-sm text-fg-muted">{installed ? version : t('notInstalled')}</span>
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
  const t = useT('settings')
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
          {busy ? t('applying') : label}
        </Button>
      </div>
    </div>
  )
}

/**
 * Único ponto do produto que pede privilégio (UAC no Windows, senha no Mac; no
 * Linux as ações ainda respondem que não estão disponíveis). O Reconcile nunca
 * eleva — só reporta as pendências —, então o card aparece com pendência ou,
 * no Mac, com algo gravado no sistema para remover; sem nenhum dos dois, some
 * inteiro.
 */
function PermissionsCard({
  changes,
  warnings,
  onChanged,
}: {
  changes: SystemChanges | null
  warnings: Warning[]
  onChanged: () => void
}) {
  const t = useT('settings')
  const { os } = usePlatform()
  const hosts = warnings.find((w) => w.code === HOSTS_PENDING)
  const ca = warnings.find((w) => w.code === CA_PENDING)
  const wildcard = warnings.find((w) => w.code === WILDCARD_PENDING)
  const removable = changes !== null && changes.supported && (changes.dns || changes.path || changes.ca)
  if (!hosts && !ca && !wildcard && !removable) return null
  return (
    <Section label={t('permissions')}>
      <p className="text-xs text-fg-faint">
        {t(byOS(os, { windows: 'permissionsHint', darwin: 'permissionsHintMac', linux: 'permissionsHintLinux' }))}
      </p>
      {hosts && <ElevatedAction warning={hosts} label={t('applyHosts')} run={SettingsService.ApplyHosts} />}
      {ca && (
        <ElevatedAction
          warning={ca}
          label={t('installCA')}
          run={() => SettingsService.InstallCA().then(onChanged)}
        />
      )}
      {wildcard && (
        <ElevatedAction
          warning={wildcard}
          label={t('applyWildcardDNS')}
          run={() => SettingsService.ApplyWildcardDNS().then(onChanged)}
        />
      )}
      {removable && <RemoveSystemAction onDone={onChanged} />}
    </Section>
  )
}

/**
 * "Remover do sistema" (só no Mac): uma senha desfaz a regra de DNS, o PATH e
 * a confiança no certificado. Pede confirmação porque os sites deixam de abrir
 * pelo nome até registrar de novo.
 */
function RemoveSystemAction({ onDone }: { onDone: () => void }) {
  const t = useT('settings')
  const [confirming, setConfirming] = useState(false)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  const remove = () => {
    setBusy(true)
    setError('')
    SettingsService.RemoveSystemChanges()
      .then(() => {
        setConfirming(false)
        onDone()
      })
      .catch((e: unknown) => setError(errorText(e)))
      .finally(() => setBusy(false))
  }

  return (
    <div className="flex flex-col gap-2">
      <p className="text-sm text-fg-muted">{t('removeSystemHint')}</p>
      {error !== '' && <p className="selectable text-sm text-err">{error}</p>}
      <div className="flex items-center gap-2">
        {confirming ? (
          <>
            <span className="text-xs text-fg-muted">{t('removeSystemConfirm')}</span>
            <Button variant="danger" size="sm" loading={busy} disabled={busy} onClick={remove}>
              {t('removeSystem')}
            </Button>
            <Button variant="ghost" size="sm" disabled={busy} onClick={() => setConfirming(false)}>
              {t('cancel')}
            </Button>
          </>
        ) : (
          <Button variant="secondary" size="sm" onClick={() => setConfirming(true)}>
            {t('removeSystem')}
          </Button>
        )}
      </div>
    </div>
  )
}

export function Settings({ onNavigate }: ScreenProps) {
  const t = useT('settings')
  const { system } = useLang()
  const { os, tray } = usePlatform()
  const { settings, save } = useSettings()
  const { installed } = useRuntimes()
  const [draft, setDraft] = useState(settings)
  const [synced, setSynced] = useState(settings)
  const [saving, setSaving] = useState(false)
  const [saveError, setSaveError] = useState<string | null>(null)
  const [switching, setSwitching] = useState(false)
  const [switchError, setSwitchError] = useState<string | null>(null)
  const [switchingDb, setSwitchingDb] = useState(false)
  const [switchDbError, setSwitchDbError] = useState<string | null>(null)
  const [roots, setRoots] = useState<string[]>([])
  const [rootsError, setRootsError] = useState<string | null>(null)
  const [pathBusy, setPathBusy] = useState(false)
  const [pathMsg, setPathMsg] = useState('')
  const [pathError, setPathError] = useState<string | null>(null)
  const [changes, setChanges] = useState<SystemChanges | null>(null)
  const [pathShadow, setPathShadow] = useState('')
  const { warnings } = useWarnings()
  // Relido a cada mudança de avisos: registrar a regra ou a CA (aqui ou pelo
  // Dashboard) muda o que há para remover.
  const loadChanges = useCallback(() => {
    void SettingsService.SystemChanges().then(setChanges)
  }, [])
  useEffect(() => {
    loadChanges()
  }, [loadChanges, warnings])

  // settings:changed chega com o state inteiro a cada gravação do backend
  // (troca de web server/banco, recolher a sidebar, comandos da CLI). Só os
  // campos que o usuário editou ficam no rascunho; o resto acompanha o state
  // novo, senão uma edição em curso sumiria sem salvar. Ajuste na própria
  // renderização (padrão do React para estado derivado) em vez de um efeito,
  // que custaria um frame de skeleton.
  if (settings !== synced) {
    setSynced(settings)
    setDraft(draft && synced && settings ? mergeDraft(draft, synced, settings) : settings)
  }

  useEffect(() => {
    void ProjectsService.Roots().then((r) => setRoots(r ?? []))
  }, [])

  if (!settings || !draft) return <Skeleton lines={10} />

  const set = <K extends keyof State>(key: K, value: State[K]) => setDraft({ ...draft, [key]: value })
  const dirty = JSON.stringify(draft) !== JSON.stringify(settings)

  const versions = newestVersions(installed)
  // `Installed.kind` é o enum gerado `runtime.Kind` (nominal): comparar com o
  // literal exige alargar para string.
  const majorSeen: Record<string, true> = {}
  for (const i of installed) if ((i.kind as string) === 'php') majorSeen[i.major] = true
  const phpMajors = Object.keys(majorSeen).sort()
  const missingWebServer = WEB_SERVERS.some((w) => versions[w.name] === undefined)
  const activeDb = activeDbEngine(settings.dbEngine, versions)
  // Sem bandeja (GNOME sem a extensão AppIndicator), o login abre a janela
  // minimizada em vez de escondida.
  const autostartHidden = !tray ? 'autostartHiddenNoTray' : os === 'darwin' ? 'autostartHiddenMac' : 'autostartHidden'

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

  const switchDb = async (name: string) => {
    setSwitchingDb(true)
    setSwitchDbError(null)
    try {
      await SettingsService.SwitchDatabase(name)
    } catch (e) {
      setSwitchDbError(errorText(e))
    } finally {
      setSwitchingDb(false)
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
    setPathShadow('')
    try {
      await AppService.AddDefaultPHPToUserPath()
      setPathMsg(t(byOS(os, { windows: 'pathAdded', darwin: 'pathAddedMac', linux: 'pathAddedLinux' })))
      setPathShadow(await AppService.ShadowingPHP())
      loadChanges()
    } catch (e) {
      setPathError(errorText(e))
    } finally {
      setPathBusy(false)
    }
  }

  return (
    // O respiro no fim só existe com a barra de "alterações não salvas" à
    // vista: ela é fixa no rodapé e cobriria o último card. Sem ela, o fim
    // da tela fica igual ao das outras.
    <div className={`flex flex-col gap-4 ${dirty ? 'pb-16' : ''}`}>
      <Section label={t('webServer')}>
        <div className="flex gap-3">
          {WEB_SERVERS.map((w) => (
            <WebServerCard
              key={w.name}
              name={w.name}
              label={w.label}
              version={versions[w.name] ?? null}
              active={settings.webServer === w.name}
              switching={switching}
              onSelect={() => void switchWeb(w.name)}
            />
          ))}
        </div>
        {switching && <ProgressBar indeterminate label={t('switchingWebServer')} />}
        {switchError && <p className="selectable text-sm text-err">{switchError}</p>}
        <div className="flex items-center justify-between gap-3">
          <p className="text-xs text-fg-muted">{t('webServerHint')}</p>
          {missingWebServer && (
            <Button variant="ghost" size="sm" className="shrink-0" onClick={() => onNavigate('runtimes')}>
              {t('downloadInRuntimes')}
            </Button>
          )}
        </div>
      </Section>

      <Section label={t('database')}>
        <div className="flex gap-3">
          {DB_ENGINES.map((e) => (
            <WebServerCard
              key={e.name}
              name={e.name}
              label={e.label}
              version={versions[e.name] ?? null}
              active={activeDb === e.name}
              switching={switchingDb}
              onSelect={() => void switchDb(e.name)}
            />
          ))}
        </div>
        {switchingDb && <ProgressBar indeterminate label={t('switchingDatabase')} />}
        {switchDbError && <p className="selectable text-sm text-err">{switchDbError}</p>}
        <div className="flex items-center justify-between gap-3">
          <p className="text-xs text-fg-muted">{t('databaseHint')}</p>
          {DB_ENGINES.some((e) => versions[e.name] === undefined) && (
            <Button variant="ghost" size="sm" className="shrink-0" onClick={() => onNavigate('runtimes')}>
              {t('downloadInRuntimes')}
            </Button>
          )}
        </div>
      </Section>

      <Section label="PHP">
        <Row label={t('defaultPhp')} hint={t('defaultPhpHint')}>
          {phpMajors.length === 0 ? (
            <Button variant="secondary" size="sm" onClick={() => onNavigate('runtimes')}>
              {t('installPhp')}
            </Button>
          ) : (
            <Select
              aria-label={t('defaultPhpAria')}
              value={draft.defaultPhp}
              options={[
                { value: '', label: t('highestInstalled') },
                ...phpMajors.map((m) => ({ value: m, label: `PHP ${m}` })),
              ]}
              onChange={(v) => set('defaultPhp', v)}
              className="w-48"
            />
          )}
        </Row>
      </Section>

      <Section label={t('pool')}>
        <Row label={t(os === 'windows' ? 'poolWorkers' : 'poolWorkersFpm')} hint={t('poolWorkersHint')}>
          <Select
            aria-label={t('poolSizeAria')}
            value={String(draft.poolSize)}
            options={[1, 2, 3, 4, 5, 6, 7, 8].map((n) => ({ value: String(n), label: String(n) }))}
            onChange={(v) => set('poolSize', Number(v))}
            className="w-24"
          />
        </Row>
      </Section>

      <Section label={t('ports')}>
        <PortInput label="HTTP" value={draft.httpPort} onChange={(n) => set('httpPort', n)} />
        <PortInput label="HTTPS" value={draft.httpsPort} onChange={(n) => set('httpsPort', n)} />
        <PortInput label="MySQL / MariaDB" value={draft.mysqlPort} onChange={(n) => set('mysqlPort', n)} />
        <PortInput label="SMTP (Mailpit)" value={draft.mailpitSmtpPort} onChange={(n) => set('mailpitSmtpPort', n)} />
        <PortInput label="Mailpit UI" value={draft.mailpitHttpPort} onChange={(n) => set('mailpitHttpPort', n)} />
      </Section>

      <PermissionsCard changes={changes} warnings={warnings} onChanged={loadChanges} />

      <Section label={t('directories')}>
        {roots.length === 0 ? (
          <p className="text-sm text-fg-faint">{t('noRoots')}</p>
        ) : (
          <ul className="divide-y divide-border">
            {roots.map((r) => (
              <li key={r} className="flex items-center justify-between py-2">
                <span className="selectable font-mono text-sm text-fg">{r}</span>
                <Button
                  variant="ghost"
                  size="sm"
                  aria-label={t('removeRoot', { dir: r })}
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
            {t('addRoot')}
          </Button>
        </div>
      </Section>

      <Section label={t('tools')}>
        <Row label={t('editor')} hint={t('editorHint')}>
          <input
            value={draft.editor}
            aria-label={t('editor')}
            placeholder={byOS(os, {
              windows: 'C:\\Users\\...\\Code.exe',
              darwin: 'Visual Studio Code.app',
              linux: '/usr/bin/code',
            })}
            onChange={(e) => set('editor', e.target.value)}
            className={`${inputClass} w-96`}
          />
        </Row>
        <Row
          label={t('terminal')}
          hint={t(byOS(os, { windows: 'terminalHint', darwin: 'terminalHintMac', linux: 'terminalHintLinux' }))}
        >
          <input
            value={draft.terminal}
            aria-label={t('terminal')}
            placeholder={byOS(os, { windows: 'wt.exe', darwin: 'iTerm', linux: 'gnome-terminal' })}
            onChange={(e) => set('terminal', e.target.value)}
            className={`${inputClass} w-96`}
          />
        </Row>
        <Row label={t('phpInPath')} hint={t('phpInPathHint')}>
          <div className="flex items-center gap-3">
            <Button variant="secondary" size="sm" loading={pathBusy} onClick={() => void addPHPToPath()}>
              {t('addPhpToPath')}
            </Button>
            {pathMsg && <span className="text-sm text-fg-muted">{pathMsg}</span>}
            {pathError && <span className="selectable text-sm text-err">{pathError}</span>}
            {pathShadow && (
              <span className="selectable text-sm text-warn">
                {t(os === 'linux' ? 'phpShadowedLinux' : 'phpShadowed', { path: pathShadow })}
              </span>
            )}
          </div>
        </Row>
      </Section>

      <Section
        label={t(byOS(os, { windows: 'autostartSection', darwin: 'autostartSectionMac', linux: 'autostartSectionLinux' }))}
      >
        <Row label={t('autostart')} hint={t('autostartHint')}>
          <Toggle checked={draft.autostart} label={t('autostart')} onChange={(on) => set('autostart', on)} />
        </Row>
        <Row label={t(autostartHidden)} hint={t('autostartHiddenHint')}>
          <Toggle
            checked={draft.autostartHidden}
            disabled={!draft.autostart}
            label={t(autostartHidden)}
            onChange={(on) => set('autostartHidden', on)}
          />
        </Row>
      </Section>

      <Section label={t('appearance')}>
        <Row label={t('theme')}>
          <Select
            aria-label={t('theme')}
            value={draft.theme || 'dark'}
            options={[
              { value: 'dark', label: t('themeDark') },
              { value: 'light', label: t('themeLight') },
              {
                value: 'system',
                label: t(byOS(os, { windows: 'themeSystem', darwin: 'themeSystemMac', linux: 'themeSystemLinux' })),
              },
            ]}
            onChange={(v) => set('theme', v)}
            className="w-64"
          />
        </Row>
        <Row label={t('language')}>
          <Select
            aria-label={t('language')}
            value={draft.language}
            options={[
              {
                value: '',
                label: t(
                  byOS(os, { windows: 'languageSystem', darwin: 'languageSystemMac', linux: 'languageSystemLinux' }),
                  { lang: LANGS.find((l) => l.code === system)?.label ?? system },
                ),
              },
              ...LANGS.map((l) => ({ value: l.code, label: l.label })),
            ]}
            onChange={(v) => set('language', v)}
            className="w-64"
          />
        </Row>
      </Section>

      <Section label={t('quitSection')}>
        <Row label={t('quitLabel')} hint={t(tray ? 'quitHint' : 'quitHintNoTray')}>
          <Button variant="danger" size="sm" onClick={() => void AppService.Quit()}>
            {t('quit')}
          </Button>
        </Row>
      </Section>

      {dirty && (
        <div className="fixed bottom-4 right-6 flex items-center gap-3 rounded-card border border-border bg-bg-card px-4 py-2 shadow-lg">
          <span className="text-sm text-fg-muted">{t('unsaved')}</span>
          {saveError && <span className="selectable text-sm text-err">{saveError}</span>}
          <Button variant="ghost" size="sm" onClick={() => setDraft(settings)} disabled={saving}>
            {t('discard')}
          </Button>
          <Button variant="primary" size="sm" onClick={() => void doSave()} loading={saving} disabled={saving}>
            {t('save')}
          </Button>
        </div>
      )}
    </div>
  )
}
