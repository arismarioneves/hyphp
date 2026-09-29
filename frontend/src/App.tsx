import { useState, type ReactElement } from 'react';
import '@wailsio/runtime';
import { Sidebar } from './components/Sidebar';
import { StatusDot, type ServiceState } from './components/StatusDot';
import { TitleBar } from './components/TitleBar';
import { UpdateBanner } from './components/UpdateBanner';
import { type Screen, type ScreenProps } from './lib/screens';
import { aggregateState, summarize } from './lib/status';
import { useApplyTheme } from './lib/theme';
import { useServices } from './lib/useServices';
import { useSettings } from './lib/useSettings';
import { Dashboard } from './screens/Dashboard';
import { Database } from './screens/Database';
import { Logs } from './screens/Logs';
import { Mail } from './screens/Mail';
import { Projects } from './screens/Projects';
import { Runtimes } from './screens/Runtimes';
import { Services } from './screens/Services';
import { Settings } from './screens/Settings';
import { useT } from './i18n';

// O tsconfig usa "jsx": "react-jsx", que não declara o global JSX; por isso o
// retorno é ReactElement e não JSX.Element.
const SCREENS: Record<Screen, (props: ScreenProps) => ReactElement> = {
  dashboard: Dashboard,
  projects: Projects,
  services: Services,
  runtimes: Runtimes,
  database: Database,
  mail: Mail,
  logs: Logs,
  settings: Settings,
};

export default function App() {
  const t = useT('app');
  const [screen, setScreen] = useState<Screen>('dashboard');
  const { services } = useServices();
  const { settings, save } = useSettings();
  useApplyTheme(settings?.theme);

  // O colapso mora no state.json, não no localStorage: o WebView pode ter o
  // armazenamento limpo entre execuções, e a preferência tem que sobreviver a
  // fechar para o tray e reabrir.
  const collapsed = settings?.sidebarCollapsed ?? false;
  const toggleCollapsed = () => {
    if (!settings) return;
    void save({ ...settings, sidebarCollapsed: !collapsed });
  };

  const summary = summarize(services);
  const overall = aggregateState(services.map((s) => s.state as ServiceState));
  const status = (
    <span
      className="flex items-center gap-2 font-mono text-xs text-fg-muted"
      title={t('statusTitle')}
    >
      <StatusDot state={overall} />
      {!collapsed && (
        <span>
          {t.plural('statusReady', summary.ready)}
          {summary.degraded > 0 ? t.plural('statusFailed', summary.degraded) : ''}
        </span>
      )}
    </span>
  );

  const Current = SCREENS[screen];

  return (
    <div className="flex h-screen flex-col bg-bg-app font-sans text-fg">
      <TitleBar />
      <div className="flex min-h-0 flex-1">
        <Sidebar
          current={screen}
          onNavigate={setScreen}
          collapsed={collapsed}
          onToggleCollapsed={toggleCollapsed}
          status={status}
        />
        <main className="flex min-h-0 flex-1 flex-col overflow-hidden">
          <UpdateBanner />
          <header className="px-6 pb-3 pt-5">
            <h1 className="font-mono text-lg text-fg">{t(screen)}</h1>
          </header>
          {/* key={screen} descarta o estado local ao trocar de tela: sem isso o
              LogView da tela anterior seguiria com o stream aberto. */}
          <div className="min-h-0 flex-1 overflow-y-auto px-6 pb-6">
            <Current key={screen} onNavigate={setScreen} />
          </div>
        </main>
      </div>
    </div>
  );
}
