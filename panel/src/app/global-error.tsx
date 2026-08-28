'use client';

import { useEffect, useState } from 'react';
import { brand, byline } from '@/lib/brand';
import { DEFAULT_LOCALE, resolveInitialLocale, translate, type Locale } from '@/i18n';

/**
 * global-error.tsx replaces the whole root layout when the application fails
 * to start, so there is NO Tailwind / globals.css here: every style is written
 * inline with the brand colours (#0B398C navy, #1791C8 cyan).
 *
 * For the same reason there is no I18nProvider above this component. It
 * resolves the locale itself and calls `translate` directly, starting from the
 * default so that the server's markup and the first client render agree. There
 * is no language switch on this screen: the application is broken, and the one
 * useful action is to retry or leave.
 */
export default function GlobalError({
  error,
  reset,
}: {
  error: Error & { digest?: string };
  reset: () => void;
}) {
  const [locale, setLocale] = useState<Locale>(DEFAULT_LOCALE);

  useEffect(() => {
    setLocale(resolveInitialLocale());
  }, []);

  const t = (key: string, params?: Record<string, string | number>) =>
    translate(locale, key, params);

  return (
    <html lang={locale}>
      <body
        style={{
          margin: 0,
          minHeight: '100vh',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          padding: '2.5rem 1rem',
          backgroundColor: '#F7F8FA',
          color: '#111827',
          fontFamily: 'Inter, system-ui, -apple-system, "Segoe UI", sans-serif',
          fontSize: '14px',
          lineHeight: 1.5,
        }}
      >
        <div
          style={{
            width: '100%',
            maxWidth: '32rem',
            backgroundColor: '#ffffff',
            border: '1px solid #e5e7eb',
            borderRadius: '8px',
            boxShadow: '0 1px 2px 0 rgb(16 24 40 / 0.05)',
          }}
        >
          <div
            style={{
              display: 'flex',
              alignItems: 'flex-start',
              gap: '0.75rem',
              padding: '1rem 1.25rem',
              borderBottom: '1px solid #e5e7eb',
            }}
          >
            <span
              style={{
                display: 'inline-flex',
                alignItems: 'center',
                justifyContent: 'center',
                flexShrink: 0,
                width: '32px',
                height: '32px',
                borderRadius: '6px',
                backgroundColor: brand.colors.navy,
              }}
            >
              <svg viewBox="0 0 64 64" width="20" height="20" aria-hidden="true" focusable="false">
                <path
                  d="M17.5 19.5 L32 44.5"
                  fill="none"
                  stroke="#FFFFFF"
                  strokeWidth="9"
                  strokeLinecap="round"
                  strokeLinejoin="round"
                />
                <path
                  d="M46.5 19.5 L32 44.5"
                  fill="none"
                  stroke={brand.colors.cyan}
                  strokeWidth="9"
                  strokeLinecap="round"
                  strokeLinejoin="round"
                />
              </svg>
            </span>
            <div>
              <h1 style={{ margin: 0, fontSize: '0.875rem', fontWeight: 600, color: '#111827' }}>
                {t('errors.globalTitle')}
              </h1>
              <p style={{ margin: '0.25rem 0 0', fontSize: '0.875rem', color: '#4b5563' }}>
                {t('errors.globalBody', { product: brand.productName, byline })}
              </p>
            </div>
          </div>

          <div style={{ padding: '1rem 1.25rem' }} role="alert">
            <p
              style={{
                margin: 0,
                fontSize: '0.75rem',
                fontWeight: 500,
                letterSpacing: '0.05em',
                textTransform: 'uppercase',
                color: '#6b7280',
              }}
            >
              {t('errors.detailsLabel')}
            </p>
            <pre
              style={{
                margin: '0.375rem 0 0',
                padding: '0.75rem',
                maxHeight: '12rem',
                overflow: 'auto',
                border: '1px solid #e5e7eb',
                borderRadius: '6px',
                backgroundColor: '#f9fafb',
                fontSize: '0.75rem',
                color: '#374151',
                whiteSpace: 'pre-wrap',
              }}
            >
              {error?.message || t('errors.noDetails')}
            </pre>
            {error?.digest && (
              <p style={{ margin: '0.75rem 0 0', fontSize: '0.75rem', color: '#6b7280' }}>
                {t('errors.digestInline')}{' '}
                <span style={{ color: '#374151' }}>{error.digest}</span>
              </p>
            )}
            <p style={{ margin: '0.75rem 0 0', fontSize: '0.875rem', color: '#4b5563' }}>
              {t('common.technicalSupport')}:{' '}
              <a
                href={`mailto:${brand.supportEmail}`}
                style={{ color: '#092E70', fontWeight: 500, textDecoration: 'none' }}
              >
                {brand.supportEmail}
              </a>
            </p>
          </div>

          <div
            style={{
              display: 'flex',
              flexWrap: 'wrap',
              gap: '0.5rem',
              padding: '1rem 1.25rem',
              borderTop: '1px solid #e5e7eb',
            }}
          >
            <button
              type="button"
              onClick={() => reset()}
              style={{
                padding: '0.5rem 0.875rem',
                fontSize: '0.875rem',
                fontWeight: 500,
                color: '#ffffff',
                backgroundColor: brand.colors.navy,
                border: `1px solid ${brand.colors.navy}`,
                borderRadius: '6px',
                cursor: 'pointer',
              }}
            >
              {t('common.retry')}
            </button>
            <a
              href="/"
              style={{
                padding: '0.5rem 0.875rem',
                fontSize: '0.875rem',
                fontWeight: 500,
                color: '#374151',
                backgroundColor: '#ffffff',
                border: '1px solid #d1d5db',
                borderRadius: '6px',
                textDecoration: 'none',
              }}
            >
              {t('common.backToHome')}
            </a>
          </div>
        </div>
      </body>
    </html>
  );
}
