import { useEffect, useMemo, useState } from 'react'
import { Scroll } from '@phosphor-icons/react'
import { LogsService } from '../../bindings/hyphp/services'
import { Button } from '../components/Button'
import { EmptyState } from '../components/EmptyState'
import { LogView } from '../components/LogView'
import { Select, type SelectGroup } from '../components/Select'
import { Skeleton } from '../components/Skeleton'
import { EVENTS, useEvent } from '../lib/events'
import type { ScreenProps } from '../lib/screens'
import type { LogSource } from '../lib/types'

const GROUP_LABELS: Record<string, string> = {
  web: 'Web server',
  php: 'PHP',
  db: 'Banco de dados',
  mail: 'Mail',
  proc: 'Processos de projeto',
}
const GROUP_ORDER = ['web', 'php', 'db', 'mail', 'proc']

function toGroups(sources: LogSource[]) {
  // Map e não Record: o agrupamento é dinâmico e a ordem sai do GROUP_ORDER.
  const map = new Map<string, LogSource[]>()
  for (const s of sources) {
    const arr = map.get(s.group) ?? []
    arr.push(s)
    map.set(s.group, arr)
  }
  const groups: SelectGroup[] = [...map.entries()]
    .sort(([a], [b]) => GROUP_ORDER.indexOf(a) - GROUP_ORDER.indexOf(b))
    .map(([group, items]) => ({
      label: GROUP_LABELS[group] ?? group,
      options: items.map((s) => ({ value: s.id, label: s.name })),
    }))
  return groups
}

export function Logs({ onNavigate }: ScreenProps) {
  const [sources, setSources] = useState<LogSource[] | null>(null)
  const [selected, setSelected] = useState('')

  const loadSources = () => {
    void LogsService.Sources().then((list) => {
      const src = list ?? []
      setSources(src)
      // a seleção só muda se a fonte escolhida sumiu da lista.
      setSelected((cur) => (cur && src.some((s) => s.id === cur) ? cur : (src[0]?.id ?? '')))
    })
  }

  useEffect(loadSources, [])
  // specs novos (projeto adicionado, versão de PHP nova) mudam a lista de fontes.
  useEvent(EVENTS.serviceState, loadSources)
  useEvent(EVENTS.serviceRemoved, loadSources)

  const groups = useMemo(() => toGroups(sources ?? []), [sources])

  if (sources === null) return <Skeleton lines={6} />
  if (sources.length === 0) {
    return (
      <EmptyState
        icon={<Scroll size={24} />}
        title="Nenhuma fonte de log"
        description="Adicione um projeto ou instale um runtime."
        action={
          <Button variant="secondary" size="sm" onClick={() => onNavigate('projects')}>
            Ir para Projetos
          </Button>
        }
      />
    )
  }

  return (
    <div className="flex h-full min-h-0 flex-col gap-3">
      <div className="flex items-center gap-3">
        <span className="text-sm text-fg-muted">Fonte</span>
        <Select aria-label="Fonte do log" value={selected} groups={groups} onChange={setSelected} className="w-72" />
      </div>
      {selected && <LogView id={selected} className="min-h-0 flex-1" />}
    </div>
  )
}
