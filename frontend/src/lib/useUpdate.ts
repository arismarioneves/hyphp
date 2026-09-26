import { useEffect, useState } from 'react'
import { UpdateService } from '../../bindings/hyphp/services'
import { errorText } from './errors'
import { EVENTS, useEvent } from './events'
import type { UpdateStatus } from './types'

export function useUpdate() {
  const [status, setStatus] = useState<UpdateStatus | null>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    void UpdateService.Status().then(setStatus)
  }, [])

  useEvent<UpdateStatus>(EVENTS.updateStatus, setStatus)

  // Check inclui o download quando há versão nova; o progresso chega pelo
  // evento enquanto a promessa não resolve.
  const check = async () => {
    setBusy(true)
    setError(null)
    try {
      await UpdateService.Check()
    } catch (e) {
      setError(errorText(e))
    } finally {
      setBusy(false)
    }
  }

  // No sucesso o app fecha; só a falha volta para a tela.
  const apply = async () => {
    setBusy(true)
    setError(null)
    try {
      await UpdateService.Apply()
    } catch (e) {
      setError(errorText(e))
      setBusy(false)
    }
  }

  return { status, busy, error, check, apply }
}
