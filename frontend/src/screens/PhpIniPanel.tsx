import { useCallback, useEffect, useState } from 'react'
import { ArrowCounterClockwise } from '@phosphor-icons/react'
import { RuntimesService } from '../../bindings/hyphp/services'
import type { IniSetting } from '../../bindings/hyphp/services'
import { Badge } from '../components/Badge'
import { Button } from '../components/Button'
import { SectionLabel } from '../components/SectionLabel'
import { Skeleton } from '../components/Skeleton'
import { errorText } from '../lib/errors'
import { useT } from '../i18n'

const inputClass =
  'selectable h-7 rounded-pill border border-border bg-bg-card px-3 font-mono text-xs text-fg outline-none focus-visible:outline-2 focus-visible:outline-accent'

/**
 * Diretivas de php.ini da série. A lista vem do Go (curadas + as do usuário) e
 * é relida depois de cada gravação: o valor efetivo e a origem mudam juntos, e
 * quem decide o padrão (HyPHP ou builtin do PHP) é o serviço.
 */
export function PhpIniPanel({ major }: { major: string }) {
  const t = useT('phpini')
  const [rows, setRows] = useState<IniSetting[] | null>(null)
  // rascunho por diretiva; ausente = o input mostra o valor efetivo.
  const [drafts, setDrafts] = useState<Record<string, string>>({})
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [newName, setNewName] = useState('')
  const [newValue, setNewValue] = useState('')

  // `isCurrent` deixa o efeito descartar a resposta que chega depois de fechar
  // o painel/desmontar o card; quem chama de dentro de `write` já está montado.
  const load = useCallback(
    async (isCurrent: () => boolean = () => true) => {
      const list = await RuntimesService.IniSettings(major)
      if (isCurrent()) setRows(list ?? [])
    },
    [major],
  )

  useEffect(() => {
    let cancelled = false
    load(() => !cancelled).catch((e: unknown) => {
      if (!cancelled) setError(errorText(e))
    })
    return () => {
      cancelled = true
    }
  }, [load])

  // write grava pelo serviço, relê a lista e descarta o rascunho da diretiva
  // gravada; em erro o rascunho fica no input para o usuário corrigir.
  const write = async (name: string, fn: () => Promise<unknown>) => {
    setBusy(true)
    setError(null)
    try {
      await fn()
      await load()
      setDrafts((prev) => {
        const { [name]: _, ...rest } = prev
        return rest
      })
      return true
    } catch (e) {
      setError(errorText(e))
      return false
    } finally {
      setBusy(false)
    }
  }

  const save = async (row: IniSetting) => {
    const value = drafts[row.name]
    if (value === undefined || value === row.value) return
    await write(row.name, () => RuntimesService.SetIniSetting(major, row.name, value))
  }

  const add = async () => {
    const name = newName.trim()
    if (await write(name, () => RuntimesService.SetIniSetting(major, name, newValue))) {
      setNewName('')
      setNewValue('')
    }
  }

  return (
    <div className="mt-4 border-t border-border pt-3">
      <SectionLabel>{t('label')}</SectionLabel>
      <p className="mt-1 text-xs text-fg-faint">{t('hint', { major })}</p>
      {error && <div className="selectable mt-2 text-sm text-err">{error}</div>}
      {rows === null ? (
        <Skeleton lines={4} className="mt-2" />
      ) : (
        <ul className="mt-2 flex flex-col">
          {rows.map((row) => {
            const draft = drafts[row.name]
            const dirty = draft !== undefined && draft !== row.value
            return (
              <li key={row.name}>
                <form
                  className="flex items-center gap-3 py-1 text-sm"
                  onSubmit={(e) => {
                    e.preventDefault()
                    void save(row)
                  }}
                >
                  <span className="selectable w-60 truncate font-mono text-fg" title={row.name}>
                    {row.name}
                  </span>
                  <input
                    value={draft ?? row.value}
                    aria-label={t('valueAria', { name: row.name })}
                    disabled={busy}
                    onChange={(e) => setDrafts((prev) => ({ ...prev, [row.name]: e.target.value }))}
                    className={`${inputClass} w-44`}
                  />
                  {row.source === 'user' ? (
                    <Badge tone="accent">{t('sourceUser')}</Badge>
                  ) : (
                    <Badge>{row.source === 'hyphp' ? t('sourceHyphp') : t('sourcePhp')}</Badge>
                  )}
                  {row.source === 'user' && (
                    <span className="selectable font-mono text-xs text-fg-faint">
                      {t('defaultIs', { value: row.defaultValue === '' ? t('emptyValue') : row.defaultValue })}
                    </span>
                  )}
                  <div className="ml-auto flex items-center gap-1">
                    {dirty && (
                      <Button type="submit" variant="primary" size="sm" disabled={busy}>
                        {t('save')}
                      </Button>
                    )}
                    {row.source === 'user' && (
                      <Button
                        variant="ghost"
                        size="sm"
                        icon={<ArrowCounterClockwise size={14} />}
                        disabled={busy}
                        onClick={() => void write(row.name, () => RuntimesService.ResetIniSetting(major, row.name))}
                      >
                        {t('reset')}
                      </Button>
                    )}
                  </div>
                </form>
              </li>
            )
          })}
        </ul>
      )}
      <form
        className="mt-3 flex items-center gap-3 border-t border-border pt-3 text-sm"
        onSubmit={(e) => {
          e.preventDefault()
          void add()
        }}
      >
        <span className="w-60 text-xs text-fg-muted">{t('addLabel')}</span>
        <input
          value={newName}
          placeholder={t('namePlaceholder')}
          aria-label={t('newNameAria')}
          disabled={busy}
          onChange={(e) => setNewName(e.target.value)}
          className={`${inputClass} w-44`}
        />
        <input
          value={newValue}
          placeholder={t('valuePlaceholder')}
          aria-label={t('newValueAria')}
          disabled={busy}
          onChange={(e) => setNewValue(e.target.value)}
          className={`${inputClass} w-44`}
        />
        <Button
          type="submit"
          variant="secondary"
          size="sm"
          loading={busy}
          disabled={busy || newName.trim() === '' || newValue.trim() === ''}
          className="ml-auto"
        >
          {t('add')}
        </Button>
      </form>
    </div>
  )
}
