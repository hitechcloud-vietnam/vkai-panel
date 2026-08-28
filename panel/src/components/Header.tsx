'use client';

import { useEffect, useRef, useState } from 'react';
import Link from 'next/link';
import { usePathname, useRouter } from 'next/navigation';
import { Bell, ChevronDown, LogOut, Search, Settings, User } from 'lucide-react';
import { useAuthStore } from '@/store/auth';
import { monitoringApi, unwrap } from '@/services/api';
import { brand, versionLabel } from '@/lib/brand';
import { EMPTY_VALUE, LanguageSwitcher, useT } from '@/i18n';

/** The focus ring shared by every interactive element in the top bar. */
const FOCUS_RING =
  'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500';

/**
 * The shape returned by GET /api/v1/monitoring/system.
 * Only the fields the top bar actually reads are declared.
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
  const t = useT();
  const [showUserMenu, setShowUserMenu] = useState(false);
  const [osLabel, setOsLabel] = useState<string | null>(null);
  const menuRef = useRef<HTMLDivElement>(null);
  const menuButtonRef = useRef<HTMLButtonElement>(null);

  // The operating system name comes from the existing monitoring endpoint.
  // That endpoint does not report uptime yet, so the uptime slot shows "—".
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
        // No permission, or the API failed: keep "—" rather than invent a value.
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
    [user?.first_name, user?.last_name].filter(Boolean).join(' ') ||
    user?.username ||
    t('header.defaultUserName');
  const initial = (user?.first_name || user?.username || 'U').charAt(0).toUpperCase();

  const handleLogout = () => {
    setShowUserMenu(false);
    logout();
    router.replace('/login');
  };

  return (
    <header className="flex h-14 shrink-0 items-center justify-between gap-4 border-b border-gray-200 bg-white px-4">
      {/* Left: panel name, operating system, uptime */}
      <div className="flex min-w-0 items-center gap-3 text-sm">
        <Link
          href="/dashboard"
          className={`shrink-0 rounded font-semibold text-gray-900 hover:text-brand-700 ${FOCUS_RING}`}
        >
          {brand.productName}
        </Link>

        <span className="hidden h-4 w-px shrink-0 bg-gray-200 sm:block" aria-hidden />

        <p className="hidden min-w-0 items-center gap-1.5 sm:flex">
          <span className="shrink-0 text-gray-500">{t('header.operatingSystem')}</span>
          <span className="truncate text-gray-700">{osLabel || EMPTY_VALUE}</span>
        </p>

        <span className="hidden h-4 w-px shrink-0 bg-gray-200 lg:block" aria-hidden />

        <p className="hidden items-center gap-1.5 lg:flex">
          <span className="shrink-0 text-gray-500">{t('header.uptime')}</span>
          <span className="text-gray-700" title={t('header.uptimeUnavailable')}>
            {EMPTY_VALUE}
          </span>
        </p>
      </div>

      {/* Right: version, search, language, notifications, user */}
      <div className="flex shrink-0 items-center gap-2">
        <span className="hidden rounded border border-brand-200 bg-brand-50 px-2 py-0.5 text-xs font-medium text-brand-700 sm:inline">
          {versionLabel}
        </span>

        {/* Search */}
        <div className="relative hidden md:block">
          <label htmlFor="header-search" className="sr-only">
            {t('common.search')}
          </label>
          <Search
            size={15}
            className="pointer-events-none absolute left-2.5 top-1/2 -translate-y-1/2 text-gray-400"
            aria-hidden
          />
          <input
            id="header-search"
            type="search"
            placeholder={t('header.searchPlaceholder')}
            className="w-48 rounded-md border border-gray-300 bg-white py-1.5 pl-8 pr-3 text-sm text-gray-900 placeholder:text-gray-500 focus:border-brand-500 focus:outline-none focus:ring-1 focus:ring-brand-500"
          />
        </div>

        {/* Language */}
        <LanguageSwitcher className="hidden sm:block" />

        {/* Notifications */}
        <Link
          href="/notifications"
          aria-label={t('header.notifications')}
          title={t('header.notifications')}
          className={`rounded-md p-2 text-gray-500 hover:bg-gray-100 hover:text-gray-900 ${FOCUS_RING}`}
        >
          <Bell size={18} aria-hidden />
        </Link>

        {/* User menu */}
        <div className="relative" ref={menuRef}>
          <button
            ref={menuButtonRef}
            type="button"
            onClick={() => setShowUserMenu((prev) => !prev)}
            aria-haspopup="menu"
            aria-expanded={showUserMenu}
            aria-label={t('header.accountMenu', { name: displayName })}
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
              aria-label={t('header.userMenu')}
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
                {t('header.profile')}
              </Link>
              <Link
                href="/settings"
                role="menuitem"
                className={`flex items-center gap-2.5 px-3 py-2 text-sm text-gray-700 hover:bg-gray-50 ${FOCUS_RING}`}
              >
                <Settings size={16} className="text-gray-500" aria-hidden />
                {t('header.settings')}
              </Link>
              <div className="my-1 border-t border-gray-200" />
              <button
                type="button"
                role="menuitem"
                onClick={handleLogout}
                className={`flex w-full items-center gap-2.5 px-3 py-2 text-sm text-red-700 hover:bg-red-50 ${FOCUS_RING}`}
              >
                <LogOut size={16} aria-hidden />
                {t('header.signOut')}
              </button>
            </div>
          )}
        </div>
      </div>
    </header>
  );
}
