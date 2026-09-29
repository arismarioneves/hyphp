// Ponto único de importação dos modelos gerados pelo wails3.
// Se `wails3 generate bindings -ts` mudar o layout, ajuste só aqui.
import type { Status } from '../../bindings/hyphp/internal/supervisor/models'
import type { Project as GeneratedProject } from '../../bindings/hyphp/internal/project/models'
import type { Installed as GeneratedInstalled, Extension as GeneratedExtension } from '../../bindings/hyphp/internal/runtime/models'
import type { Package as GeneratedPackage } from '../../bindings/hyphp/internal/pkgmgr/models'
import type { State as GeneratedState } from '../../bindings/hyphp/internal/state/models'
import type { Warning as GeneratedWarning } from '../../bindings/hyphp/internal/stack/models'
import type { Status as GeneratedUpdateStatus } from '../../bindings/hyphp/internal/update/models'
import type {
  DBInfo as GeneratedDBInfo,
  LogSource as GeneratedLogSource,
  Credentials as GeneratedCredentials,
} from '../../bindings/hyphp/services/models'

export type ServiceStatus = Status
export type Project = GeneratedProject
export type Installed = GeneratedInstalled
export type Extension = GeneratedExtension
export type Package = GeneratedPackage
export type State = GeneratedState
export type Warning = GeneratedWarning
export type UpdateStatus = GeneratedUpdateStatus
export type DBInfo = GeneratedDBInfo
export type LogSource = GeneratedLogSource
export type Credentials = GeneratedCredentials

export type ProgressPhase = 'download' | 'verify' | 'extract' | 'done' | 'error' | 'canceled'

/**
 * Espelho de `pkgmgr.Progress` (payload de `download:progress`).
 * Escrito à mão porque o gerador só emite modelos citados em assinatura de
 * método bindado, e Progress só trafega por evento.
 */
export type Progress = {
  packageId: string
  done: number
  /** -1 quando o servidor não informa Content-Length */
  total: number
  phase: ProgressPhase
  error: string
}

export type RuntimeKind = 'php' | 'apache' | 'nginx' | 'mysql' | 'mailpit' | 'mkcert' | 'phpmyadmin'
export type WebServerName = 'apache' | 'nginx'
