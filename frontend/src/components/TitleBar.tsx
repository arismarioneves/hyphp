import { Window } from '@wailsio/runtime';
import { Minus, Square, X } from '@phosphor-icons/react';

async function toggleMaximise() {
  if (await Window.IsMaximised()) {
    await Window.Restore();
  } else {
    await Window.Maximise();
  }
}

const BTN =
  'no-drag inline-flex h-10 w-11 items-center justify-center text-fg-muted transition-colors hover:bg-bg-card-hover hover:text-fg';

export function TitleBar() {
  return (
    <header
      className="drag flex h-10 shrink-0 items-center justify-between border-b border-border bg-bg-sidebar"
      onDoubleClick={toggleMaximise}
    >
      <div className="flex items-center gap-2 pl-4">
        <span className="inline-block h-3 w-3 rounded-sm bg-accent" aria-hidden="true" />
        <span className="font-mono text-sm font-semibold tracking-tight text-fg">HyPHP</span>
      </div>
      <div className="flex" onDoubleClick={(e) => e.stopPropagation()}>
        <button type="button" className={BTN} aria-label="Minimizar" onClick={() => Window.Minimise()}>
          <Minus size={16} />
        </button>
        <button type="button" className={BTN} aria-label="Maximizar" onClick={toggleMaximise}>
          <Square size={14} />
        </button>
        <button
          type="button"
          className={`${BTN} hover:bg-err hover:text-white`}
          aria-label="Fechar"
          title="Fechar (mantém no tray)"
          onClick={() => Window.Close()}
        >
          <X size={16} />
        </button>
      </div>
    </header>
  );
}
