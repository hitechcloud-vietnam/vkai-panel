'use client';

import Link from 'next/link';
import { usePathname } from 'next/navigation';
import { useEffect, useState } from 'react';
import {
  Activity,
  BarChart3,
  ChevronDown,
  ChevronRight,
  Clock,
  Container,
  Database,
  FileCheck2,
  FileText,
  FileWarning,
  Flame,
  Gauge,
  GitBranch,
  Globe,
  HardDrive,
  Key,
  LayoutDashboard,
  ListTree,
  Lock,
  Mail,
  Megaphone,
  Network,
  PanelLeftClose,
  PanelLeftOpen,
  ScrollText,
  Server,
  Settings,
  Shield,
  ShieldCheck,
  Terminal,
  Users,
} from 'lucide-react';
import { brand, versionLabel } from '@/lib/brand';

const SIDEBAR_STORAGE_KEY = 'vkai_sidebar_collapsed';

interface MenuItem {
  label: string;
  icon: React.ReactNode;
  href?: string;
  children?: { label: string; href: string }[];
}

interface MenuGroup {
  label: string;
  items: MenuItem[];
}

const menuGroups: MenuGroup[] = [
  {
    label: 'Tổng quan',
    items: [
      { label: 'Bảng điều khiển', icon: <LayoutDashboard size={18} />, href: '/dashboard' },
    ],
  },
  {
    label: 'Hạ tầng',
    items: [
      {
        label: 'Máy chủ',
        icon: <Server size={18} />,
        children: [
          { label: 'Danh sách máy chủ', href: '/servers' },
          { label: 'Thêm máy chủ', href: '/servers/add' },
        ],
      },
      { label: 'Cụm & HA', icon: <Network size={18} />, href: '/clusters' },
      { label: 'Docker', icon: <Container size={18} />, href: '/docker' },
      { label: 'Giám sát', icon: <Gauge size={18} />, href: '/monitoring' },
    ],
  },
  {
    label: 'Web',
    items: [
      {
        label: 'Website',
        icon: <Globe size={18} />,
        children: [
          { label: 'Tất cả website', href: '/websites' },
          { label: 'WordPress', href: '/websites/wordpress' },
        ],
      },
      { label: 'DNS', icon: <ListTree size={18} />, href: '/dns' },
      { label: 'SSL', icon: <Lock size={18} />, href: '/ssl' },
      { label: 'Cơ sở dữ liệu', icon: <Database size={18} />, href: '/databases' },
      { label: 'Máy chủ mail', icon: <Mail size={18} />, href: '/mail-server' },
    ],
  },
  {
    label: 'Bảo mật',
    items: [
      { label: 'Bảo mật', icon: <Shield size={18} />, href: '/security' },
      { label: 'WAF Pro', icon: <ShieldCheck size={18} />, href: '/waf' },
      { label: 'Tường lửa', icon: <Flame size={18} />, href: '/firewall' },
      { label: 'Bảo vệ tệp tin', icon: <FileCheck2 size={18} />, href: '/file-protection' },
      { label: 'Chống giả mạo', icon: <FileWarning size={18} />, href: '/tamper-proof' },
      { label: 'Nhật ký kiểm toán', icon: <ScrollText size={18} />, href: '/audit' },
    ],
  },
  {
    label: 'Vận hành',
    items: [
      { label: 'Sao lưu', icon: <HardDrive size={18} />, href: '/backups' },
      { label: 'Cron', icon: <Clock size={18} />, href: '/cron' },
      { label: 'Tác vụ định kỳ', icon: <Clock size={18} />, href: '/scheduled-tasks' },
      { label: 'Hàng đợi tác vụ', icon: <Activity size={18} />, href: '/jobs' },
      { label: 'Triển khai', icon: <GitBranch size={18} />, href: '/deployments' },
      { label: 'Khôi phục cấu hình', icon: <FileText size={18} />, href: '/config' },
      { label: 'Terminal', icon: <Terminal size={18} />, href: '/terminal' },
      { label: 'Nhật ký hệ thống', icon: <FileText size={18} />, href: '/logs' },
    ],
  },
  {
    label: 'Kinh doanh',
    items: [
      { label: 'Thống kê website', icon: <BarChart3 size={18} />, href: '/website-stats' },
      { label: 'Email Marketing', icon: <Megaphone size={18} />, href: '/email-marketing' },
      { label: 'Báo cáo hằng ngày', icon: <FileText size={18} />, href: '/daily-reports' },
    ],
  },
  {
    label: 'Hệ thống',
    items: [
      { label: 'Người dùng', icon: <Users size={18} />, href: '/users' },
      { label: 'API Keys', icon: <Key size={18} />, href: '/api-keys' },
      { label: 'Thông báo', icon: <Activity size={18} />, href: '/notifications' },
      { label: 'Cài đặt', icon: <Settings size={18} />, href: '/settings' },
    ],
  },
];

