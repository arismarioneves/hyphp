import { useEffect, useMemo, useRef, useState } from 'react'
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
import { useT } from '../i18n'
import type { logs } from '../i18n/locales/pt-BR/logs'

// guarda a chave do catálogo: o texto só pode ser resolvido dentro do componente.
const GROUP_LABELS: Record<string, keyof typeof logs> = {
  web: 'groupWeb',
  php: 'groupPhp',
  db: 'groupDb',
  mail: 'groupMail',
  proc: 'groupProc',
}
const GROUP_ORDER = ['web', 'php', 'db', 'mail', 'proc']

function toGroups(sources: LogSource[], t: (key: keyof typeof logs) => string) {
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
      label: GROUP_LABELS[group] ? t(GROUP_LABELS[group]) : group,
      options: items.map((s) => ({ value: s.id, label: s.name })),
    }))
  return groups
}

export function Logs({ onNavigate }: ScreenProps) {
  const t = useT('logs')
  const [sources, setSources] = useState<LogSource[] | null>(null)
  const [selected, setSelected] = useState('')

  // Cada service:state relê a lista; numa rajada (Iniciar tudo) as respostas
  // podem chegar fora de ordem, e só a da chamada mais recente vale.
  const lastCall = useRef(0)
  const loadSources = () => {
    const call = ++lastCall.current
    void LogsService.Sources().then((list) => {
      if (call !== lastCall.current) return
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

  const groups = useMemo(() => toGroups(sources ?? [], t), [sources, t])

  if (sources === null) return <Skeleton lines={6} />
  if (sources.length === 0) {
    return (
      <EmptyState
        icon={<Scroll size={24} />}
        title={t('noSourcesTitle')}
        description={t('noSourcesDescription')}
        action={
          <Button variant="secondary" size="sm" onClick={() => onNavigate('projects')}>
            {t('goToProjects')}
          </Button>
        }
      />
    )
  }

  return (
    <div className="flex h-full min-h-0 flex-col gap-3">
      <div className="flex items-center gap-3">
        <span className="text-sm text-fg-muted">{t('source')}</span>
        <Select aria-label={t('sourceAria')} value={selected} groups={groups} onChange={setSelected} className="w-72" />
      </div>
      {selected && <LogView id={selected} className="min-h-0 flex-1" />}
    </div>
  )
}
