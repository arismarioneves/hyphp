import type { ReactNode } from 'react';

type SplitPaneProps = {
  list: ReactNode;
  footer?: ReactNode;
  children: ReactNode;
};

/** Lista (260px) | detalhe. A lista rola sozinha; o rodapé fica fixo. */
export function SplitPane({ list, footer, children }: SplitPaneProps) {
  return (
    <div className="grid h-full min-h-0 grid-cols-[260px_1fr] gap-4">
      <aside className="flex min-h-0 flex-col overflow-hidden rounded-card border border-border bg-bg-card">
        <div className="min-h-0 flex-1 overflow-y-auto p-2">{list}</div>
        {footer && <div className="border-t border-border p-2">{footer}</div>}
      </aside>
      <section className="min-h-0 overflow-y-auto">{children}</section>
    </div>
  );
}

type SplitPaneItemProps = {
  selected: boolean;
  onClick: () => void;
  children: ReactNode;
};

export function SplitPaneItem({ selected, onClick, children }: SplitPaneItemProps) {
  return (
    <button
      type="button"
      onClick={onClick}
      aria-current={selected ? 'true' : undefined}
      className={`flex w-full items-center gap-2 rounded-pill px-3 py-2 text-left text-sm focus-visible:outline-2 focus-visible:outline-accent ${
        selected ? 'bg-accent-soft text-accent-fg' : 'text-fg hover:bg-bg-card-hover'
      }`}
    >
      {children}
    </button>
  );
}
