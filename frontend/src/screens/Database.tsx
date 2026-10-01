import { useCallback, useEffect, useState } from 'react'
import { ArrowSquareOut, Database as DatabaseIcon, DownloadSimple, Plus, Trash } from '@phosphor-icons/react'
import { AppService, DatabaseService } from '../../bindings/hyphp/services'
import { Button } from '../components/Button'
import { Card } from '../components/Card'
import { CopyButton } from '../components/CopyButton'
import { EmptyState } from '../components/EmptyState'
import { SectionLabel } from '../components/SectionLabel'
import { Skeleton } from '../components/Skeleton'
import { StatusDot, type ServiceState } from '../components/StatusDot'
import type { ScreenProps } from '../lib/screens'
import type { Credentials, DBInfo } from '../lib/types'
import { errorText } from '../lib/errors'
import { useServices } from '../lib/useServices'
import { useT } from '../i18n'

/** Mesma validação do mysqlcli.ValidateName no Go: o erro chega antes da ida ao backend. */
const NAME_RE = /^[a-z0-9_]{1,64}$/

export function Database({ onNavigate }: ScreenProps) {
  const t = useT('database')
  const tc = useT('common')
  const { services } = useServices()
  const mysql = services.find((s) => s.id === 'mysql')
  // `ServiceStatus.state` é o enum gerado `supervisor.State` (nominal).
  const state = (mysql?.state ?? 'stopped') as ServiceState
  // O serviço "mysql" roda o motor escolhido em Configurações; o nome dele
  // ("MariaDB 11.4.13") diz qual.
  const engine = mysql?.name.split(' ')[0] ?? 'MySQL'

  const [creds, setCreds] = useState<Credentials | null>(null)
  const [pmaURL, setPmaURL] = useState<string | null>(null)
  const [dbs, setDbs] = useState<DBInfo[] | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [newName, setNewName] = useState('')
  const [creating, setCreating] = useState(false)
  const [confirmDrop, setConfirmDrop] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  const loadDbs = useCallback(async () => {
    try {
      const list = await DatabaseService.Databases()
      setDbs(list ?? [])
      setError(null)
    } catch (e) {
      setError(errorText(e))
      setDbs([])
    }
  }, [])

  // Porta, comando do cliente e URL do phpMyAdmin dependem do motor ativo: a
  // troca de motor (pela CLI ou por outra tela) muda o nome/estado do serviço
  // sem desmontar esta tela, então relê nesses momentos. `cancelled` impede
  // que a resposta de um motor antigo sobrescreva a do novo.
  // Em PhpMyAdminURL, "" significa não instalado; null é "ainda não perguntei
  // ao Go", e nesse intervalo nenhum dos dois botões deve aparecer piscando.
  const mysqlName = mysql?.name
  useEffect(() => {
    let cancelled = false
    void DatabaseService.Credentials().then((c) => {
      if (!cancelled) setCreds(c)
    })
    void DatabaseService.PhpMyAdminURL().then((u) => {
      if (!cancelled) setPmaURL(u)
    })
    return () => {
      cancelled = true
    }
  }, [mysqlName, state])

  // sem servidor no ar não há schema para listar: volta ao skeleton em vez de manter dados velhos.
  useEffect(() => {
    if (state === 'ready') void loadDbs()
    else setDbs(null)
  }, [state, loadDbs])

  const create = async () => {
    if (!NAME_RE.test(newName)) {
      setError(t('invalidName'))
      return
    }
    setBusy(true)
    try {
      await DatabaseService.Create(newName)
      setNewName('')
      setCreating(false)
      await loadDbs()
    } catch (e) {
      setError(errorText(e))
    } finally {
      setBusy(false)
    }
  }

  const drop = async (name: string) => {
    setBusy(true)
    try {
      await DatabaseService.Drop(name)
      setConfirmDrop(null)
      await loadDbs()
    } catch (e) {
      setError(errorText(e))
    } finally {
      setBusy(false)
    }
  }

  // Vem pronto do Go (mysqlcli.Client.Command), com o caminho completo do
  // cliente que casa com o banco ativo: o `mysql` do PATH pode ser de outra
  // instalação, e `mariadb` nem costuma estar lá.
  const connCmd = creds?.command ?? ''

  return (
    <div className="flex flex-col gap-4">
      <Card>
        <div className="flex items-center justify-between gap-3">
          <SectionLabel>{engine.toUpperCase()}</SectionLabel>
          {pmaURL === null ? null : pmaURL === '' ? (
            <Button
              variant="secondary"
              size="sm"
              icon={<DownloadSimple size={14} />}
              onClick={() => onNavigate('runtimes')}
            >
              {t('installPma')}
            </Button>
          ) : (
            // Com o MySQL parado o phpMyAdmin abre direto numa tela de erro de
            // conexão: desabilitar e dizer o porquê é mais honesto do que
            // mandar o usuário ao navegador para ver a falha lá.
            <span title={state === 'ready' ? undefined : t('pmaNeedsMysql', { engine })}>
              <Button
                variant="secondary"
                size="sm"
                icon={<ArrowSquareOut size={14} />}
                disabled={state !== 'ready'}
                onClick={() =>
                  void AppService.OpenExternal(pmaURL).catch((e: unknown) => setError(errorText(e)))
                }
              >
                {t('openPma')}
              </Button>
            </span>
          )}
        </div>
        <div className="mt-3 flex items-center gap-3">
          <StatusDot state={state} />
          <span className="text-sm text-fg">{mysql?.name ?? engine}</span>
          <span className="text-xs text-fg-faint">{tc(`state_${state}`)}</span>
          {mysql?.lastError && <span className="selectable text-xs text-err">{mysql.lastError}</span>}
        </div>
        {creds === null ? (
          <Skeleton lines={3} className="mt-3" />
        ) : (
          <dl className="mt-3 grid grid-cols-[120px_1fr_auto] items-center gap-y-1 text-sm">
            <dt className="text-fg-muted">Host</dt>
            <dd className="selectable font-mono text-fg">
              {creds.host}:{creds.port}
            </dd>
            <dd>
              <CopyButton text={`${creds.host}:${creds.port}`} label={t('copyHost')} />
            </dd>
            <dt className="text-fg-muted">{t('user')}</dt>
            <dd className="selectable font-mono text-fg">{creds.user}</dd>
            <dd>
              <CopyButton text={creds.user} label={t('copyUser')} />
            </dd>
            <dt className="text-fg-muted">{t('password')}</dt>
            <dd className="selectable font-mono text-fg">
              {creds.password === '' ? <span className="text-fg-faint">{t('empty')}</span> : creds.password}
            </dd>
            <dd>
              <CopyButton text={creds.password} label={t('copyPassword')} />
            </dd>
            <dt className="text-fg-muted">{t('connection')}</dt>
            <dd className="selectable break-all font-mono text-fg">{connCmd}</dd>
            <dd>
              <CopyButton text={connCmd} label={t('copyConnection')} />
            </dd>
          </dl>
        )}
      </Card>

      <Card
        title={t('databasesTitle')}
        actions={
          <Button
            variant="secondary"
            size="sm"
            icon={<Plus size={14} />}
            disabled={state !== 'ready' || busy}
            onClick={() => setCreating(true)}
          >
            {t('create')}
          </Button>
        }
      >
        {error && <div className="selectable mb-3 text-sm text-err">{error}</div>}
        {creating && (
          <form
            className="mb-3 flex items-center gap-2"
            onSubmit={(e) => {
              e.preventDefault()
              void create()
            }}
          >
            <input
              autoFocus
              value={newName}
              onChange={(e) => setNewName(e.target.value)}
              placeholder={t('namePlaceholder')}
              aria-label={t('newNameAria')}
              className="selectable h-9 w-64 rounded-pill border border-border bg-bg-card px-3 font-mono text-sm text-fg outline-none focus-visible:outline-2 focus-visible:outline-accent"
            />
            <Button type="submit" variant="primary" size="sm" loading={busy} disabled={busy}>
              {t('create')}
            </Button>
            <Button
              type="button"
              variant="ghost"
              size="sm"
              onClick={() => {
                setCreating(false)
                setNewName('')
              }}
            >
              {t('cancel')}
            </Button>
          </form>
        )}
        {state !== 'ready' ? (
          <EmptyState
            icon={<DatabaseIcon size={24} />}
            title={t('notReadyTitle', { engine })}
            description={t('notReadyDescription')}
            action={
              <Button variant="secondary" size="sm" onClick={() => onNavigate('services')}>
                {t('goToServices')}
              </Button>
            }
          />
        ) : dbs === null ? (
          <Skeleton lines={5} />
        ) : dbs.length === 0 ? (
          <EmptyState title={t('noDbsTitle')} description={t('noDbsDescription')} />
        ) : (
          <table className="w-full text-sm">
            <thead className="text-left text-xs text-fg-faint">
              <tr>
                <th className="py-1 font-normal">{t('name')}</th>
                <th className="py-1 text-right font-normal">{t('size')}</th>
                <th className="py-1 text-right font-normal" />
              </tr>
            </thead>
            <tbody className="divide-y divide-border">
              {dbs.map((db) => (
                <tr key={db.name} className="hover:bg-bg-card-hover">
                  <td className="selectable py-2 font-mono text-fg">{db.name}</td>
                  <td className="py-2 text-right font-mono text-fg-muted">{db.sizeMb.toFixed(1)} MB</td>
                  <td className="py-2">
                    <div className="flex items-center justify-end gap-1">
                      {confirmDrop === db.name ? (
                        <>
                          <span className="text-xs text-fg-muted">{t('dropConfirm', { name: db.name })}</span>
                          <Button
                            variant="danger"
                            size="sm"
                            loading={busy}
                            disabled={busy}
                            onClick={() => void drop(db.name)}
                          >
                            {t('drop')}
                          </Button>
                          <Button variant="ghost" size="sm" onClick={() => setConfirmDrop(null)}>
                            {t('cancel')}
                          </Button>
                        </>
                      ) : (
                        <Button
                          variant="ghost"
                          size="sm"
                          aria-label={t('dropAria', { name: db.name })}
                          icon={<Trash size={14} />}
                          disabled={busy}
                          onClick={() => setConfirmDrop(db.name)}
                        />
                      )}
                    </div>
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
