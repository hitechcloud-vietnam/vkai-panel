'use client';

import { useEffect, useMemo, useRef, useState } from 'react';
import Link from 'next/link';
import { usePathname, useRouter } from 'next/navigation';
import { Bell, ChevronDown, LogOut, Search, Settings, User } from 'lucide-react';
import { useAuthStore } from '@/store/auth';
import { brand } from '@/lib/brand';

const SEGMENT_LABELS: Record<string, string> = {
  dashboard: 'Bảng điều khiển',
  servers: 'Máy chủ',
  add: 'Thêm mới',
  clusters: 'Cụm & HA',
  docker: 'Docker',
  monitoring: 'Giám sát',
  websites: 'Website',
  wordpress: 'WordPress',
  dns: 'DNS',
  ssl: 'SSL',
  databases: 'Cơ sở dữ liệu',
  'mail-server': 'Máy chủ mail',
  security: 'Bảo mật',
  waf: 'WAF Pro',
  firewall: 'Tường lửa',
  'file-protection': 'Bảo vệ tệp tin',
  'tamper-proof': 'Chống giả mạo',
  audit: 'Nhật ký kiểm toán',
  backups: 'Sao lưu',
  cron: 'Cron',
  'scheduled-tasks': 'Tác vụ định kỳ',
  jobs: 'Hàng đợi tác vụ',
  deployments: 'Triển khai',
  config: 'Khôi phục cấu hình',
  terminal: 'Terminal',
  logs: 'Nhật ký hệ thống',
  'website-stats': 'Thống kê website',
  'email-marketing': 'Email Marketing',
  'daily-reports': 'Báo cáo hằng ngày',
  users: 'Người dùng',
  'api-keys': 'API Keys',
  notifications: 'Thông báo',
  settings: 'Cài đặt',
};

function labelFor(segment: string) {
  if (SEGMENT_LABELS[segment]) return SEGMENT_LABELS[segment];
  return segment
    .split('-')
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join(' ');
}

export default function Header() {
  const pathname = usePathname();
  const router = useRouter();
  const { user, logout } = useAuthStore();
  const [showUserMenu, setShowUserMenu] = useState(false);
  const menuRef = useRef<HTMLDivElement>(null);

  const crumbs = useMemo(() => {
    const segments = (pathname || '').split('/').filter(Boolean);
    return segments.map((segment, index) => ({
      label: labelFor(segment),
      href: '/' + segments.slice(0, index + 1).join('/'),
    }));
  }, [pathname]);

  useEffect(() => {
    if (!showUserMenu) return;
    const onClickOutside = (event: MouseEvent) => {
      if (menuRef.current && !menuRef.current.contains(event.target as Node)) {
        setShowUserMenu(false);
      }
    };
    document.addEventListener('mousedown', onClickOutside);
    return () => document.removeEventListener('mousedown', onClickOutside);
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
    <header className="sticky top-0 z-30 flex h-16 shrink-0 items-center justify-between gap-4 border-b border-gray-200 bg-white px-6">
      {/* Breadcrumb */}
      <nav aria-label="Breadcrumb" className="min-w-0 flex-1">
        <ol className="flex items-center gap-1.5 truncate text-sm">
          <li className="shrink-0">
            <Link
              href="/dashboard"
              className="rounded text-gray-500 hover:text-gray-900 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500"
            >
              {brand.productName}
            </Link>
          </li>
          {crumbs.map((crumb, index) => {
            const isLast = index === crumbs.length - 1;
            return (
              <li key={crumb.href} className="flex min-w-0 items-center gap-1.5">
                <span className="text-gray-300" aria-hidden>
                  /
                </span>
                {isLast ? (
                  <span className="truncate font-medium text-gray-900" aria-current="page">
                    {crumb.label}
                  </span>
                ) : (
                  <Link
                    href={crumb.href}
                    className="truncate rounded text-gray-500 hover:text-gray-900 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500"
                  >
                    {crumb.label}
                  </Link>
                )}
              </li>
            );
          })}
        </ol>
      </nav>

      <div className="flex shrink-0 items-center gap-2">
        {/* Tim kiem */}
        <div className="relative hidden md:block">
          <Search
            size={15}
            className="pointer-events-none absolute left-2.5 top-1/2 -translate-y-1/2 text-gray-400"
          />
          <input
            type="search"
            placeholder="Tìm kiếm…"
            aria-label="Tìm kiếm"
            className="w-56 rounded-md border border-gray-300 bg-white py-1.5 pl-8 pr-3 text-sm text-gray-900 placeholder:text-gray-400 focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
          />
        </div>

        {/* Thong bao */}
        <Link
          href="/notifications"
          aria-label="Thông báo"
          className="relative rounded-md p-2 text-gray-500 hover:bg-gray-100 hover:text-gray-900 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500"
        >
          <Bell size={18} />
        </Link>

        {/* Menu nguoi dung */}
        <div className="relative" ref={menuRef}>
          <button
            type="button"
            onClick={() => setShowUserMenu((prev) => !prev)}
            aria-haspopup="menu"
            aria-expanded={showUserMenu}
            className="flex items-center gap-2 rounded-md px-2 py-1.5 hover:bg-gray-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500"
          >
            <span className="flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-blue-600 text-sm font-medium text-white">
              {initial}
            </span>
            <span className="hidden text-left lg:block">
              <span className="block text-sm font-medium leading-tight text-gray-900">
                {displayName}
              </span>
              <span className="block text-xs leading-tight text-gray-500">{user?.email}</span>
            </span>
            <ChevronDown size={14} className="hidden text-gray-400 lg:block" />
          </button>

          {showUserMenu && (
            <div
              role="menu"
              className="absolute right-0 top-full z-50 mt-2 w-56 rounded-md border border-gray-200 bg-white py-1 shadow-lg"
            >
              <div className="border-b border-gray-200 px-3 py-2">
                <p className="truncate text-sm font-medium text-gray-900">{displayName}</p>
                <p className="truncate text-xs text-gray-500">{user?.email}</p>
              </div>
              <Link
                href="/settings"
                role="menuitem"
                className="flex items-center gap-2.5 px-3 py-2 text-sm text-gray-700 hover:bg-gray-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500"
              >
                <User size={16} className="text-gray-500" />
                Hồ sơ cá nhân
              </Link>
              <Link
                href="/settings"
                role="menuitem"
                className="flex items-center gap-2.5 px-3 py-2 text-sm text-gray-700 hover:bg-gray-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500"
              >
                <Settings size={16} className="text-gray-500" />
                Cài đặt
              </Link>
              <div className="my-1 border-t border-gray-200" />
              <button
                type="button"
                role="menuitem"
                onClick={handleLogout}
                className="flex w-full items-center gap-2.5 px-3 py-2 text-sm text-red-700 hover:bg-red-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500"
              >
                <LogOut size={16} />
                Đăng xuất
              </button>
            </div>
          )}
        </div>
      </div>
    </header>
  );
}
