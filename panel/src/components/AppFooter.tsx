'use client';

import { useEffect, useState } from 'react';
import { company, docsUrl, supportEmail } from '@/lib/brand';
import { useT } from '@/i18n';

interface AppFooterProps {
  /** Copyright year. Computed in an effect when not supplied. */
  year?: number;
}

const FOCUS_RING =
  'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500';

/**
 * The one-line footer of the application shell.
 *
 * The year is settled in an effect (or handed in through props) so that the
 * server's markup and the browser's first render are identical and hydration
 * cannot break across a New Year boundary.
 */
export default function AppFooter({ year }: AppFooterProps) {
  const t = useT();
  const [resolvedYear, setResolvedYear] = useState<number | null>(year ?? null);

  useEffect(() => {
    if (year !== undefined) {
      setResolvedYear(year);
      return;
    }
    setResolvedYear(new Date().getFullYear());
  }, [year]);

  return (
    <footer className="flex h-10 shrink-0 flex-wrap items-center justify-between gap-x-4 gap-y-1 border-t border-gray-200 bg-white px-5 text-xs text-gray-500">
      <p className="truncate">
        {resolvedYear === null
          ? t('common.copyrightNoYear', { company })
          : t('common.copyright', { year: resolvedYear, company })}
      </p>

      <nav aria-label={t('footer.supportLinks')} className="flex shrink-0 items-center gap-4">
        <a
          href={docsUrl}
          target="_blank"
          rel="noopener noreferrer"
          className={`rounded text-brand-700 hover:text-brand-800 ${FOCUS_RING}`}
        >
          {t('common.documentation')}
        </a>
        <a
          href={`mailto:${supportEmail}`}
          className={`rounded text-brand-700 hover:text-brand-800 ${FOCUS_RING}`}
        >
          {t('common.support')}
        </a>
      </nav>
    </footer>
  );
}
