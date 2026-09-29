export type Vars = Record<string, string | number>

/**
 * Troca `{nome}` por `vars.nome`. Um marcador sem valor fica como está: um
 * texto quebrado na tela é achado na hora, um texto sumido não.
 */
export function interpolate(template: string, vars?: Vars): string {
  if (!vars) return template
  return template.replace(/\{(\w+)\}/g, (marcador, nome: string) => (nome in vars ? String(vars[nome]) : marcador))
}
