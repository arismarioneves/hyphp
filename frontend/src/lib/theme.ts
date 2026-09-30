import { useEffect } from 'react'

export type ResolvedTheme = 'dark' | 'light'

/** "" é o state.json anterior à v3, quando só existia o escuro. */
export function resolveTheme(preference: string, systemLight: boolean): ResolvedTheme {
  if (preference === 'light') return 'light'
  if (preference === 'system') return systemLight ? 'light' : 'dark'
  return 'dark'
}

/**
 * Aplica o tema em `<html data-theme>`, que troca as variáveis de
 * `styles/tokens.css`. Em "system" acompanha o Windows ao vivo: o WebView2
 * repassa a troca de tema do sistema para `prefers-color-scheme`.
 */
export function useApplyTheme(preference: string | undefined) {
  useEffect(() => {
    if (preference === undefined) return
    const media = window.matchMedia('(prefers-color-scheme: light)')
    const apply = () => {
      const theme = resolveTheme(preference, media.matches)
      document.documentElement.dataset.theme = theme
      document.documentElement.style.colorScheme = theme
    }
    apply()
    if (preference !== 'system') return
    media.addEventListener('change', apply)
    return () => media.removeEventListener('change', apply)
  }, [preference])
}
