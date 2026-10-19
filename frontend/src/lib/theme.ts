import { useEffect } from 'react'
import type { Platform } from './types'

export type ResolvedTheme = 'dark' | 'light'

/** "" é o state.json anterior à v3, quando só existia o escuro. */
export function resolveTheme(preference: string, systemLight: boolean): ResolvedTheme {
  if (preference === 'light') return 'light'
  if (preference === 'system') return systemLight ? 'light' : 'dark'
  return 'dark'
}

/**
 * Aplica o tema em `<html data-theme>`, que troca as variáveis de
 * `styles/tokens.css`. Em "system" acompanha o sistema ao vivo: o WebView2 e o
 * WKWebView repassam a troca de tema para `prefers-color-scheme`; o WebKitGTK
 * não, e no Linux o tema do sistema vem do Go (`Platform.dark`).
 */
export function useApplyTheme(preference: string | undefined, platform: Platform) {
  const goDark = platform.os === 'linux' ? platform.dark : undefined
  useEffect(() => {
    if (preference === undefined) return
    const media = window.matchMedia('(prefers-color-scheme: light)')
    const apply = () => {
      const theme = resolveTheme(preference, goDark === undefined ? media.matches : !goDark)
      document.documentElement.dataset.theme = theme
      document.documentElement.style.colorScheme = theme
    }
    apply()
    if (preference !== 'system' || goDark !== undefined) return
    media.addEventListener('change', apply)
    return () => media.removeEventListener('change', apply)
  }, [preference, goDark])
}
