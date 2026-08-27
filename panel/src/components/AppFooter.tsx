'use client';

import { useEffect, useState } from 'react';
import { company, copyright, docsUrl, supportEmail } from '@/lib/brand';

interface AppFooterProps {
  /** Nam ban quyen. Neu khong truyen, component tu tinh trong useEffect. */
  year?: number;
}

const FOCUS_RING =
  'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500';

/**
 * Chan trang mot dong cho vo ung dung.
 *
 * Nam duoc tinh trong useEffect (hoac nhan qua props) de ket qua render tren
 * may chu va tren trinh duyet luon giong nhau, tranh lech hydration.
 */
export default function AppFooter({ year }: AppFooterProps) {
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
        {resolvedYear === null ? `© ${company}. Bảo lưu mọi quyền.` : copyright(resolvedYear)}
      </p>

      <nav aria-label="Liên kết hỗ trợ" className="flex shrink-0 items-center gap-4">
        <a
          href={docsUrl}
          target="_blank"
          rel="noopener noreferrer"
          className={`rounded text-brand-700 hover:text-brand-800 ${FOCUS_RING}`}
        >
          Tài liệu
        </a>
        <a
          href={`mailto:${supportEmail}`}
          className={`rounded text-brand-700 hover:text-brand-800 ${FOCUS_RING}`}
        >
          Hỗ trợ
        </a>
      </nav>
    </footer>
  );
}
