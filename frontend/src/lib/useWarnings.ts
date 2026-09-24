import { useEffect, useState } from 'react'
import { SettingsService } from '../../bindings/hyphp/services'
import { EVENTS, useEvent } from './events'
import type { Warning } from './types'

export function useWarnings() {
  const [warnings, setWarnings] = useState<Warning[]>([])

  useEffect(() => {
    void SettingsService.Warnings().then((w) => setWarnings(w ?? []))
  }, [])

  useEvent<Warning[]>(EVENTS.stackWarnings, (w) => setWarnings(w ?? []))

  return { warnings }
}
