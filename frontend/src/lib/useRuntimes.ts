import { useCallback, useEffect, useState } from 'react'
import { RuntimesService } from '../../bindings/hyphp/services'
import { EVENTS, useEvent } from './events'
import type { BrewStatus, Installed, Package, Progress } from './types'

// Antes da primeira resposta assume Windows (supported=false): a UI do
// Homebrew só aparece quando o Go confirma a plataforma, sem piscar no Windows.
const NO_BREW: BrewStatus = { supported: false, found: false, prefix: '', installCommand: '' }

export function useRuntimes() {
  const [installed, setInstalled] = useState<Installed[]>([])
  const [available, setAvailable] = useState<Package[]>([])
  const [progress, setProgress] = useState<Record<string, Progress>>({})
  const [loading, setLoading] = useState(true)
  const [homebrew, setHomebrew] = useState<BrewStatus>(NO_BREW)

  const refresh = useCallback(async () => {
    const [inst, avail, brew] = await Promise.all([
      RuntimesService.Installed(),
      RuntimesService.Available(),
      RuntimesService.Homebrew(),
    ])
    setInstalled(inst ?? [])
    setAvailable(avail ?? [])
    setHomebrew(brew)
    setLoading(false)
  }, [])

  // Rescan relocaliza o Homebrew no Go; a lista chega por runtime:changed, mas
  // o status do brew e os disponíveis (fórmulas) só vêm relendo.
  const rescan = useCallback(async () => {
    await RuntimesService.Rescan()
    await refresh()
  }, [refresh])

  useEffect(() => {
    void refresh()
  }, [refresh])

  useEvent<Installed[]>(EVENTS.runtimeChanged, (list) => {
    setInstalled(list ?? [])
    // o brew pode ter sido instalado/removido entre varreduras: relê o status.
    void RuntimesService.Homebrew().then(setHomebrew)
    // No Mac a fórmula disponível tem a série ("8.5") e o keg instalado a versão
    // completa ("8.5.1"): o filtro por versão da tela não a esconde. Quem exclui
    // os kegs instalados é o Available() do Go, então relê a cada mudança.
    void RuntimesService.Available().then((avail) => setAvailable(avail ?? []))
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

  return { installed, available, progress, loading, homebrew, refresh, rescan }
}
