'use client';

import { useEffect, useRef, useState } from 'react';
import Link from 'next/link';
import { usePathname, useRouter } from 'next/navigation';
import { Bell, ChevronDown, LogOut, Search, Settings, User } from 'lucide-react';
import { useAuthStore } from '@/store/auth';
import { monitoringApi, unwrap } from '@/services/api';
import { brand, versionLabel } from '@/lib/brand';

/** Gia tri hien thi khi API chua cung cap truong du lieu tuong ung. */
const EMPTY_VALUE = '—';

/** Vien focus dung chung cho moi phan tu tuong tac tren thanh tren. */
const FOCUS_RING =
  'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500';

/**
 * Dang du lieu tra ve tu GET /api/v1/monitoring/system.
 * Chi khai bao nhung truong thanh tren thuc su dung den.
 */
interface SystemInfoResponse {
  system?: {
    os?: string;
    arch?: string;
    num_cpu?: number;
  };
}

export default function Header() {
  const pathname = usePathname();
  const router = useRouter();
  const { user, logout } = useAuthStore();
  const [showUserMenu, setShowUserMenu] = useState(false);
  const [osLabel, setOsLabel] = useState<string | null>(null);
  const menuRef = useRef<HTMLDivElement>(null);
  const menuButtonRef = useRef<HTMLButtonElement>(null);

  // Ten he dieu hanh lay tu endpoint giam sat co san.
  // Endpoint hien khong tra ve thoi gian hoat dong (uptime) nen o do hien "—".
  useEffect(() => {
    let cancelled = false;

    monitoringApi
      .getSystemInfo()
      .then((res) => {
        if (cancelled) return;
        const info = unwrap<SystemInfoResponse>(res, null);
        const os = info?.system?.os;
        const arch = info?.system?.arch;
        if (os) {
          setOsLabel(arch ? `${os} / ${arch}` : os);
        }
      })
      .catch(() => {
        // Khong co quyen hoac API loi: giu nguyen "—", khong bia du lieu.
        if (!cancelled) setOsLabel(null);
      });

    return () => {
      cancelled = true;
    };
  }, []);

  useEffect(() => {
    if (!showUserMenu) return;

    const onClickOutside = (event: MouseEvent) => {
      if (menuRef.current && !menuRef.current.contains(event.target as Node)) {
        setShowUserMenu(false);
      }
    };
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        setShowUserMenu(false);
        menuButtonRef.current?.focus();
      }
    };

    document.addEventListener('mousedown', onClickOutside);
    document.addEventListener('keydown', onKeyDown);
    return () => {
      document.removeEventListener('mousedown', onClickOutside);
      document.removeEventListener('keydown', onKeyDown);
    };
  }, [showUserMenu]);

  useEffect(() => {
    setShowUserMenu(false);
  }, [pathname]);

  const displayName =
    [user?.first_name, user?.last_name].filter(Boolean).join(' ') || user?.username || 'Người dùng';
  const initial = (user?.first_name || user?.username || 'U').charAt(0).toUpperCase();

  const handleLogout = () => {
    setShowUserMenu(false);
    logout();
    router.replace('/login');
  };

  return (
    <header className="flex h-14 shrink-0 items-center justify-between gap-4 border-b border-gray-200 bg-white px-4">
      {/* Trai: ten panel, he dieu hanh, thoi gian hoat dong */}
      <div className="flex min-w-0 items-center gap-3 text-sm">
        <Link
          href="/dashboard"
          className={`shrink-0 rounded font-semibold text-gray-900 hover:text-brand-700 ${FOCUS_RING}`}
        >
          {brand.productName}
        </Link>

        <span className="hidden h-4 w-px shrink-0 bg-gray-200 sm:block" aria-hidden />

        <p className="hidden min-w-0 items-center gap-1.5 sm:flex">
          <span className="shrink-0 text-gray-500">Hệ điều hành:</span>
          <span className="truncate text-gray-700">{osLabel || EMPTY_VALUE}</span>
        </p>

        <span className="hidden h-4 w-px shrink-0 bg-gray-200 lg:block" aria-hidden />

        <p className="hidden items-center gap-1.5 lg:flex">
          <span className="shrink-0 text-gray-500">Thời gian hoạt động:</span>
          <span
            className="text-gray-700"
            title="API hiện chưa cung cấp thời gian hoạt động của máy chủ"
          >
            {EMPTY_VALUE}
          </span>
        </p>
      </div>

      {/* Phai: phien ban, tim kiem, thong bao, nguoi dung */}
      <div className="flex shrink-0 items-center gap-2">
        <span className="hidden rounded border border-brand-200 bg-brand-50 px-2 py-0.5 text-xs font-medium text-brand-700 sm:inline">
          {versionLabel}
        </span>

        {/* Tim kiem */}
        <div className="relative hidden md:block">
          <label htmlFor="header-search" className="sr-only">
            Tìm kiếm
          </label>
          <Search
            size={15}
            className="pointer-events-none absolute left-2.5 top-1/2 -translate-y-1/2 text-gray-400"
            aria-hidden
          />
          <input
            id="header-search"
            type="search"
            placeholder="Tìm kiếm…"
            className="w-48 rounded-md border border-gray-300 bg-white py-1.5 pl-8 pr-3 text-sm text-gray-900 placeholder:text-gray-500 focus:border-brand-500 focus:outline-none focus:ring-1 focus:ring-brand-500"
          />
        </div>

        {/* Thong bao */}
        <Link
          href="/notifications"
          aria-label="Thông báo"
          title="Thông báo"
          className={`rounded-md p-2 text-gray-500 hover:bg-gray-100 hover:text-gray-900 ${FOCUS_RING}`}
        >
          <Bell size={18} aria-hidden />
        </Link>

        {/* Menu nguoi dung */}
        <div className="relative" ref={menuRef}>
          <button
            ref={menuButtonRef}
            type="button"
            onClick={() => setShowUserMenu((prev) => !prev)}
            aria-haspopup="menu"
            aria-expanded={showUserMenu}
            aria-label={`Tài khoản: ${displayName}`}
            className={`flex items-center gap-2 rounded-md px-2 py-1.5 hover:bg-gray-100 ${FOCUS_RING}`}
          >
            <span className="flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-brand-600 text-sm font-medium text-white">
              {initial}
            </span>
            <span className="hidden text-left lg:block">
              <span className="block text-sm font-medium leading-tight text-gray-900">
                {displayName}
              </span>
              <span className="block text-xs leading-tight text-gray-500">{user?.email}</span>
            </span>
            <ChevronDown size={14} className="hidden text-gray-500 lg:block" aria-hidden />
          </button>

          {showUserMenu && (
            <div
              role="menu"
              aria-label="Menu người dùng"
              className="absolute right-0 top-full z-50 mt-2 w-56 rounded-md border border-gray-200 bg-white py-1 shadow-lg"
            >
              <div className="border-b border-gray-200 px-3 py-2">
                <p className="truncate text-sm font-medium text-gray-900">{displayName}</p>
                <p className="truncate text-xs text-gray-500">{user?.email}</p>
              </div>
              <Link
                href="/settings"
                role="menuitem"
                className={`flex items-center gap-2.5 px-3 py-2 text-sm text-gray-700 hover:bg-gray-50 ${FOCUS_RING}`}
              >
                <User size={16} className="text-gray-500" aria-hidden />
                Hồ sơ cá nhân
              </Link>
              <Link
                href="/settings"
                role="menuitem"
                className={`flex items-center gap-2.5 px-3 py-2 text-sm text-gray-700 hover:bg-gray-50 ${FOCUS_RING}`}
              >
                <Settings size={16} className="text-gray-500" aria-hidden />
                Cài đặt
              </Link>
              <div className="my-1 border-t border-gray-200" />
              <button
                type="button"
                role="menuitem"
                onClick={handleLogout}
                className={`flex w-full items-center gap-2.5 px-3 py-2 text-sm text-red-700 hover:bg-red-50 ${FOCUS_RING}`}
              >
                <LogOut size={16} aria-hidden />
                Đăng xuất
              </button>
            </div>
          )}
        </div>
      </div>
    </header>
  );
}
