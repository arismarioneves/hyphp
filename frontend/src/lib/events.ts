import { useEffect, useRef } from 'react'
import { Events } from '@wailsio/runtime'

// Nomes exatos do contrato C12.
export const EVENTS = {
  serviceState: 'service:state',
  serviceLog: 'service:log',
  projectChanged: 'project:changed',
  runtimeChanged: 'runtime:changed',
  downloadProgress: 'download:progress',
  stackWarnings: 'stack:warnings',
  settingsChanged: 'settings:changed',
  updateStatus: 'update:status',
} as const

export type EventName = (typeof EVENTS)[keyof typeof EVENTS]

/**
 * Assina um evento Go→UI e cancela ao desmontar.
 * O callback mais recente é sempre chamado, sem re-assinar a cada render.
 */
export function useEvent<T>(name: EventName, cb: (data: T) => void): void {
  const cbRef = useRef(cb)
  cbRef.current = cb
  useEffect(() => {
    const off = Events.On(name, (event) => {
      cbRef.current(event.data as T)
    })
    return off
  }, [name])
}
