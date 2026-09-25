import { useCallback, useEffect, useState } from 'react'
import { ArrowSquareOut, Copy, Database as DatabaseIcon, DownloadSimple, Plus, Trash } from '@phosphor-icons/react'
import { AppService, DatabaseService } from '../../bindings/hyphp/services'
import { Button } from '../components/Button'
import { Card } from '../components/Card'
import { EmptyState } from '../components/EmptyState'
import { SectionLabel } from '../components/SectionLabel'
import { Skeleton } from '../components/Skeleton'
import { StatusDot, type ServiceState } from '../components/StatusDot'
import type { ScreenProps } from '../lib/screens'
import type { Credentials, DBInfo } from '../lib/types'
import { useServices } from '../lib/useServices'

/** Mesma validação do mysqlcli.ValidateName no Go: o erro chega antes da ida ao backend. */
const NAME_RE = /^[a-z0-9_]{1,64}$/

function CopyButton({ text, label }: { text: string; label: string }) {
  const [done, setDone] = useState(false)
  return (
    <Button
      variant="ghost"
      size="sm"
      aria-label={label}
      icon={<Copy size={14} />}
      onClick={() => {
        void navigator.clipboard.writeText(text).then(() => {
          setDone(true)
          setTimeout(() => setDone(false), 1200)
        })
      }}
    >
      {done ? 'copiado' : undefined}
    </Button>
  )
}

export function Database({ onNavigate }: ScreenProps) {
  const { services } = useServices()
  const mysql = services.find((s) => s.id === 'mysql')
  // `ServiceStatus.state` é o enum gerado `supervisor.State` (nominal).
  const state = (mysql?.state ?? 'stopped') as ServiceState

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
      setError(e instanceof Error ? e.message : String(e))
      setDbs([])
    }
  }, [])

  useEffect(() => {
    void DatabaseService.Credentials().then(setCreds)
  }, [])

  // "" significa phpMyAdmin não instalado; null é "ainda não perguntei ao Go",
  // e nesse intervalo nenhum dos dois botões deve aparecer piscando.
  useEffect(() => {
    void DatabaseService.PhpMyAdminURL().then(setPmaURL)
  }, [])

  // sem servidor no ar não há schema para listar: volta ao skeleton em vez de manter dados velhos.
  useEffect(() => {
    if (state === 'ready') void loadDbs()
    else setDbs(null)
  }, [state, loadDbs])

  const create = async () => {
    if (!NAME_RE.test(newName)) {
      setError('Nome inválido: use [a-z0-9_], até 64 caracteres.')
      return
    }
    setBusy(true)
    try {
      await DatabaseService.Create(newName)
      setNewName('')
      setCreating(false)
      await loadDbs()
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
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
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setBusy(false)
    }
  }

  const connCmd = creds
    ? `mysql -u${creds.user}${creds.password ? ` -p${creds.password}` : ''} -h${creds.host} -P${creds.port}`
    : ''

  return (
    <div className="flex flex-col gap-4">
      <Card>
        <div className="flex items-center justify-between gap-3">
          <SectionLabel>MYSQL</SectionLabel>
          {pmaURL === null ? null : pmaURL === '' ? (
            <Button
              variant="secondary"
              size="sm"
              icon={<DownloadSimple size={14} />}
              onClick={() => onNavigate('runtimes')}
            >
              Instalar phpMyAdmin
            </Button>
          ) : (
            // Com o MySQL parado o phpMyAdmin abre direto numa tela de erro de
            // conexão: desabilitar e dizer o porquê é mais honesto do que
            // mandar o usuário ao navegador para ver a falha lá.
            <span title={state === 'ready' ? undefined : 'Inicie o MySQL antes: sem servidor o phpMyAdmin abre com erro de conexão.'}>
              <Button
                variant="secondary"
                size="sm"
                icon={<ArrowSquareOut size={14} />}
                disabled={state !== 'ready'}
                onClick={() =>
                  void AppService.OpenExternal(pmaURL).catch((e: unknown) =>
                    setError(e instanceof Error ? e.message : String(e)),
                  )
                }
              >
                Abrir phpMyAdmin
              </Button>
            </span>
          )}
        </div>
        <div className="mt-3 flex items-center gap-3">
          <StatusDot state={state} />
          <span className="text-sm text-fg">{mysql?.name ?? 'MySQL'}</span>
          <span className="text-xs text-fg-faint">{state}</span>
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
              <CopyButton text={`${creds.host}:${creds.port}`} label="Copiar host" />
            </dd>
            <dt className="text-fg-muted">Usuário</dt>
            <dd className="selectable font-mono text-fg">{creds.user}</dd>
            <dd>
              <CopyButton text={creds.user} label="Copiar usuário" />
            </dd>
            <dt className="text-fg-muted">Senha</dt>
            <dd className="selectable font-mono text-fg">
              {creds.password === '' ? <span className="text-fg-faint">(vazia)</span> : creds.password}
            </dd>
            <dd>
              <CopyButton text={creds.password} label="Copiar senha" />
            </dd>
            <dt className="text-fg-muted">Conexão</dt>
            <dd className="selectable break-all font-mono text-fg">{connCmd}</dd>
            <dd>
              <CopyButton text={connCmd} label="Copiar comando de conexão" />
            </dd>
          </dl>
        )}
      </Card>

      <Card
        title="Databases"
        actions={
          <Button
            variant="secondary"
            size="sm"
            icon={<Plus size={14} />}
            disabled={state !== 'ready' || busy}
            onClick={() => setCreating(true)}
          >
            Criar
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
              placeholder="nome_do_banco"
              aria-label="Nome do novo database"
              className="selectable h-9 w-64 rounded-pill border border-border bg-bg-card px-3 font-mono text-sm text-fg outline-none focus-visible:outline-2 focus-visible:outline-accent"
            />
            <Button type="submit" variant="primary" size="sm" loading={busy} disabled={busy}>
              Criar
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
              Cancelar
            </Button>
          </form>
        )}
        {state !== 'ready' ? (
          <EmptyState
            icon={<DatabaseIcon size={24} />}
            title="MySQL não está pronto"
            description="Inicie o serviço para listar os databases."
            action={
              <Button variant="secondary" size="sm" onClick={() => onNavigate('services')}>
                Ir para Serviços
              </Button>
            }
          />
        ) : dbs === null ? (
          <Skeleton lines={5} />
        ) : dbs.length === 0 ? (
          <EmptyState title="Nenhum database de usuário" description="Crie o primeiro com o botão acima." />
        ) : (
          <table className="w-full text-sm">
            <thead className="text-left text-xs text-fg-faint">
              <tr>
                <th className="py-1 font-normal">Nome</th>
                <th className="py-1 text-right font-normal">Tamanho</th>
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
                          <span className="text-xs text-fg-muted">Excluir {db.name}?</span>
                          <Button
                            variant="danger"
                            size="sm"
                            loading={busy}
                            disabled={busy}
                            onClick={() => void drop(db.name)}
                          >
                            Excluir
                          </Button>
                          <Button variant="ghost" size="sm" onClick={() => setConfirmDrop(null)}>
                            Cancelar
                          </Button>
                        </>
                      ) : (
                        <Button
                          variant="ghost"
                          size="sm"
                          aria-label={`Excluir ${db.name}`}
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
