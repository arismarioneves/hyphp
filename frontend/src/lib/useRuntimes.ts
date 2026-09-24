import { useCallback, useEffect, useState } from 'react'
import { RuntimesService } from '../../bindings/hyphp/services'
import { EVENTS, useEvent } from './events'
import type { Installed, Package, Progress } from './types'

export function useRuntimes() {
  const [installed, setInstalled] = useState<Installed[]>([])
  const [available, setAvailable] = useState<Package[]>([])
  const [progress, setProgress] = useState<Record<string, Progress>>({})
  const [loading, setLoading] = useState(true)

  const refresh = useCallback(async () => {
    const [inst, avail] = await Promise.all([RuntimesService.Installed(), RuntimesService.Available()])
    setInstalled(inst ?? [])
    setAvailable(avail ?? [])
    setLoading(false)
  }, [])

  useEffect(() => {
    void refresh()
  }, [refresh])

  useEvent<Installed[]>(EVENTS.runtimeChanged, (list) => {
    setInstalled(list ?? [])
    // instalação concluída já refletiu em `installed`; limpa barras "done"
    setProgress((prev) => {
      const next: Record<string, Progress> = {}
      for (const [id, p] of Object.entries(prev)) if (p.phase !== 'done') next[id] = p
      return next
    })
  })

  useEvent<Progress>(EVENTS.downloadProgress, (p) => {
    setProgress((prev) => ({ ...prev, [p.packageId]: p }))
  })

  return { installed, available, progress, loading, refresh }
}
