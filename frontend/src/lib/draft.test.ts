import { describe, expect, it } from 'vitest'
import { mergeDraft } from './draft'

type S = { porta: number; editor: string; recolhida: boolean; raizes: string[] }

const base: S = { porta: 80, editor: 'code', recolhida: false, raizes: ['C:/a'] }

describe('mergeDraft', () => {
  it('mantém o campo editado e aceita o resto do state novo', () => {
    const draft = { ...base, porta: 8080 }
    const next = { ...base, recolhida: true, raizes: ['C:/a', 'C:/b'] }
    expect(mergeDraft(draft, base, next)).toEqual({ porta: 8080, editor: 'code', recolhida: true, raizes: ['C:/a', 'C:/b'] })
  })

  it('array igual por valor, mas outra instância, não conta como edição', () => {
    const draft = { ...base, raizes: [...base.raizes] }
    const next = { ...base, raizes: ['C:/b'] }
    expect(mergeDraft(draft, base, next).raizes).toEqual(['C:/b'])
  })

  it('array editado pelo usuário vence o do backend', () => {
    const draft = { ...base, raizes: ['C:/x'] }
    const next = { ...base, raizes: ['C:/b'] }
    expect(mergeDraft(draft, base, next).raizes).toEqual(['C:/x'])
  })

  it('sem edição, o rascunho vira o state novo', () => {
    const next = { ...base, editor: 'vim' }
    expect(mergeDraft({ ...base }, base, next)).toEqual(next)
  })
})
