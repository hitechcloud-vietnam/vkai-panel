'use client';

import { useState } from 'react';
import { useRouter } from 'next/navigation';
import { useAuthStore } from '@/store/auth';
import { brand, byline, copyright, description as brandDescription } from '@/lib/brand';
import { Eye, EyeOff, Loader2, AlertCircle, BookOpen, LifeBuoy } from 'lucide-react';

export default function LoginPage() {
  const router = useRouter();
  const { login, isLoading } = useAuthStore();
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [showPassword, setShowPassword] = useState(false);
  const [error, setError] = useState('');

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError('');

    try {
      await login(username, password);
      router.push('/dashboard');
    } catch (err: any) {
      setError(
        err?.response?.data?.error ||
          err?.message ||
          'Đăng nhập không thành công. Vui lòng kiểm tra lại tài khoản và mật khẩu.'
      );
    }
  };

  return (
    <div className="flex min-h-screen items-center justify-center bg-[#F7F8FA] px-4 py-12">
      <div className="w-full max-w-md">
        {/* The dang nhap - nen trang, vien xam, bong nhe */}
        <div className="rounded-lg border border-gray-200 bg-white shadow-sm">
          {/* Khoi thuong hieu: dau nhan + ten san pham + dong phu de */}
          <div className="border-b border-gray-200 px-6 py-7 text-center">
            <span className="mx-auto mb-4 inline-flex h-12 w-12 items-center justify-center rounded-lg bg-brand-600">
              <svg viewBox="0 0 64 64" width="30" height="30" aria-hidden="true" focusable="false">
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
            <h1 className="text-lg font-semibold text-gray-900">{brand.productName}</h1>
            <p className="mt-1 text-sm text-gray-500">{byline}</p>
            <p className="mt-3 text-sm text-gray-600">{brandDescription}</p>
          </div>

          {/* Bieu mau dang nhap */}
          <div className="px-6 py-6">
            {error && (
              <div
                role="alert"
                className="mb-4 flex items-start gap-2 rounded-md border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700"
              >
                <AlertCircle size={16} className="mt-0.5 flex-shrink-0" aria-hidden="true" />
                <span>{error}</span>
              </div>
            )}

            <form onSubmit={handleSubmit} className="space-y-4">
              <div>
                <label
                  htmlFor="login-username"
                  className="mb-1.5 block text-sm font-medium text-gray-700"
                >
                  Tên đăng nhập
                </label>
                <input
                  id="login-username"
                  name="username"
                  type="text"
                  autoComplete="username"
                  value={username}
                  onChange={(e) => setUsername(e.target.value)}
                  className="w-full rounded-md border border-gray-300 bg-white px-3 py-2 text-sm text-gray-900 placeholder:text-gray-400 focus:border-brand-500 focus:outline-none focus:ring-1 focus:ring-brand-500"
                  placeholder="Nhập tên đăng nhập"
                  required
                />
              </div>

              <div>
                <label
                  htmlFor="login-password"
                  className="mb-1.5 block text-sm font-medium text-gray-700"
                >
                  Mật khẩu
                </label>
                <div className="relative">
                  <input
                    id="login-password"
                    name="password"
                    type={showPassword ? 'text' : 'password'}
                    autoComplete="current-password"
                    value={password}
                    onChange={(e) => setPassword(e.target.value)}
                    className="w-full rounded-md border border-gray-300 bg-white px-3 py-2 pr-10 text-sm text-gray-900 placeholder:text-gray-400 focus:border-brand-500 focus:outline-none focus:ring-1 focus:ring-brand-500"
                    placeholder="Nhập mật khẩu"
                    required
                  />
                  <button
                    type="button"
                    onClick={() => setShowPassword(!showPassword)}
                    aria-label={showPassword ? 'Ẩn mật khẩu' : 'Hiện mật khẩu'}
                    className="absolute right-2 top-1/2 -translate-y-1/2 rounded-md p-1 text-gray-500 hover:bg-gray-50 hover:text-gray-900 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500"
                  >
                    {showPassword ? (
                      <EyeOff size={18} aria-hidden="true" />
                    ) : (
                      <Eye size={18} aria-hidden="true" />
                    )}
                  </button>
                </div>
              </div>

              <div className="flex items-center justify-between">
                <label htmlFor="login-remember" className="flex items-center gap-2">
                  <input
                    id="login-remember"
                    name="remember"
                    type="checkbox"
                    className="h-4 w-4 rounded border-gray-300 bg-white text-brand-600 focus:ring-brand-500"
                  />
                  <span className="text-sm text-gray-700">Ghi nhớ đăng nhập</span>
                </label>
                <a
                  href={`mailto:${brand.supportEmail}`}
                  className="rounded-md text-sm font-medium text-brand-700 hover:text-brand-800 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500"
                >
                  Quên mật khẩu?
                </a>
              </div>

              <button
                type="submit"
                disabled={isLoading}
                className="inline-flex w-full items-center justify-center gap-2 rounded-md bg-brand-600 px-3.5 py-2 text-sm font-medium text-white hover:bg-brand-700 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500 focus-visible:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-60"
              >
                {isLoading ? (
                  <>
                    <Loader2 className="animate-spin" size={16} aria-hidden="true" />
                    Đang đăng nhập…
                  </>
                ) : (
                  'Đăng nhập'
                )}
              </button>
            </form>
          </div>

          {/* Chan the: tai lieu va ho tro ky thuat */}
          <div className="flex flex-wrap items-center justify-center gap-x-6 gap-y-2 border-t border-gray-200 bg-brand-50 px-6 py-3.5">
            <a
              href={brand.docsUrl}
              target="_blank"
              rel="noreferrer"
              className="inline-flex items-center gap-1.5 rounded-md text-sm font-medium text-brand-700 hover:text-brand-800 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500"
            >
              <BookOpen size={15} aria-hidden="true" />
              Tài liệu hướng dẫn
            </a>
            <a
              href={`mailto:${brand.supportEmail}`}
              className="inline-flex items-center gap-1.5 rounded-md text-sm font-medium text-brand-700 hover:text-brand-800 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500"
            >
              <LifeBuoy size={15} aria-hidden="true" />
              Hỗ trợ kỹ thuật
            </a>
          </div>
        </div>

        {/* Chan trang */}
        <p className="mt-6 text-center text-sm text-gray-500">{copyright()}</p>
      </div>
    </div>
  );
}
