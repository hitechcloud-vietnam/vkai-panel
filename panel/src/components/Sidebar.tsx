'use client';

import Link from 'next/link';
import { usePathname } from 'next/navigation';
import { useEffect, useState } from 'react';
import {
  FolderTree,
  ArrowLeftRight,
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
import { useT } from '@/i18n';

const SIDEBAR_STORAGE_KEY = 'vkai_sidebar_collapsed';

/**
 * The menu carries translation KEYS, not text. It is module-level data, built
 * once outside any component, so it cannot call the hook; every key is resolved
 * with t() at the point it is rendered.
 */
interface MenuItem {
  /** Translation key for the item label, e.g. 'nav.dashboard'. */
  labelKey: string;
  icon: React.ReactNode;
  href?: string;
  children?: { labelKey: string; href: string }[];
  /**
   * Short qualifier shown beside the label, for a section that is not part of
   * the ordinary path through the product. "Optional" on Clusters & HA is the
   * only user of this today: clustering is a layer an operator adds when a
   * fleet grows, not a step towards a working install, and an unqualified menu
   * item reads as a requirement.
   */
  badgeKey?: string;
  /** Key for the longer explanation, shown on hover. */
  badgeTitleKey?: string;
}

interface MenuGroup {
  labelKey: string;
  items: MenuItem[];
}

const menuGroups: MenuGroup[] = [
  {
    labelKey: 'nav.group.overview',
    items: [
      { labelKey: 'nav.dashboard', icon: <LayoutDashboard size={18} />, href: '/dashboard' },
    ],
  },
  {
    labelKey: 'nav.group.infrastructure',
    items: [
      {
        labelKey: 'nav.servers',
        icon: <Server size={18} />,
        children: [
          { labelKey: 'nav.serversList', href: '/servers' },
          { labelKey: 'nav.serversAdd', href: '/servers/add' },
        ],
      },
      {
        labelKey: 'nav.clusters',
        icon: <Network size={18} />,
        href: '/clusters',
        badgeKey: 'nav.clustersBadge',
        badgeTitleKey: 'nav.clustersBadgeTitle',
      },
      { labelKey: 'nav.docker', icon: <Container size={18} />, href: '/docker' },
      { labelKey: 'nav.monitoring', icon: <Gauge size={18} />, href: '/monitoring' },
    ],
  },
  {
    labelKey: 'nav.group.web',
    items: [
      {
        labelKey: 'nav.websites',
        icon: <Globe size={18} />,
        children: [
          { labelKey: 'nav.websitesAll', href: '/websites' },
          { labelKey: 'nav.wpToolkit', href: '/wp-toolkit' },
        ],
      },
      { labelKey: 'nav.dns', icon: <ListTree size={18} />, href: '/dns' },
      { labelKey: 'nav.ssl', icon: <Lock size={18} />, href: '/ssl' },
      { labelKey: 'nav.databases', icon: <Database size={18} />, href: '/databases' },
      { labelKey: 'nav.files', icon: <FolderTree size={18} />, href: '/files' },
      { labelKey: 'nav.ftp', icon: <ArrowLeftRight size={18} />, href: '/ftp' },
      { labelKey: 'nav.emailServer', icon: <Mail size={18} />, href: '/email-server' },
    ],
  },
  {
    labelKey: 'nav.group.security',
    items: [
      { labelKey: 'nav.security', icon: <Shield size={18} />, href: '/security' },
      { labelKey: 'nav.waf', icon: <ShieldCheck size={18} />, href: '/waf' },
      { labelKey: 'nav.firewall', icon: <Flame size={18} />, href: '/firewall' },
      { labelKey: 'nav.fileProtection', icon: <FileCheck2 size={18} />, href: '/file-protection' },
      { labelKey: 'nav.tamperProof', icon: <FileWarning size={18} />, href: '/tamper-proof' },
    ],
  },
  {
    labelKey: 'nav.group.operations',
    items: [
      { labelKey: 'nav.backups', icon: <HardDrive size={18} />, href: '/backups' },
      { labelKey: 'nav.cron', icon: <Clock size={18} />, href: '/cron' },
      { labelKey: 'nav.scheduledTasks', icon: <Clock size={18} />, href: '/scheduled-tasks' },
      { labelKey: 'nav.jobQueue', icon: <Activity size={18} />, href: '/jobs' },
      { labelKey: 'nav.deployments', icon: <GitBranch size={18} />, href: '/deployments' },
      { labelKey: 'nav.configRestore', icon: <FileText size={18} />, href: '/config' },
      { labelKey: 'nav.terminal', icon: <Terminal size={18} />, href: '/terminal' },
      { labelKey: 'nav.logs', icon: <FileText size={18} />, href: '/logs' },
    ],
  },
  {
    labelKey: 'nav.group.business',
    items: [
      { labelKey: 'nav.websiteStats', icon: <BarChart3 size={18} />, href: '/website-stats' },
      { labelKey: 'nav.dailyReports', icon: <FileText size={18} />, href: '/daily-reports' },
    ],
  },
  {
    labelKey: 'nav.group.system',
    items: [
      { labelKey: 'nav.users', icon: <Users size={18} />, href: '/users' },
      { labelKey: 'nav.apiKeys', icon: <Key size={18} />, href: '/api-keys' },
      { labelKey: 'nav.notifications', icon: <Activity size={18} />, href: '/notifications' },
      { labelKey: 'nav.settings', icon: <Settings size={18} />, href: '/settings' },
    ],
  },
];

/** The focus ring shared by every interactive element in the sidebar. */
const FOCUS_RING =
  'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-brand-500';

export default function Sidebar() {
  const pathname = usePathname();
  const t = useT();
  const [collapsed, setCollapsed] = useState(false);
  const [expanded, setExpanded] = useState<Record<string, boolean>>({});

  // The collapsed state is read in an effect, not during render, so the
  // server's markup and the browser's first render cannot disagree.
  useEffect(() => {
    try {
      setCollapsed(window.localStorage.getItem(SIDEBAR_STORAGE_KEY) === '1');
    } catch {
      setCollapsed(false);
    }
  }, []);

  // Opens the menu group that holds the page being viewed.
  useEffect(() => {
    const next: Record<string, boolean> = {};
    menuGroups.forEach((group) => {
      group.items.forEach((item) => {
        if (item.children?.some((child) => child.href === pathname)) {
          next[item.labelKey] = true;
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
        /* storage blocked by the browser: the state still applies for this page load */
      }
      return next;
    });
  };

  const toggleExpand = (labelKey: string) => {
    setExpanded((prev) => ({ ...prev, [labelKey]: !prev[labelKey] }));
  };

  const isItemActive = (item: MenuItem) =>
    item.href === pathname || Boolean(item.children?.some((child) => child.href === pathname));

  /** The 2px indicator down the left edge of the selected item. */
  const activeBar = <span className="absolute inset-y-0 left-0 w-0.5 bg-brand-600" aria-hidden />;

  const renderItem = (item: MenuItem) => {
    const active = isItemActive(item);
    const hasChildren = Boolean(item.children && item.children.length > 0);
    const isExpanded = Boolean(expanded[item.labelKey]);
    const label = t(item.labelKey);
    const badge = item.badgeKey ? t(item.badgeKey) : undefined;
    const labelWithBadge = badge ? `${label} (${badge})` : label;

    // Collapsed: icons only, each item still one 44px row.
    if (collapsed) {
      const href = item.href || item.children?.[0]?.href || '#';
      return (
        <Link
          key={item.labelKey}
          href={href}
          title={labelWithBadge}
          aria-label={labelWithBadge}
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
        <div key={item.labelKey}>
          <button
            type="button"
            onClick={() => toggleExpand(item.labelKey)}
            aria-expanded={isExpanded}
            className={`relative flex h-11 w-full items-center gap-2.5 border-b border-gray-100 px-4 text-sm ${FOCUS_RING} ${
              active ? 'bg-brand-50 font-medium text-brand-700' : 'text-gray-700 hover:bg-gray-50'
            }`}
          >
            {active && activeBar}
            <span className={active ? 'text-brand-600' : 'text-gray-500'}>{item.icon}</span>
            <span className="flex-1 truncate text-left">{label}</span>
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
                    <span className="truncate">{t(child.labelKey)}</span>
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
        key={item.labelKey}
        href={item.href || '#'}
        aria-current={active ? 'page' : undefined}
        className={`relative flex h-11 items-center gap-2.5 border-b border-gray-100 px-4 text-sm ${FOCUS_RING} ${
          active ? 'bg-brand-50 font-medium text-brand-700' : 'text-gray-700 hover:bg-gray-50'
        }`}
      >
        {active && activeBar}
        <span className={active ? 'text-brand-600' : 'text-gray-500'}>{item.icon}</span>
        <span className="truncate">{label}</span>
        {badge && (
          <span
            title={item.badgeTitleKey ? t(item.badgeTitleKey) : undefined}
            className="ml-auto shrink-0 rounded-md border border-gray-200 bg-gray-100 px-1.5 py-0.5 text-[10px] font-medium uppercase tracking-wide text-gray-600"
          >
            {badge}
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
      aria-label={t('sidebar.ariaLabel')}
    >
      {/* Brand block */}
      <div
        className={`flex h-14 shrink-0 items-center border-b border-gray-200 ${
          collapsed ? 'justify-center px-2' : 'px-4'
        }`}
      >
        <Link
          href="/dashboard"
          aria-label={t('sidebar.brandHome', { product: brand.productName })}
          className={`flex min-w-0 items-center gap-2.5 rounded-md ${FOCUS_RING}`}
        >
          {/* Logo from /public/logo.svg; collapsed shows only the mark on its left. */}
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

      {/* Navigation - scrolls on its own */}
      <nav className="min-h-0 flex-1 overflow-y-auto">
        {menuGroups.map((group) => (
          <div key={group.labelKey}>
            {collapsed ? (
              <div className="border-b-2 border-gray-200" aria-hidden />
            ) : (
              <p className="border-b border-gray-100 bg-gray-50 px-4 py-1.5 text-[11px] font-semibold uppercase tracking-wider text-gray-500">
                {t(group.labelKey)}
              </p>
            )}
            {group.items.map((item) => renderItem(item))}
          </div>
        ))}
      </nav>

      {/* Sidebar footer: version and the collapse button */}
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
          title={collapsed ? t('sidebar.expand') : t('sidebar.collapse')}
          aria-label={collapsed ? t('sidebar.expand') : t('sidebar.collapse')}
          className={`rounded-md p-1.5 text-gray-500 hover:bg-gray-100 hover:text-gray-900 ${FOCUS_RING}`}
        >
          {collapsed ? <PanelLeftOpen size={16} /> : <PanelLeftClose size={16} />}
        </button>
      </div>
    </aside>
  );
}
