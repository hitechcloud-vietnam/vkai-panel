'use client';

/**
 * The WP Toolkit section shell.
 *
 * A top-level section rather than a page under Websites: the toolkit is where
 * an operator lives while they are looking after WordPress, and seven screens
 * do not fit behind one sidebar entry. The sub-navigation below is the section's
 * own; the sidebar needs a single entry pointing at /wp-toolkit.
 */

import Link from 'next/link';
import { usePathname } from 'next/navigation';
import { cn } from '@/lib/utils';

interface Entry {
  href: string;
  label: string;
  /** Matched exactly, so /wp-toolkit does not light up on every child route. */
  exact?: boolean;
}

const ENTRIES: Entry[] = [
  { href: '/wp-toolkit', label: 'Installations', exact: true },
  { href: '/wp-toolkit/add', label: 'Add WordPress' },
  { href: '/wp-toolkit/migrate', label: 'Migrate Site' },
  { href: '/wp-toolkit/panel-migrate', label: 'Other Panel Migrate' },
  { href: '/wp-toolkit/sets', label: 'WP Sets' },
  { href: '/wp-toolkit/data-copy', label: 'Data Copy' },
  { href: '/wp-toolkit/statistics', label: 'Statistics' },
];

export default function WpToolkitLayout({ children }: { children: React.ReactNode }) {
  const pathname = usePathname() || '';

  return (
    <div className="space-y-5">
      <nav aria-label="WP Toolkit" className="rounded-lg border border-gray-200 bg-white shadow-sm">
        <ul className="flex flex-wrap items-center gap-1 p-1.5">
          {ENTRIES.map((entry) => {
            const active = entry.exact
              ? pathname === entry.href
              : pathname === entry.href || pathname.startsWith(`${entry.href}/`);
            return (
              <li key={entry.href}>
                <Link
                  href={entry.href}
                  aria-current={active ? 'page' : undefined}
                  className={cn(
                    'block rounded-md px-3 py-2 text-sm font-medium',
                    active
                      ? 'bg-brand-600 text-white'
                      : 'text-gray-600 hover:bg-gray-50 hover:text-gray-900',
                  )}
                >
                  {entry.label}
                </Link>
              </li>
            );
          })}
        </ul>
      </nav>

      {children}
    </div>
  );
}
