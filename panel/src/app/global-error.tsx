'use client';

import { brand, byline } from '@/lib/brand';

/**
 * global-error.tsx thay the toan bo root layout khi ung dung sup do,
 * nen KHONG co Tailwind / globals.css o day: moi kieu dang phai viet inline
 * bang dung mau thuong hieu (#0B398C navy, #1791C8 cyan).
 */
export default function GlobalError({
  error,
  reset,
}: {
  error: Error & { digest?: string };
  reset: () => void;
}) {
  return (
    <html lang="vi">
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
                Ứng dụng gặp sự cố nghiêm trọng
              </h1>
              <p style={{ margin: '0.25rem 0 0', fontSize: '0.875rem', color: '#4b5563' }}>
                {brand.productName} {byline} không thể khởi tạo giao diện. Vui lòng thử lại hoặc
                liên hệ quản trị viên.
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
              Chi tiết lỗi
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
              {error?.message || 'Không có thông tin chi tiết.'}
            </pre>
            {error?.digest && (
              <p style={{ margin: '0.75rem 0 0', fontSize: '0.75rem', color: '#6b7280' }}>
                Mã lỗi (digest): <span style={{ color: '#374151' }}>{error.digest}</span>
              </p>
            )}
            <p style={{ margin: '0.75rem 0 0', fontSize: '0.875rem', color: '#4b5563' }}>
              Hỗ trợ kỹ thuật:{' '}
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
              Thử lại
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
              Về trang chủ
            </a>
          </div>
        </div>
      </body>
    </html>
  );
}
