type ProgressBarProps = {
  /** 0..1; ignorado quando indeterminate */
  value?: number;
  indeterminate?: boolean;
  label?: string;
  tone?: 'accent' | 'err';
};

export function ProgressBar({ value = 0, indeterminate = false, label, tone = 'accent' }: ProgressBarProps) {
  const pct = Math.max(0, Math.min(1, value)) * 100;
  const fill = tone === 'err' ? 'bg-err' : 'bg-accent';
  return (
    <div className="flex flex-col gap-1">
      {label && (
        <div className="flex justify-between text-xs text-fg-muted">
          <span>{label}</span>
          {!indeterminate && <span className="font-mono">{Math.round(pct)}%</span>}
        </div>
      )}
      <div
        role="progressbar"
        aria-valuemin={0}
        aria-valuemax={100}
        aria-valuenow={indeterminate ? undefined : Math.round(pct)}
        className="relative h-1.5 w-full overflow-hidden rounded-full bg-bg-card-hover"
      >
        {indeterminate ? (
          <div className={`absolute inset-y-0 w-1/3 animate-pulse rounded-full ${fill}`} style={{ left: '33%' }} />
        ) : (
          <div className={`h-full rounded-full transition-[width] ${fill}`} style={{ width: `${pct}%` }} />
        )}
      </div>
    </div>
  );
}