export default function Sidebar() {
  const pathname = usePathname();
  const [collapsed, setCollapsed] = useState(false);
  const [expanded, setExpanded] = useState<Record<string, boolean>>({});

  // Doc trang thai thu gon trong useEffect de tranh hydration mismatch.
  useEffect(() => {
    try {
      setCollapsed(window.localStorage.getItem(SIDEBAR_STORAGE_KEY) === '1');
    } catch {
      setCollapsed(false);
    }
  }, []);

  // Tu dong mo nhom menu chua trang dang xem.
  useEffect(() => {
    const next: Record<string, boolean> = {};
    menuGroups.forEach((group) => {
      group.items.forEach((item) => {
        if (item.children?.some((child) => child.href === pathname)) {
          next[item.label] = true;
        }
      });
    });
    if (Object.keys(next).length > 0) {
      setExpanded((prev) => ({ ...prev, ...next }));
    }
  }, [pathname]);

  const toggleCollapsed = () => {
    setCollapsed((prev) => {
      const next = !prev;
      try {
        window.localStorage.setItem(SIDEBAR_STORAGE_KEY, next ? '1' : '0');
      } catch {
        /* bo qua khi trinh duyet chan localStorage */
      }
      return next;
    });
  };

  const toggleExpand = (label: string) => {
    setExpanded((prev) => ({ ...prev, [label]: !prev[label] }));
  };

  const isItemActive = (item: MenuItem) =>
    item.href === pathname || Boolean(item.children?.some((child) => child.href === pathname));

  const renderItem = (item: MenuItem) => {
    const active = isItemActive(item);
    const hasChildren = Boolean(item.children && item.children.length > 0);
    const isExpanded = Boolean(expanded[item.label]);

    if (collapsed) {
      const href = item.href || item.children?.[0]?.href || '#';
      return (
        <Link
          key={item.label}
          href={href}
          title={item.label}
          className={`relative flex items-center justify-center rounded-md py-2 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500 ${
            active ? 'bg-blue-50 text-blue-700' : 'text-gray-500 hover:bg-gray-50 hover:text-gray-900'
          }`}
        >
          {active && (
            <span className="absolute inset-y-1 left-0 w-0.5 rounded-r bg-blue-600" aria-hidden />
          )}
          <span className={active ? 'text-blue-600' : 'text-gray-500'}>{item.icon}</span>
        </Link>
      );
    }

    if (hasChildren) {
      return (
        <div key={item.label}>
          <button
            type="button"
            onClick={() => toggleExpand(item.label)}
            aria-expanded={isExpanded}
            className={`relative flex w-full items-center gap-2.5 rounded-md px-3 py-2 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500 ${
              active
                ? 'bg-blue-50 font-medium text-blue-700'
                : 'text-gray-700 hover:bg-gray-50'
            }`}
          >
            {active && (
              <span className="absolute inset-y-1 left-0 w-0.5 rounded-r bg-blue-600" aria-hidden />
            )}
            <span className={active ? 'text-blue-600' : 'text-gray-500'}>{item.icon}</span>
            <span className="flex-1 truncate text-left">{item.label}</span>
            {isExpanded ? (
              <ChevronDown size={14} className="text-gray-400" />
            ) : (
              <ChevronRight size={14} className="text-gray-400" />
            )}
          </button>

          {isExpanded && item.children && (
            <div className="ml-[1.375rem] mt-1 space-y-0.5 border-l border-gray-200 pl-3">
              {item.children.map((child) => {
                const childActive = child.href === pathname;
                return (
                  <Link
                    key={child.href}
                    href={child.href}
                    className={`block truncate rounded-md px-3 py-1.5 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500 ${
                      childActive
                        ? 'bg-blue-50 font-medium text-blue-700'
                        : 'text-gray-600 hover:bg-gray-50 hover:text-gray-900'
                    }`}
                  >
                    {child.label}
                  </Link>
                );
              })}
            </div>
          )}
        </div>
      );
    }

    return (
      <Link
        key={item.label}
        href={item.href || '#'}
        className={`relative flex items-center gap-2.5 rounded-md px-3 py-2 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500 ${
          active ? 'bg-blue-50 font-medium text-blue-700' : 'text-gray-700 hover:bg-gray-50'
        }`}
      >
        {active && (
          <span className="absolute inset-y-1 left-0 w-0.5 rounded-r bg-blue-600" aria-hidden />
        )}
        <span className={active ? 'text-blue-600' : 'text-gray-500'}>{item.icon}</span>
        <span className="truncate">{item.label}</span>
      </Link>
    );
  };

  return (
    <aside
      className={`flex shrink-0 flex-col border-r border-gray-200 bg-white ${
        collapsed ? 'w-16' : 'w-64'
      }`}
    >
      {/* Thuong hieu */}
      <div
        className={`flex h-16 shrink-0 items-center border-b border-gray-200 ${
          collapsed ? 'justify-center px-2' : 'px-4'
        }`}
      >
        <Link
          href="/dashboard"
          className="flex items-center gap-2.5 rounded-md focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500"
        >
          <span
            className="flex h-8 w-8 shrink-0 items-center justify-center rounded-md"
            style={{ backgroundColor: brand.colors.navy }}
          >
            <svg viewBox="0 0 64 64" width="20" height="20" aria-hidden="true" focusable="false">
              <path
                d="M46 18 L32 46"
                fill="none"
                stroke={brand.colors.cyan}
                strokeWidth="8"
                strokeLinecap="round"
              />
              <path
                d="M18 18 L32 46"
                fill="none"
                stroke="#FFFFFF"
                strokeWidth="8"
                strokeLinecap="round"
              />
            </svg>
          </span>
          {!collapsed && (
            <span className="min-w-0">
              <span className="block text-sm font-semibold leading-tight text-gray-900">
                {brand.productName}
              </span>
              <span className="block text-[11px] leading-tight text-gray-500">
                {brand.company}
              </span>
            </span>
          )}
        </Link>
      </div>

      {/* Dieu huong */}
      <nav className="flex-1 overflow-y-auto px-2 py-3">
        {menuGroups.map((group) => (
          <div key={group.label} className="mb-4 last:mb-0">
            {collapsed ? (
              <div className="mx-2 mb-2 border-t border-gray-200 first:border-t-0" aria-hidden />
            ) : (
              <p className="px-3 pb-1.5 text-[11px] font-semibold uppercase tracking-wider text-gray-400">
                {group.label}
              </p>
            )}
            <div className="space-y-0.5">{group.items.map((item) => renderItem(item))}</div>
          </div>
        ))}
      </nav>

      {/* Chan sidebar */}
      <div
        className={`flex shrink-0 items-center gap-2 border-t border-gray-200 px-2 py-3 ${
          collapsed ? 'justify-center' : 'justify-between px-3'
        }`}
      >
        {!collapsed && (
          <span className="text-xs text-gray-500">
            {brand.productName} {versionLabel}
          </span>
        )}
        <button
          type="button"
          onClick={toggleCollapsed}
          title={collapsed ? 'Mở rộng menu' : 'Thu gọn menu'}
          aria-label={collapsed ? 'Mở rộng menu' : 'Thu gọn menu'}
          className="rounded-md p-1.5 text-gray-500 hover:bg-gray-100 hover:text-gray-900 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500"
        >
          {collapsed ? <PanelLeftOpen size={16} /> : <PanelLeftClose size={16} />}
        </button>
      </div>
    </aside>
  );
}
