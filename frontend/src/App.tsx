import { useCallback, useEffect, useState } from 'react';
import { ArrowSquareOut, FolderOpen, Terminal } from '@phosphor-icons/react';
import { AppService } from '../bindings/hyphp/services';
import { TitleBar } from './components/TitleBar';
import { Sidebar } from './components/Sidebar';
import { Card } from './components/Card';
import { SectionLabel } from './components/SectionLabel';
import { Badge } from './components/Badge';
import { StatusDot } from './components/StatusDot';
import { Skeleton } from './components/Skeleton';
import { EmptyState } from './components/EmptyState';
import { Button } from './components/Button';
import { SCREEN_LABELS, type Screen } from './lib/screens';

const COLLAPSED_KEY = 'hyphp.sidebarCollapsed';



function ScreenPlaceholder({ screen }: { screen: Screen }) {
  return (
    <Card title={SCREEN_LABELS[screen]}>
      <EmptyState
        title="Em construção"
        description="Esta tela é implementada no plano 08 (ui-screens)."
        action={
          <Badge tone="accent" mono>
            {screen}
          </Badge>
        }
      />
    </Card>
  );
}

function DashboardPlaceholder() {
  return (
    <div className="flex flex-col gap-4">
      <Card>
        <SectionLabel className="mb-3">Your stack</SectionLabel>
        <div className="flex flex-wrap items-center gap-4 text-sm">
          <span className="flex items-center gap-2">
            <StatusDot state="stopped" /> Apache
          </span>
          <span className="flex items-center gap-2">
            <StatusDot state="starting" /> PHP <Badge mono>8.1</Badge>
          </span>
          <span className="flex items-center gap-2">
            <StatusDot state="ready" /> MySQL
          </span>
          <span className="flex items-center gap-2">
            <StatusDot state="degraded" /> Mailpit
          </span>
          <span className="flex items-center gap-2">
            <StatusDot state="failed" /> queue
          </span>
        </div>
      </Card>
      <Card title="Carregando (skeleton)">
        <Skeleton lines={3} />
      </Card>
    </div>
  );
}

function SettingsPlaceholder() {
  const [version, setVersion] = useState<string | null>(null);
  const [root, setRoot] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    AppService.Version().then(setVersion).catch((e: unknown) => setError(String(e)));
    AppService.RuntimeRoot().then(setRoot).catch((e: unknown) => setError(String(e)));
  }, []);

  const run = (p: Promise<void>) => p.catch((e: unknown) => setError(String(e)));

  return (
    <div className="flex flex-col gap-4">
      <Card title="Sobre">
        <dl className="grid grid-cols-[140px_1fr] gap-y-2 text-sm">
          <dt className="text-fg-muted">Versão</dt>
          <dd className="selectable font-mono">{version ?? <Skeleton className="w-16" />}</dd>
          <dt className="text-fg-muted">Raiz do runtime</dt>
          <dd className="selectable font-mono">{root ?? <Skeleton className="w-64" />}</dd>
        </dl>
        <div className="mt-4 flex flex-wrap gap-2">
          <Button
            icon={<FolderOpen size={16} />}
            disabled={!root}
            onClick={() => root && run(AppService.OpenFolder(root))}
          >
            Abrir pasta
          </Button>
          <Button
            icon={<Terminal size={16} />}
            disabled={!root}
            onClick={() => root && run(AppService.OpenTerminal(root))}
          >
            Abrir terminal
          </Button>
          <Button
            icon={<ArrowSquareOut size={16} />}
            variant="ghost"
            onClick={() => run(AppService.OpenExternal('https://v3.wails.io'))}
          >
            Documentação do Wails
          </Button>
          <Button variant="danger" onClick={() => run(AppService.Quit())}>
            Sair do HyPHP
          </Button>
        </div>
        {error && <p className="selectable mt-3 text-xs text-err">{error}</p>}
      </Card>
    </div>
  );
}

function App() {
  const [screen, setScreen] = useState<Screen>('dashboard');
  const [collapsed, setCollapsed] = useState<boolean>(
    () => localStorage.getItem(COLLAPSED_KEY) === '1',
  );

  const toggleCollapsed = useCallback(() => {
    setCollapsed((c) => {
      localStorage.setItem(COLLAPSED_KEY, c ? '0' : '1');
      return !c;
    });
  }, []);

  let content: JSX.Element;
  switch (screen) {
    case 'dashboard':
      content = <DashboardPlaceholder />;
      break;
    case 'settings':
      content = <SettingsPlaceholder />;
      break;
    default:
      content = <ScreenPlaceholder screen={screen} />;
  }

  return (
    <div className="flex h-full flex-col bg-bg-app text-fg">
      <TitleBar />
      <div className="flex min-h-0 flex-1">
        <Sidebar
          current={screen}
          onNavigate={setScreen}
          collapsed={collapsed}
          onToggleCollapsed={toggleCollapsed}
        />
        <main className="min-w-0 flex-1 overflow-auto p-6">
          <h1 className="mb-4 font-mono text-lg font-semibold">{SCREEN_LABELS[screen]}</h1>
          {content}
        </main>
      </div>
    </div>
  );
}

export default App;
