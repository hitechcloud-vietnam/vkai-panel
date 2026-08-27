import Link from 'next/link';
import { Home, LayoutDashboard } from 'lucide-react';
import { brand } from '@/lib/brand';

export default function NotFound() {
  return (
    <div className="flex min-h-screen items-center justify-center bg-[#F7F8FA] px-4 py-10">
      <div className="w-full max-w-md rounded-lg border border-gray-200 bg-white shadow-sm">
        <div className="border-b border-gray-200 px-5 py-4">
          <p className="text-xs font-semibold uppercase tracking-wide text-gray-500">Lỗi 404</p>
          <h1 className="mt-1 text-sm font-semibold text-gray-900">Không tìm thấy trang</h1>
        </div>
        <div className="px-5 py-4">
          <p className="text-sm text-gray-600">
            Đường dẫn bạn truy cập không tồn tại hoặc đã được di chuyển. Hãy quay lại bảng điều
            khiển {brand.productName} để tiếp tục.
          </p>
        </div>
        <div className="flex flex-wrap items-center gap-2 border-t border-gray-200 px-5 py-4">
          <Link
            href="/dashboard"
            className="inline-flex items-center gap-2 rounded-md bg-brand-600 px-3.5 py-2 text-sm font-medium text-white hover:bg-brand-700 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500 focus-visible:ring-offset-2"
          >
            <LayoutDashboard size={16} aria-hidden="true" />
            Về bảng điều khiển
          </Link>
          <Link
            href="/"
            className="inline-flex items-center gap-2 rounded-md border border-gray-300 bg-white px-3.5 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500 focus-visible:ring-offset-2"
          >
            <Home size={16} aria-hidden="true" />
            Trang chủ
          </Link>
        </div>
      </div>
    </div>
  );
}
