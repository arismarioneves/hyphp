import type { ptBR } from './locales/pt-BR'

/** Formato de todo catálogo: o do pt-BR, que é a referência. */
export type Messages = typeof ptBR
export type Namespace = keyof Messages

/** Chaves `x_one`/`x_other` de um namespace, sem o sufixo: as que aceitam plural. */
export type PluralBase<T> = { [K in keyof T]: K extends `${infer B}_other` ? B : never }[keyof T]
