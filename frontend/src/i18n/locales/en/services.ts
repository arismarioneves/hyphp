import type { services as base } from '../pt-BR/services'

export const services: typeof base = {
  groupDb: 'DATABASE',
  groupProc: 'PROJECT PROCESSES',
  groupWeb: 'WEB SERVER',
  groupMail: 'MAIL',
  uptime: 'Uptime',
  restarts: 'Restarts',
  noServicesTitle: 'No services',
  noServicesDesc: 'Install a web server and PHP in Runtimes and add a project.',
  openRuntimes: 'Open Runtimes',
  service: 'Service',
  port: 'Port',
  actions: 'Actions',
  startName: 'Start {name}',
  stopName: 'Stop {name}',
  restartName: 'Restart {name}',
  logOf: 'Log for {name}',
  closeLog: 'Close log',
}
