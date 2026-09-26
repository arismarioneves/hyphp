// Mesmo desenho de build/logo.svg. Fica inline, e não como <img>, para herdar
// a cor por currentColor e acompanhar o token de destaque.
const MINI = [
  [9, 9],
  [13, 9],
  [9, 13],
  [13, 13],
];

export function Logo({ size = 16, className = '' }: { size?: number; className?: string }) {
  return (
    <svg width={size} height={size} viewBox="0 0 16 16" fill="currentColor" className={className} aria-hidden="true">
      <path d="M0 1a1 1 0 0 1 1-1h5a1 1 0 0 1 1 1v14a1 1 0 0 1-1 1H1a1 1 0 0 1-1-1zm9 0a1 1 0 0 1 1-1h5a1 1 0 0 1 1 1v5a1 1 0 0 1-1 1h-5a1 1 0 0 1-1-1z" />
      {MINI.map(([x, y]) => (
        <rect key={`${x}-${y}`} x={x} y={y} width="3" height="3" rx=".6" />
      ))}
    </svg>
  );
}
