import type { ReactNode } from 'react';
import type { Icon } from '@phosphor-icons/react';
import {
  CaretLeft,
  CaretRight,
  Database,
  EnvelopeSimple,
  FolderOpen,
  Heartbeat,
  Pulse,
  Scroll,
  Sliders,
  SquaresFour,
  Stack,
  TerminalWindow,
} from '@phosphor-icons/react';
import type { Screen } from '../lib/screens';
import { useT } from '../i18n';

type SidebarProps = {
  current: Screen;
  onNavigate: (s: Screen) => void;
  collapsed: boolean;
  onToggleCollapsed: () => void;
  /** Resumo compacto de serviços (plano 08); renderizado no item "Status" do rodapé. */
  status?: ReactNode;
};

const MAIN_ITEMS: { screen: Screen; icon: Icon }[] = [
  { screen: 'dashboard', icon: SquaresFour },
  { screen: 'projects', icon: FolderOpen },
  { screen: 'services', icon: Pulse },
  { screen: 'runtimes', icon: Stack },
  { screen: 'database', icon: Database },
  { screen: 'mail', icon: EnvelopeSimple },
  { screen: 'logs', icon: Scroll },
  { screen: 'cli', icon: TerminalWindow },
];

type NavItemProps = {
  icon: Icon;
  label: string;
  active: boolean;
  collapsed: boolean;
  onClick: () => void;
  trailing?: ReactNode;
};

function NavItem({ icon: IconCmp, label, active, collapsed, onClick, trailing }: NavItemProps) {
  return (
    <button
      type="button"
      onClick={onClick}
      title={collapsed ? label : undefined}
      aria-current={active ? 'page' : undefined}
      className={`flex h-9 w-full items-center gap-3 rounded-pill px-3 text-[13px] transition-colors ${
        active ? 'bg-accent-soft text-accent-fg' : 'text-fg-muted hover:bg-bg-card-hover hover:text-fg'
      } ${collapsed ? 'justify-center px-0' : ''}`}
    >
      <IconCmp size={18} weight={active ? 'fill' : 'regular'} className="shrink-0" />
      {!collapsed && <span className="truncate">{label}</span>}
      {!collapsed && trailing && <span className="ml-auto">{trailing}</span>}
    </button>
  );
}

export function Sidebar({ current, onNavigate, collapsed, onToggleCollapsed, status }: SidebarProps) {
  const t = useT('app');
  return (
    <aside
      className={`flex shrink-0 flex-col border-r border-border bg-bg-sidebar transition-[width] duration-150 ${
        collapsed ? 'w-14' : 'w-[225px]'
      }`}
    >
      <nav className="flex flex-1 flex-col gap-1 p-2">
        {MAIN_ITEMS.map(({ screen, icon }) => (
          <NavItem
            key={screen}
            icon={icon}
            label={t(screen)}
            active={current === screen}
            collapsed={collapsed}
            onClick={() => onNavigate(screen)}
          />
        ))}
      </nav>

      <div className="flex flex-col gap-1 border-t border-border p-2">
        <NavItem
          icon={Heartbeat}
          label={t('status')}
          active={false}
          collapsed={collapsed}
          onClick={() => onNavigate('services')}
          trailing={status}
        />
        <NavItem
          icon={Sliders}
          label={t('settings')}
          active={current === 'settings'}
          collapsed={collapsed}
          onClick={() => onNavigate('settings')}
        />
        <button
          type="button"
          onClick={onToggleCollapsed}
          aria-label={collapsed ? t('expandSidebar') : t('collapseSidebar')}
          className="mt-1 flex h-8 w-full items-center justify-center rounded-pill text-fg-faint hover:bg-bg-card-hover hover:text-fg"
        >
          {collapsed ? <CaretRight size={14} /> : <CaretLeft size={14} />}
        </button>
      </div>
    </aside>
  );
}
