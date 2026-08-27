'use client';

import { brand } from '@/lib/brand';

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
          backgroundColor: '#f9fafb',
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
          <div style={{ padding: '1rem 1.25rem', borderBottom: '1px solid #e5e7eb' }}>
            <h1 style={{ margin: 0, fontSize: '0.875rem', fontWeight: 600, color: '#111827' }}>
              Ứng dụng gặp sự cố nghiêm trọng
            </h1>
            <p style={{ margin: '0.25rem 0 0', fontSize: '0.875rem', color: '#4b5563' }}>
              {brand.productName} không thể khởi tạo giao diện. Vui lòng thử lại hoặc liên hệ quản
              trị viên.
            </p>
          </div>

          <div style={{ padding: '1rem 1.25rem' }}>
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
                backgroundColor: '#2563eb',
                border: '1px solid #2563eb',
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
