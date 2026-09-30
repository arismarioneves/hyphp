import { useCallback, useEffect, useState } from 'react'
import { ServicesService } from '../../bindings/hyphp/services'
import { EVENTS, useEvent } from './events'
import { upsertStatus } from './status'
import type { ServiceStatus } from './types'

export function useServices() {
  const [services, setServices] = useState<ServiceStatus[]>([])
  const [loading, setLoading] = useState(true)

  const refresh = useCallback(async () => {
    const list = await ServicesService.List()
    setServices(list ?? [])
    setLoading(false)
  }, [])

  useEffect(() => {
    void refresh()
  }, [refresh])

  useEvent<ServiceStatus>(EVENTS.serviceState, (s) => {
    setServices((prev) => upsertStatus(prev, s))
  })
  // Sem isto, um serviço removido (pool de uma série que nenhum projeto usa
  // mais) ficava na lista como "stopped" e deixava o resumo amarelo.
  useEvent<string>(EVENTS.serviceRemoved, (id) => {
    setServices((prev) => prev.filter((s) => s.id !== id))
  })

  return { services, loading, refresh }
}
