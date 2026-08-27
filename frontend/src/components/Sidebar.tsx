'use client';

import Link from 'next/link';
import { usePathname } from 'next/navigation';
import {
  LayoutDashboard,
  Server,
  Globe,
  Database,
  Shield,
  FileCode,
  Container,
  Clock,
  Terminal,
  Flame,
  HardDrive,
  Activity,
  FileText,
  GitBranch,
  Users,
  Key,
  Settings,
  ChevronDown,
  ChevronRight,
  BarChart3,
  Mail,
} from 'lucide-react';
import { useState } from 'react';

interface MenuItem {
  label: string;
  icon: React.ReactNode;
  href?: string;
  children?: MenuItem[];
}

const menuItems: MenuItem[] = [
  {
    label: 'Dashboard',
    icon: <LayoutDashboard size={18} />,
    href: '/dashboard',
  },
  {
    label: 'Servers',
    icon: <Server size={18} />,
    children: [
      { label: 'All Servers', icon: <></>, href: '/servers' },
      { label: 'Add Server', icon: <></>, href: '/servers/add' },
      { label: 'Server Groups', icon: <></>, href: '/servers/groups' },
      { label: 'Monitoring', icon: <></>, href: '/servers/monitoring' },
    ],
  },
  {
    label: 'Websites',
    icon: <Globe size={18} />,
    children: [
      { label: 'All Websites', icon: <></>, href: '/websites' },
      { label: 'Add Website', icon: <></>, href: '/websites/add' },
      { label: 'PHP Sites', icon: <></>, href: '/websites/php' },
      { label: 'Node.js', icon: <></>, href: '/websites/nodejs' },
      { label: 'Reverse Proxy', icon: <></>, href: '/websites/proxy' },
      { label: 'WordPress', icon: <></>, href: '/websites/wordpress' },
    ],
  },
  {
    label: 'Databases',
    icon: <Database size={18} />,
    children: [
      { label: 'MySQL/MariaDB', icon: <></>, href: '/databases/mysql' },
      { label: 'PostgreSQL', icon: <></>, href: '/databases/postgresql' },
      { label: 'Redis', icon: <></>, href: '/databases/redis' },
    ],
  },
  {
    label: 'DNS',
    icon: <FileCode size={18} />,
    href: '/dns',
  },
  {
    label: 'SSL',
    icon: <Shield size={18} />,
    href: '/ssl',
  },
  {
    label: 'Docker',
    icon: <Container size={18} />,
    href: '/docker',
  },
  {
    label: 'Files',
    icon: <FileText size={18} />,
    href: '/files',
  },
  {
    label: 'Cron',
    icon: <Clock size={18} />,
    href: '/cron',
  },
  {
    label: 'Terminal',
    icon: <Terminal size={18} />,
    href: '/terminal',
  },
  {
    label: 'Firewall',
    icon: <Flame size={18} />,
    href: '/firewall',
  },
  {
    label: 'Security',
    icon: <Shield size={18} />,
    href: '/security',
  },
  {
    label: 'WAF Pro',
    icon: <Shield size={18} />,
    href: '/waf',
  },
  {
    label: 'Website Stats',
    icon: <BarChart3 size={18} />,
    href: '/website-stats',
  },
  {
    label: 'Email Marketing',
    icon: <Mail size={18} />,
    href: '/email-marketing',
  },
  {
    label: 'Mail Server',
    icon: <Server size={18} />,
    href: '/mail-server',
  },
  {
    label: 'File Protection',
    icon: <Shield size={18} />,
    href: '/file-protection',
  },
  {
    label: 'Backups',
    icon: <HardDrive size={18} />,
    href: '/backups',
  },
  {
    label: 'Logs',
    icon: <Activity size={18} />,
    href: '/logs',
  },
  {
    label: 'Monitoring',
    icon: <Activity size={18} />,
    href: '/monitoring',
  },
  {
    label: 'Notifications',
    icon: <Activity size={18} />,
    href: '/notifications',
  },
  {
    label: 'Audit',
    icon: <Activity size={18} />,
    href: '/audit',
  },
  {
    label: 'Clusters & HA',
    icon: <Activity size={18} />,
    href: '/clusters',
  },
  {
    label: 'Job Queue',
    icon: <Activity size={18} />,
    href: '/jobs',
  },
  {
    label: 'Config Rollback',
    icon: <Activity size={18} />,
    href: '/config',
  },
  {
    label: 'Deployments',
    icon: <GitBranch size={18} />,
    href: '/deployments',
  },
  {
    label: 'Users',
    icon: <Users size={18} />,
    href: '/users',
  },
  {
    label: 'API',
    icon: <Key size={18} />,
    href: '/api-keys',
  },
  {
    label: 'Settings',
    icon: <Settings size={18} />,
    href: '/settings',
  },
];

export default function Sidebar() {
  const pathname = usePathname();
  const [expanded, setExpanded] = useState<Record<string, boolean>>({});

  const toggleExpand = (label: string) => {
    setExpanded((prev) => ({ ...prev, [label]: !prev[label] }));
  };

  const renderMenuItem = (item: MenuItem, depth = 0) => {
    const isActive = item.href === pathname;
    const isExpanded = expanded[item.label];
    const hasChildren = item.children && item.children.length > 0;

    if (hasChildren) {
      return (
        <div key={item.label}>
          <button
            onClick={() => toggleExpand(item.label)}
            className="w-full flex items-center gap-3 px-4 py-2.5 text-sm text-dark-300 hover:text-dark-50 hover:bg-dark-800 rounded-lg transition-colors"
          >
            {item.icon}
            <span className="flex-1 text-left">{item.label}</span>
            {isExpanded ? <ChevronDown size={14} /> : <ChevronRight size={14} />}
          </button>
          {isExpanded && item.children && (
            <div className="ml-4 mt-1 space-y-1">
              {item.children.map((child) => renderMenuItem(child, depth + 1))}
            </div>
          )}
        </div>
      );
    }

    return (
      <Link
        key={item.label}
        href={item.href || '#'}
        className={`flex items-center gap-3 px-4 py-2.5 text-sm rounded-lg transition-colors ${
          isActive
            ? 'bg-primary-600/10 text-primary-400 border-l-2 border-primary-500'
            : 'text-dark-300 hover:text-dark-50 hover:bg-dark-800'
        }`}
        style={{ paddingLeft: depth > 0 ? '1rem' : '1rem' }}
      >
        {item.icon}
        <span>{item.label}</span>
      </Link>
    );
  };

  return (
    <aside className="w-64 bg-dark-900 border-r border-dark-700 flex flex-col">
      {/* Logo */}
      <div className="h-16 flex items-center px-6 border-b border-dark-700">
        <div className="flex items-center gap-2">
          <div className="w-8 h-8 bg-primary-600 rounded-lg flex items-center justify-center">
            <span className="text-white font-bold text-sm">vK</span>
          </div>
          <div>
            <h1 className="text-lg font-bold text-dark-50">vKAI</h1>
            <p className="text-[10px] text-dark-400 -mt-1">HiTechCloud</p>
          </div>
        </div>
      </div>

      {/* Navigation */}
      <nav className="flex-1 overflow-y-auto p-4 space-y-1">
        {menuItems.map((item) => renderMenuItem(item))}
      </nav>

      {/* Footer */}
      <div className="p-4 border-t border-dark-700">
        <div className="text-xs text-dark-500">
          vKAI Panel v1.0.0
        </div>
      </div>
    </aside>
  );
}
