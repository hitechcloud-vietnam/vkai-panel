'use client';

/**
 * Sign-in, in one or two steps.
 *
 * Step one is the password. If the account has a second factor the server
 * answers with a challenge instead of a session - no token is issued, nothing
 * is stored - and step two exchanges that challenge plus one code for the real
 * pair.
 *
 * Two things drive the shape of this page. Every exchange attempt spends the
 * challenge it was made with, so a wrong code comes back with a replacement
 * that this page swaps in; and a challenge that has expired or been spent
 * cannot be retried at all, so the page returns to the password step and says
 * why rather than leaving the user typing codes at a dead token.
 */

import { useState } from 'react';
import { useRouter } from 'next/navigation';
import { TwoFactorError, useAuthStore } from '@/store/auth';
import { brand, byline, copyright, description as brandDescription } from '@/lib/brand';
import {
  AlertCircle,
  ArrowLeft,
  BookOpen,
  Eye,
  EyeOff,
  KeyRound,
  LifeBuoy,
  Loader2,
  ShieldCheck,
} from 'lucide-react';

type Step = 'password' | 'two-factor';

const inputClass =
  'w-full rounded-md border border-gray-300 bg-white px-3 py-2 text-sm text-gray-900 placeholder:text-gray-400 focus:border-brand-500 focus:outline-none focus:ring-1 focus:ring-brand-500';

/** Pulls the panel's error text out of an axios failure. */
function errorMessage(err: any, fallback: string): string {
  return err?.response?.data?.error?.message || err?.message || fallback;
}

export default function LoginPage() {
  const router = useRouter();
  const { login, completeTwoFactor, isLoading } = useAuthStore();

  const [step, setStep] = useState<Step>('password');
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [showPassword, setShowPassword] = useState(false);
  const [error, setError] = useState('');

  const [challengeToken, setChallengeToken] = useState('');
  const [code, setCode] = useState('');
  const [useRecoveryCode, setUseRecoveryCode] = useState(false);

  /** Returns to step one, keeping the username and clearing the rest. */
  const backToPassword = (message: string) => {
    setStep('password');
    setChallengeToken('');
    setCode('');
    setUseRecoveryCode(false);
    setPassword('');
    setError(message);
  };

  const handlePasswordSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError('');

    try {
      const result = await login(username, password);
      if (result.status === 'two_factor_required') {
        setChallengeToken(result.challengeToken);
        setCode('');
        setUseRecoveryCode(false);
        setStep('two-factor');
        return;
      }
      router.push('/dashboard');
    } catch (err: any) {
      setError(
        errorMessage(
          err,
          'Đăng nhập không thành công. Vui lòng kiểm tra lại tài khoản và mật khẩu.'
        )
      );
    }
  };

  const handleCodeSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError('');

    try {
      await completeTwoFactor(challengeToken, code.trim());
      router.push('/dashboard');
    } catch (err: any) {
      if (err instanceof TwoFactorError && err.reason === 'retry' && err.challengeToken) {
        // The code was wrong and the window is still open: swap in the
        // replacement challenge and ask again.
        setChallengeToken(err.challengeToken);
        setCode('');
        setError(err.message);
        return;
      }
      backToPassword(errorMessage(err, 'Xác thực không thành công. Vui lòng đăng nhập lại.'));
    }
  };

  return (
    <div className="flex min-h-screen items-center justify-center bg-[#F7F8FA] px-4 py-12">
      <div className="w-full max-w-md">
        {/* Sign-in card: white, gray border, light shadow */}
        <div className="rounded-lg border border-gray-200 bg-white shadow-sm">
          {/* Brand block: mark, product name, byline */}
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

            {step === 'password' ? (
              <form onSubmit={handlePasswordSubmit} className="space-y-4">
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
                    className={inputClass}
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
                      className={`${inputClass} pr-10`}
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
            ) : (
              <form onSubmit={handleCodeSubmit} className="space-y-4">
                <div className="flex items-start gap-2 rounded-md border border-gray-200 bg-gray-50 px-3 py-2.5 text-sm text-gray-700">
                  <ShieldCheck size={16} className="mt-0.5 flex-shrink-0 text-gray-400" aria-hidden="true" />
                  <span>
                    {useRecoveryCode
                      ? 'Nhập một mã khôi phục bạn đã lưu khi bật xác thực hai lớp. Mỗi mã chỉ dùng được một lần.'
                      : 'Tài khoản này bật xác thực hai lớp. Nhập mã đang hiển thị trong ứng dụng xác thực của bạn.'}
                  </span>
                </div>

                <div>
                  <label
                    htmlFor="login-code"
                    className="mb-1.5 block text-sm font-medium text-gray-700"
                  >
                    {useRecoveryCode ? 'Mã khôi phục' : 'Mã xác thực'}
                  </label>
                  <input
                    id="login-code"
                    name="code"
                    type="text"
                    autoComplete="one-time-code"
                    inputMode={useRecoveryCode ? 'text' : 'numeric'}
                    autoFocus
                    spellCheck={false}
                    value={code}
                    onChange={(e) => setCode(e.target.value)}
                    className={`${inputClass} font-mono tracking-[0.2em]`}
                    placeholder={useRecoveryCode ? 'XXXXX-XXXXX' : '000000'}
                    maxLength={useRecoveryCode ? 24 : 8}
                    required
                  />
                </div>

                <button
                  type="submit"
                  disabled={isLoading}
                  className="inline-flex w-full items-center justify-center gap-2 rounded-md bg-brand-600 px-3.5 py-2 text-sm font-medium text-white hover:bg-brand-700 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500 focus-visible:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-60"
                >
                  {isLoading ? (
                    <>
                      <Loader2 className="animate-spin" size={16} aria-hidden="true" />
                      Đang xác thực…
                    </>
                  ) : (
                    'Xác thực và đăng nhập'
                  )}
                </button>

                <div className="flex items-center justify-between pt-1">
                  <button
                    type="button"
                    onClick={() => backToPassword('')}
                    className="inline-flex items-center gap-1.5 rounded-md text-sm font-medium text-gray-600 hover:text-gray-900 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500"
                  >
                    <ArrowLeft size={15} aria-hidden="true" />
                    Quay lại
                  </button>
                  <button
                    type="button"
                    onClick={() => {
                      setUseRecoveryCode(!useRecoveryCode);
                      setCode('');
                      setError('');
                    }}
                    className="inline-flex items-center gap-1.5 rounded-md text-sm font-medium text-brand-700 hover:text-brand-800 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500"
                  >
                    <KeyRound size={15} aria-hidden="true" />
                    {useRecoveryCode ? 'Dùng mã từ ứng dụng xác thực' : 'Dùng mã khôi phục'}
                  </button>
                </div>
              </form>
            )}
          </div>

          {/* Card footer: documentation and support */}
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

        {/* Page footer */}
        <p className="mt-6 text-center text-sm text-gray-500">{copyright()}</p>
      </div>
    </div>
  );
}
