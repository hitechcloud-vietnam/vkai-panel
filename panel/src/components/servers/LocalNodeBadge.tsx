'use client';

/**
 * Marks the machine the panel is installed on. It is a managed node like any
 * other, but it is the one an operator never had to add, so it says so.
 */

import { Home } from 'lucide-react';

import { cn } from '@/lib/utils';

export interface LocalNodeBadgeProps {
  /** Wording comes from the calling screen, already translated. */
  label: string;
  /** Longer explanation shown on hover. */
  title?: string;
  className?: string;
}

export default function LocalNodeBadge({ label, title, className }: LocalNodeBadgeProps) {
  return (
    <span
      title={title}
      className={cn(
        'inline-flex shrink-0 items-center gap-1 rounded-md border border-brand-200 bg-brand-50 px-2 py-0.5 text-xs font-medium text-brand-700',
        className
      )}
    >
      <Home size={12} aria-hidden="true" />
      {label}
    </span>
  );
}
