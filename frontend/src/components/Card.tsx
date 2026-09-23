import type { ReactNode } from 'react';

type CardProps = {
  title?: string;
  actions?: ReactNode;
  className?: string;
  children: ReactNode;
};

export function Card({ title, actions, className = '', children }: CardProps) {
  return (
    <section className={`rounded-card border border-border bg-bg-card ${className}`}>
      {(title || actions) && (
        <header className="flex items-center justify-between gap-3 border-b border-border px-4 py-3">
          {title && <h2 className="font-mono text-sm font-medium text-fg">{title}</h2>}
          {actions && <div className="flex items-center gap-2">{actions}</div>}
        </header>
      )}
      <div className="p-4">{children}</div>
    </section>
  );
}
