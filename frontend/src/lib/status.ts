import type { ServiceState } from '../components/StatusDot'
import type { ServiceStatus } from './types'

/** Estado agregado de um conjunto de serviços (projeto, pool, sidebar). */
export function aggregateState(states: ServiceState[]): ServiceState {
  if (states.length === 0) return 'stopped'
  if (states.includes('failed')) return 'failed'
  if (states.includes('starting')) return 'starting'
  if (states.includes('stopping')) return 'stopping'
  if (states.includes('degraded')) return 'degraded'
  if (states.every((s) => s === 'ready')) return 'ready'
  if (states.every((s) => s === 'stopped')) return 'stopped'
  return 'degraded'
}

function compareStatus(a: ServiceStatus, b: ServiceStatus): number {
  if (a.group !== b.group) return a.group < b.group ? -1 : 1
  if (a.id === b.id) return 0
  return a.id < b.id ? -1 : 1
}

/** Aplica um `service:state` à lista, preservando a ordem do Go (group, id). */
export function upsertStatus(list: ServiceStatus[], s: ServiceStatus): ServiceStatus[] {
  const idx = list.findIndex((x) => x.id === s.id)
  const next = idx === -1 ? [...list, s] : list.map((x, i) => (i === idx ? s : x))
  return next.sort(compareStatus)
}

export type PhpPool = { major: string; ready: number; total: number; state: ServiceState }

/** Agrupa specs `php:<major>:<i>` por major. Ordenado por major. */
export function phpPools(services: ServiceStatus[]): PhpPool[] {
  const byMajor = new Map<string, ServiceStatus[]>()
  for (const s of services) {
    const parts = s.id.split(':')
    if (parts[0] !== 'php' || parts.length !== 3) continue
    const arr = byMajor.get(parts[1]) ?? []
    arr.push(s)
    byMajor.set(parts[1], arr)
  }
  return [...byMajor.entries()]
    .sort(([a], [b]) => (a < b ? -1 : a > b ? 1 : 0))
    .map(([major, workers]) => ({
      major,
      ready: workers.filter((w) => w.state === 'ready').length,
      total: workers.length,
      state: aggregateState(workers.map((w) => w.state as ServiceState)),
    }))
}

const ZERO_TIME_PREFIX = '0001-01-01'

/** Uptime humano a partir de `startedAt` (RFC3339 vindo do Go). */
export function formatUptime(startedAt: unknown, nowMs: number): string {
  if (typeof startedAt !== 'string' || startedAt.startsWith(ZERO_TIME_PREFIX)) return '—'
  const started = Date.parse(startedAt)
  if (Number.isNaN(started)) return '—'
  const total = Math.max(0, Math.floor((nowMs - started) / 1000))
  const days = Math.floor(total / 86400)
  const hours = Math.floor((total % 86400) / 3600)
  const minutes = Math.floor((total % 3600) / 60)
  const seconds = total % 60
  const pad = (n: number) => String(n).padStart(2, '0')
  if (days > 0) return `${days}d ${pad(hours)}h`
  if (hours > 0) return `${hours}h ${pad(minutes)}m`
  if (minutes > 0) return `${minutes}m ${pad(seconds)}s`
  return `${seconds}s`
}

export type Summary = { ready: number; degraded: number; total: number }

/** Resumo para o rodapé da sidebar. `degraded` inclui `failed`. */
export function summarize(services: ServiceStatus[]): Summary {
  let ready = 0
  let degraded = 0
  for (const s of services) {
    if (s.state === 'ready') ready++
    else if (s.state === 'degraded' || s.state === 'failed') degraded++
  }
  return { ready, degraded, total: services.length }
}
