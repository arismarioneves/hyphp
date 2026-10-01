export type Screen =
  | 'dashboard'
  | 'projects'
  | 'services'
  | 'runtimes'
  | 'database'
  | 'mail'
  | 'logs'
  | 'cli'
  | 'settings';

// O título de cada tela está no catálogo `app`, com o id da tela como chave.

/** Props de toda tela: navegação entre telas é a única dependência comum. */
export type ScreenProps = { onNavigate: (screen: Screen) => void };
