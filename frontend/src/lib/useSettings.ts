import { useCallback, useEffect, useState } from 'react'
import { SettingsService } from '../../bindings/hyphp/services'
import { EVENTS, useEvent } from './events'
import type { State } from './types'

export function useSettings() {
  const [settings, setSettings] = useState<State | null>(null)

  useEffect(() => {
    void SettingsService.Get().then(setSettings)
  }, [])

  useEvent<State>(EVENTS.settingsChanged, setSettings)

  const save = useCallback(async (next: State) => {
    await SettingsService.Set(next)
    setSettings(next)
  }, [])

  return { settings, save }
}
