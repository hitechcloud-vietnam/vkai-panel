'use client';

import type { ReactNode } from 'react';
import { cn } from '@/lib/utils';

/**
 * The trang dung chung cho moi khoi cua trang Dashboard:
 * bo 8px, vien 1px #E5E7EB, bong nhe, tieu de co duong ke duoi.
 */
export interface StatCardProps {
  title?: string;
  /** Dong mo ta ngan canh tieu de. */
  description?: string;
  /** Vung thao tac ben phai tieu de (nut, tab, bo loc...). */
  action?: ReactNode;
  children: ReactNode;
  className?: string;
  bodyClassName?: string;
  /** Bo padding cua than the - dung cho bang cuon ngang. */
  flush?: boolean;
}

export default function StatCard({
  title,
  description,
  action,
  children,
  className,
  bodyClassName,
  flush = false,
}: StatCardProps) {
  return (
    <section
      className={cn(
        'flex flex-col rounded-lg border border-gray-200 bg-white shadow-sm',
        className
      )}
    >
      {(title || action) && (
        <div className="flex flex-wrap items-center justify-between gap-3 border-b border-gray-200 px-5 py-3">
          <div className="min-w-0">
            {title && <h2 className="truncate text-sm font-semibold text-gray-900">{title}</h2>}
            {description && <p className="mt-0.5 truncate text-xs text-gray-500">{description}</p>}
          </div>
          {action && <div className="flex shrink-0 items-center gap-2">{action}</div>}
        </div>
      )}
      <div className={cn('min-w-0 flex-1', flush ? '' : 'px-5 py-4', bodyClassName)}>
        {children}
      </div>
    </section>
  );
}

/** Khoi xam nhat dung cho trang thai dang tai. */
export function Skeleton({ className }: { className?: string }) {
  return <div className={cn('rounded bg-gray-100', className)} aria-hidden="true" />;
}

/**
 * Trang thai rong / loi hien thi gon trong than the.
 * `tone` = 'error' doi sang tong do, mac dinh la tong trung tinh.
 */
export function StateMessage({
  icon,
  title,
  hint,
  action,
  tone = 'empty',
}: {
  icon?: ReactNode;
  title: string;
  hint?: string;
  action?: ReactNode;
  tone?: 'empty' | 'error';
}) {
  return (
    <div
      className="flex flex-col items-center justify-center px-4 py-8 text-center"
      role={tone === 'error' ? 'alert' : undefined}
    >
      {icon && (
        <div className={cn('mb-3', tone === 'error' ? 'text-red-600' : 'text-gray-300')}>
          {icon}
        </div>
      )}
      <p
        className={cn(
          'text-sm font-medium',
          tone === 'error' ? 'text-red-700' : 'text-gray-900'
        )}
      >
        {title}
      </p>
      {hint && <p className="mt-1 max-w-md text-xs text-gray-500">{hint}</p>}
      {action && <div className="mt-4">{action}</div>}
    </div>
  );
}

/** Chu thich ngan cho truong ma API chua tra ve. */
export function MissingNote({ children }: { children: ReactNode }) {
  return <span className="text-xs text-gray-500">{children}</span>;
}
