import { createContext, useContext, useEffect, useState, type ReactNode } from 'react'
import { AppService } from '../../bindings/hyphp/services'
import { EVENTS, useEvent } from './events'
import type { Platform } from './types'

export type OS = 'windows' | 'darwin' | 'linux'

const PlatformContext = createContext<Platform>({ os: 'windows', tray: true, dark: true })

/**
 * Sistema, bandeja e tema do sistema, vindos do Go e relidos a cada
 * platform:changed. Nada é renderizado antes da primeira resposta: um texto
 * que depende do sistema apareceria primeiro no de outro.
 */
export function PlatformProvider({ children }: { children: ReactNode }) {
  const [platform, setPlatform] = useState<Platform | null>(null)

  // Um platform:changed que chegue antes da primeira resposta é mais novo que
  // ela e fica valendo.
  useEffect(() => {
    void AppService.Platform().then((p) => setPlatform((current) => current ?? p))
  }, [])

  useEvent<Platform>(EVENTS.platformChanged, setPlatform)

  if (!platform) return null
  return <PlatformContext.Provider value={platform}>{children}</PlatformContext.Provider>
}

export function usePlatform(): Platform {
  return useContext(PlatformContext)
}

/** O valor do sistema em uso; `os` é o `runtime.GOOS` do Go. */
export function byOS<const T>(os: string, values: Record<OS, T>): T {
  return os === 'darwin' || os === 'linux' ? values[os] : values.windows
}
