'use client';

/**
 * A figure the backend has not reported.
 *
 * The panel never prints 0 for something it does not know. A missing CPU
 * reading and an idle CPU look identical as "0%", and an operator who cannot
 * tell them apart cannot trust any number on the page. So the value renders as
 * an em dash carrying the reason: in the tooltip for a pointer, in the
 * accessible name for a screen reader.
 */

import { cn } from '@/lib/utils';
import { UNAVAILABLE } from '@/lib/servers';

export interface UnavailableProps {
  /** Why the figure is missing. Shown on hover and read out by assistive tech. */
  reason: string;
  className?: string;
}

export function Unavailable({ reason, className }: UnavailableProps) {
  return (
    <span
      title={reason}
      aria-label={reason}
      className={cn('cursor-help text-gray-400 underline decoration-dotted underline-offset-2', className)}
    >
      {UNAVAILABLE}
    </span>
  );
}

export interface MetricTextProps {
  /** The formatted figure, or null when the backend has not reported it. */
  value: string | null | undefined;
  /** Why it is missing. Used only when `value` is null. */
  reason: string;
  className?: string;
}

/** Renders a figure, or the unavailable dash with its reason. */
export function MetricText({ value, reason, className }: MetricTextProps) {
  if (value === null || value === undefined || value === '') {
    return <Unavailable reason={reason} className={className} />;
  }
  return <span className={className}>{value}</span>;
}

export default Unavailable;
