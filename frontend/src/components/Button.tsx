import type { ButtonHTMLAttributes, ReactNode } from 'react';
import { SpinnerGap } from '@phosphor-icons/react';

type Variant = 'primary' | 'secondary' | 'ghost' | 'danger';
type Size = 'sm' | 'md';

type ButtonProps = {
  variant?: Variant;
  size?: Size;
  icon?: ReactNode;
  loading?: boolean;
} & ButtonHTMLAttributes<HTMLButtonElement>;

const VARIANTS: Record<Variant, string> = {
  primary: 'bg-accent text-white hover:brightness-110 disabled:hover:brightness-100',
  secondary: 'border border-border bg-bg-card text-fg hover:bg-bg-card-hover',
  ghost: 'text-fg-muted hover:bg-bg-card-hover hover:text-fg',
  danger: 'bg-err/15 text-err hover:bg-err/25',
};

const SIZES: Record<Size, string> = {
  sm: 'h-7 px-2.5 text-xs gap-1.5',
  md: 'h-8 px-3 text-[13px] gap-2',
};

export function Button({
  variant = 'secondary',
  size = 'md',
  icon,
  loading = false,
  className = '',
  disabled,
  children,
  ...rest
}: ButtonProps) {
  return (
    <button
      type="button"
      disabled={disabled || loading}
      className={`inline-flex items-center justify-center rounded-pill font-medium transition-colors disabled:cursor-not-allowed disabled:opacity-50 ${VARIANTS[variant]} ${SIZES[size]} ${className}`}
      {...rest}
    >
      {loading ? <SpinnerGap size={16} className="animate-spin" /> : icon}
      {children}
    </button>
  );
}
