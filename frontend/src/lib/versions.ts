import type { Installed } from './types'

/**
 * Ordena da versão mais nova para a mais antiga comparando segmento a segmento:
 * a ordem lexicográfica colocaria "8.1.9" acima de "8.1.10".
 */
export function compareVersionDesc(a: string, b: string): number {
  const left = a.split('.')
  const right = b.split('.')
  const len = Math.max(left.length, right.length)
  for (let i = 0; i < len; i++) {
    const na = Number.parseInt(left[i] || '0', 10) || 0
    const nb = Number.parseInt(right[i] || '0', 10) || 0
    if (na !== nb) return nb - na
  }
  return 0
}

/**
 * Maior versão instalada por tipo: é a que a stack sobe para o que roda uma
 * instância só (runtime.Newest no Go).
 */
export function newestVersions(installed: Installed[]): Record<string, string> {
  const versions: Record<string, string> = {}
  for (const i of installed) {
    // `Installed.kind` é o enum gerado `runtime.Kind` (nominal).
    const kind = i.kind as string
    const cur = versions[kind]
    if (cur === undefined || compareVersionDesc(cur, i.version) > 0) versions[kind] = i.version
  }
  return versions
}
