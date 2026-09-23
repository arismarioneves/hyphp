export type Screen =
  | 'dashboard'
  | 'projects'
  | 'services'
  | 'runtimes'
  | 'database'
  | 'mail'
  | 'logs'
  | 'settings';

/** Ordem de exibição na sidebar (settings fica no rodapé). */
export const SCREENS: readonly Screen[] = [
  'dashboard',
  'projects',
  'services',
  'runtimes',
  'database',
  'mail',
  'logs',
  'settings',
];

export const SCREEN_LABELS: Record<Screen, string> = {
  dashboard: 'Dashboard',
  projects: 'Projetos',
  services: 'Serviços',
  runtimes: 'Runtimes',
  database: 'Banco',
  mail: 'Mail',
  logs: 'Logs',
  settings: 'Configurações',
};
