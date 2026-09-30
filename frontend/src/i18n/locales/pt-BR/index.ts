// Catálogo de referência da interface. Toda chave nasce aqui; os outros
// idiomas são tipados contra ele (typeof), e o tsc aponta chave faltando ou
// sobrando.
import { common } from './common'
import { app } from './app'
import { dashboard } from './dashboard'
import { projects } from './projects'
import { services } from './services'
import { runtimes } from './runtimes'
import { phpini } from './phpini'
import { database } from './database'
import { mail } from './mail'
import { logs } from './logs'
import { settings } from './settings'
import { cli } from './cli'

export const ptBR = { common, app, dashboard, projects, services, runtimes, phpini, database, mail, logs, cli, settings }
