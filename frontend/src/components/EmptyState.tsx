import type { ReactNode } from 'react';

type EmptyStateProps = { icon?: ReactNode; title: string; description?: string; action?: ReactNode };

/** Estado vazio com ação (spec §12.5: "Nenhum projeto — adicionar diretório"). */
export function EmptyState({ icon, title, description, action }: EmptyStateProps) {
  return (
    <div className="flex flex-col items-center justify-center gap-2 px-6 py-10 text-center">
      {icon && <div className="text-fg-faint">{icon}</div>}
      <div className="font-mono text-sm text-fg">{title}</div>
      {description && <p className="max-w-sm text-xs text-fg-muted">{description}</p>}
      {action && <div className="mt-2">{action}</div>}
    </div>
  );
}
