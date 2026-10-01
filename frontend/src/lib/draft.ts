/**
 * Rebaseia um rascunho sobre um state novo vindo do backend: os campos que o
 * usuário editou (diferentes do `synced`, o state em que o rascunho se
 * baseou) são mantidos; o resto acompanha `next`.
 */
export function mergeDraft<T extends object>(draft: T, synced: T, next: T): T {
  const merged = { ...next }
  for (const k of Object.keys(draft) as (keyof T)[]) {
    // Comparação por valor: arrays e objetos do state chegam como instâncias
    // novas a cada leitura do Go, e `!==` acusaria edição em todo campo não primitivo.
    if (draft[k] !== synced[k] && JSON.stringify(draft[k]) !== JSON.stringify(synced[k])) merged[k] = draft[k]
  }
  return merged
}
