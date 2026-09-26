// As chamadas de binding rejeitam com Error cuja mensagem é o erro do Go.
export function errorText(e: unknown): string {
  return e instanceof Error ? e.message : String(e)
}
