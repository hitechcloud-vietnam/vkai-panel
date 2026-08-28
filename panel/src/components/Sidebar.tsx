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
  /**
   * Short qualifier shown beside the label, for a section that is not part of
   * the ordinary path through the product. "Tùy chọn" on Cụm & HA is the only
   * user of this today: clustering is a layer an operator adds when a fleet
   * grows, not a step towards a working install, and an unqualified menu item
   * reads as a requirement.
   */
  badge?: string;
  /** Longer explanation, shown on hover. */
  badgeTitle?: string;
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
      {
        label: 'Cụm & HA',
        icon: <Network size={18} />,
        href: '/clusters',
        badge: 'Tùy chọn',
        badgeTitle:
          'Lớp tùy chọn cho nhiều máy. Panel đã quản lý sẵn máy mà nó đang chạy, nên bạn không cần cụm để tạo website.',
      },
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

/** Vien focus dung chung cho moi phan tu tuong tac trong sidebar. */
const FOCUS_RING =
  'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-brand-500';

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

  /** Thanh chi bao 2px ben trai cho muc dang chon. */
  const activeBar = <span className="absolute inset-y-0 left-0 w-0.5 bg-brand-600" aria-hidden />;

  const renderItem = (item: MenuItem) => {
    const active = isItemActive(item);
    const hasChildren = Boolean(item.children && item.children.length > 0);
    const isExpanded = Boolean(expanded[item.label]);

    // Che do thu gon: chi hien icon, moi muc van la mot hang cao 44px.
    if (collapsed) {
      const href = item.href || item.children?.[0]?.href || '#';
      return (
        <Link
          key={item.label}
          href={href}
          title={item.badge ? `${item.label} (${item.badge})` : item.label}
          aria-label={item.badge ? `${item.label} (${item.badge})` : item.label}
          aria-current={active ? 'page' : undefined}
          className={`relative flex h-11 items-center justify-center border-b border-gray-100 ${FOCUS_RING} ${
            active ? 'bg-brand-50 text-brand-700' : 'text-gray-500 hover:bg-gray-50 hover:text-gray-900'
          }`}
        >
          {active && activeBar}
          <span className={active ? 'text-brand-600' : 'text-gray-500'}>{item.icon}</span>
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
            className={`relative flex h-11 w-full items-center gap-2.5 border-b border-gray-100 px-4 text-sm ${FOCUS_RING} ${
              active ? 'bg-brand-50 font-medium text-brand-700' : 'text-gray-700 hover:bg-gray-50'
            }`}
          >
            {active && activeBar}
            <span className={active ? 'text-brand-600' : 'text-gray-500'}>{item.icon}</span>
            <span className="flex-1 truncate text-left">{item.label}</span>
            {isExpanded ? (
              <ChevronDown size={14} className="shrink-0 text-gray-400" aria-hidden />
            ) : (
              <ChevronRight size={14} className="shrink-0 text-gray-400" aria-hidden />
            )}
          </button>

          {isExpanded && item.children && (
            <div className="bg-gray-50/60">
              {item.children.map((child) => {
                const childActive = child.href === pathname;
                return (
                  <Link
                    key={child.href}
                    href={child.href}
                    aria-current={childActive ? 'page' : undefined}
                    className={`relative flex h-9 items-center border-b border-gray-100 pl-11 pr-4 text-sm ${FOCUS_RING} ${
                      childActive
                        ? 'bg-brand-50 font-medium text-brand-700'
                        : 'text-gray-600 hover:bg-gray-100 hover:text-gray-900'
                    }`}
                  >
                    {childActive && activeBar}
                    <span className="truncate">{child.label}</span>
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
        aria-current={active ? 'page' : undefined}
        className={`relative flex h-11 items-center gap-2.5 border-b border-gray-100 px-4 text-sm ${FOCUS_RING} ${
          active ? 'bg-brand-50 font-medium text-brand-700' : 'text-gray-700 hover:bg-gray-50'
        }`}
      >
        {active && activeBar}
        <span className={active ? 'text-brand-600' : 'text-gray-500'}>{item.icon}</span>
        <span className="truncate">{item.label}</span>
        {item.badge && (
          <span
            title={item.badgeTitle}
            className="ml-auto shrink-0 rounded-md border border-gray-200 bg-gray-100 px-1.5 py-0.5 text-[10px] font-medium uppercase tracking-wide text-gray-600"
          >
            {item.badge}
          </span>
        )}
      </Link>
    );
  };

  return (
    <aside
      className={`flex h-full shrink-0 flex-col border-r border-gray-200 bg-white ${
        collapsed ? 'w-16' : 'w-[236px]'
      }`}
      aria-label="Điều hướng chính"
    >
      {/* Khoi thuong hieu */}
      <div
        className={`flex h-14 shrink-0 items-center border-b border-gray-200 ${
          collapsed ? 'justify-center px-2' : 'px-4'
        }`}
      >
        <Link
          href="/dashboard"
          aria-label={`${brand.productName} — về bảng điều khiển`}
          className={`flex min-w-0 items-center gap-2.5 rounded-md ${FOCUS_RING}`}
        >
          {/* Logo lay tu /public/logo.svg; khi thu gon chi hien phan bieu trung ben trai. */}
          {/* eslint-disable-next-line @next/next/no-img-element */}
          <img
            src="/logo.svg"
            alt=""
            aria-hidden="true"
            width={32}
            height={32}
            className="h-8 w-8 max-w-none shrink-0 rounded-md object-cover object-left"
          />
          {!collapsed && (
            <span className="min-w-0">
              <span className="block truncate text-sm font-semibold leading-tight text-gray-900">
                {brand.productName}
              </span>
              <span className="block truncate text-[11px] leading-tight text-gray-500">
                {brand.company}
              </span>
            </span>
          )}
        </Link>
      </div>

      {/* Dieu huong - vung cuon rieng */}
      <nav className="min-h-0 flex-1 overflow-y-auto">
        {menuGroups.map((group) => (
          <div key={group.label}>
            {collapsed ? (
              <div className="border-b-2 border-gray-200" aria-hidden />
            ) : (
              <p className="border-b border-gray-100 bg-gray-50 px-4 py-1.5 text-[11px] font-semibold uppercase tracking-wider text-gray-500">
                {group.label}
              </p>
            )}
            {group.items.map((item) => renderItem(item))}
          </div>
        ))}
      </nav>

      {/* Chan sidebar: phien ban + nut thu gon */}
      <div
        className={`flex h-12 shrink-0 items-center gap-2 border-t border-gray-200 px-2 ${
          collapsed ? 'justify-center' : 'justify-between px-4'
        }`}
      >
        {!collapsed && (
          <span className="truncate text-xs text-gray-500">
            {brand.productName} {versionLabel}
          </span>
        )}
        <button
          type="button"
          onClick={toggleCollapsed}
          title={collapsed ? 'Mở rộng menu' : 'Thu gọn menu'}
          aria-label={collapsed ? 'Mở rộng menu' : 'Thu gọn menu'}
          className={`rounded-md p-1.5 text-gray-500 hover:bg-gray-100 hover:text-gray-900 ${FOCUS_RING}`}
        >
          {collapsed ? <PanelLeftOpen size={16} /> : <PanelLeftClose size={16} />}
        </button>
      </div>
    </aside>
  );
}
