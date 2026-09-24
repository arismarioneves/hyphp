import { describe, expect, it } from 'vitest'
import type { ServiceState } from '../components/StatusDot'
import { aggregateState, formatUptime, phpPools, summarize, upsertStatus } from './status'
import type { ServiceStatus } from './types'

// `state` entra como ServiceState (união literal) porque o modelo gerado usa o
// enum supervisor.State, ao qual literais não são atribuíveis — só comparáveis.
function svc(id: string, state: ServiceState, extra: Partial<ServiceStatus> = {}): ServiceStatus {
  return {
    id,
    name: id,
    group: id.split(':')[0],
    state,
    pid: 0,
    port: 0,
    startedAt: '0001-01-01T00:00:00Z',
    restarts: 0,
    lastError: '',
    ...extra,
  } as ServiceStatus
}

describe('aggregateState', () => {
  const cases: Array<{ name: string; states: ServiceState[]; want: ServiceState }> = [
    { name: 'vazio é stopped', states: [], want: 'stopped' },
    { name: 'todos ready', states: ['ready', 'ready'], want: 'ready' },
    { name: 'todos stopped', states: ['stopped', 'stopped'], want: 'stopped' },
    { name: 'failed vence tudo', states: ['ready', 'failed', 'starting'], want: 'failed' },
    { name: 'starting vence degraded', states: ['degraded', 'starting'], want: 'starting' },
    { name: 'stopping vence degraded', states: ['degraded', 'stopping'], want: 'stopping' },
    { name: 'mistura ready/stopped é degraded', states: ['ready', 'stopped'], want: 'degraded' },
  ]
  for (const c of cases) {
    it(c.name, () => expect(aggregateState(c.states)).toBe(c.want))
  }
})

describe('upsertStatus', () => {
  it('substitui por id e mantém ordem group,id', () => {
    const list = [svc('php:8.1:0', 'ready'), svc('web:apache', 'ready')]
    const out = upsertStatus(list, svc('web:apache', 'degraded'))
    expect(out.map((s) => `${s.id}=${s.state}`)).toEqual(['php:8.1:0=ready', 'web:apache=degraded'])
  })
  it('insere novo id na posição ordenada', () => {
    const list = [svc('php:8.1:0', 'ready'), svc('web:apache', 'ready')]
    const out = upsertStatus(list, svc('mysql', 'starting', { group: 'db' }))
    expect(out.map((s) => s.id)).toEqual(['mysql', 'php:8.1:0', 'web:apache'])
  })
})

describe('phpPools', () => {
  it('agrupa workers por major e conta ready/total', () => {
    const pools = phpPools([
      svc('php:8.1:0', 'ready'),
      svc('php:8.1:1', 'degraded'),
      svc('php:7.2:0', 'ready'),
      svc('web:apache', 'ready'),
    ])
    expect(pools).toEqual([
      { major: '7.2', ready: 1, total: 1, state: 'ready' },
      { major: '8.1', ready: 1, total: 2, state: 'degraded' },
    ])
  })
})

describe('formatUptime', () => {
  const now = Date.parse('2026-09-20T12:00:00Z')
  const cases: Array<{ name: string; startedAt: string; want: string }> = [
    { name: 'zero-time vira travessão', startedAt: '0001-01-01T00:00:00Z', want: '—' },
    { name: 'segundos', startedAt: '2026-09-20T11:59:57Z', want: '3s' },
    { name: 'minutos com segundos zero-padded', startedAt: '2026-09-20T11:57:55Z', want: '2m 05s' },
    { name: 'horas com minutos', startedAt: '2026-09-20T10:57:00Z', want: '1h 03m' },
    { name: 'dias com horas', startedAt: '2026-09-18T08:00:00Z', want: '2d 04h' },
    { name: 'futuro (relógio) não fica negativo', startedAt: '2026-09-20T12:00:05Z', want: '0s' },
  ]
  for (const c of cases) {
    it(c.name, () => expect(formatUptime(c.startedAt, now)).toBe(c.want))
  }
})

describe('summarize', () => {
  it('conta ready e degraded (degraded|failed)', () => {
    expect(summarize([svc('a', 'ready'), svc('b', 'degraded'), svc('c', 'failed'), svc('d', 'stopped')])).toEqual({
      ready: 1,
      degraded: 2,
      total: 4,
    })
  })
})
