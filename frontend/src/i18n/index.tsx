import { createContext, useContext, useEffect, useMemo, useState, type ReactNode } from 'react'
import { SettingsService } from '../../bindings/hyphp/services'
import { useSettings } from '../lib/useSettings'
import { interpolate, type Vars } from './format'
import { en } from './locales/en'
import { ptBR } from './locales/pt-BR'
import type { Messages, Namespace, PluralBase } from './types'

/**
 * Idiomas da interface. Para acrescentar um: copiar `locales/en` para
 * `locales/<código>`, traduzir, e registrar aqui e em `internal/i18n` (Go).
 * O tsc aponta qualquer chave que faltar.
 */
export const LANGS = [
  { code: 'pt-BR', label: 'Português', messages: ptBR },
  { code: 'en', label: 'English', messages: en },
] as const satisfies ReadonlyArray<{ code: string; label: string; messages: Messages }>

export type Lang = (typeof LANGS)[number]['code']

function isLang(code: string): code is Lang {
  return LANGS.some((l) => l.code === code)
}

/** Preferência gravada, se for um idioma conhecido; senão o do Windows. */
export function resolveLang(preference: string, system: string): Lang {
  if (isLang(preference)) return preference
  return isLang(system) ? system : 'en'
}

type Ctx = { lang: Lang; system: Lang; messages: Messages }

const I18nContext = createContext<Ctx>({ lang: 'pt-BR', system: 'pt-BR', messages: ptBR })

/**
 * Escolhe o idioma pela preferência de Configurações ou, sem ela, pelo que o
 * Go diz ser o do Windows — o mesmo critério dos textos que o Go desenha
 * (tray, avisos). Nada é renderizado antes de saber o idioma: a janela
 * piscaria em português antes de trocar para inglês.
 */
export function I18nProvider({ children }: { children: ReactNode }) {
  const { settings } = useSettings()
  const [system, setSystem] = useState<string | null>(null)

  useEffect(() => {
    void SettingsService.SystemLanguage().then(setSystem)
  }, [])

  const value = useMemo<Ctx | null>(() => {
    if (!settings || system === null) return null
    const lang = resolveLang(settings.language, system)
    const messages = LANGS.find((l) => l.code === lang)?.messages ?? en
    return { lang, system: resolveLang('', system), messages }
  }, [settings, system])

  useEffect(() => {
    if (value) document.documentElement.lang = value.lang
  }, [value])

  if (!value) return null
  return <I18nContext.Provider value={value}>{children}</I18nContext.Provider>
}

/** Idioma atual e o do Windows, para Intl (datas, números) e para Configurações. */
export function useLang(): { lang: Lang; system: Lang } {
  const { lang, system } = useContext(I18nContext)
  return { lang, system }
}

/**
 * Tradutor de um namespace: `t('chave', { nome })` e
 * `t.plural('chave', n)`, que escolhe entre `chave_one` e `chave_other` pelas
 * regras de plural do idioma e oferece `{count}` ao texto.
 */
export function useT<N extends Namespace>(ns: N) {
  const { lang, messages } = useContext(I18nContext)
  return useMemo(() => {
    const dict = messages[ns] as Record<string, string>
    const rules = new Intl.PluralRules(lang)
    const t = (key: keyof Messages[N] & string, vars?: Vars) => interpolate(dict[key] ?? key, vars)
    const plural = (key: PluralBase<Messages[N]> & string, count: number, vars?: Vars) => {
      const exact = `${key}_${rules.select(count)}`
      const chosen = dict[exact] !== undefined ? exact : `${key}_other`
      return interpolate(dict[chosen] ?? key, { count, ...vars })
    }
    return Object.assign(t, { plural })
  }, [lang, messages, ns])
}
