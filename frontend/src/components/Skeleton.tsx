type SkeletonProps = { className?: string; lines?: number };

/** Placeholder de carregamento. `lines` > 1 empilha barras com larguras decrescentes. */
export function Skeleton({ className = '', lines = 1 }: SkeletonProps) {
  if (lines <= 1) {
    return <div className={`h-4 animate-pulse rounded-pill bg-bg-card-hover ${className}`} />;
  }
  const widths = ['w-full', 'w-11/12', 'w-4/5', 'w-2/3', 'w-1/2'];
  return (
    <div className={`flex flex-col gap-2 ${className}`}>
      {Array.from({ length: lines }, (_, i) => (
        <div key={i} className={`h-4 animate-pulse rounded-pill bg-bg-card-hover ${widths[i % widths.length]}`} />
      ))}
    </div>
  );
}
