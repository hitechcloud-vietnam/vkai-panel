'use client';

/**
 * The panel host, before the API has confirmed it.
 *
 * A fresh install manages the machine it was installed on, so an empty server
 * list is not "you have no servers" - it is "the panel has not finished
 * registering the machine you are looking at". Showing nothing at that moment
 * teaches the operator the wrong thing about the product. So the list shows the
 * host anyway, named by the address the browser reached it on, with every
 * hardware figure marked unavailable rather than guessed, and a way to look
 * again.
 */

import { useEffect, useState } from 'react';
import { RefreshCw, Server } from 'lucide-react';

import LocalNodeBadge from './LocalNodeBadge';
import { Unavailable } from '@/components/Unavailable';

export interface PanelHostPlaceholderCopy {
  title: string;
  /** Why this row is here and what it means. */
  body: string;
  localBadge: string;
  localBadgeTitle: string;
  statusLabel: string;
  /** Status word, for example "registering". */
  statusValue: string;
  /** Tooltip on every unavailable figure. */
  unavailableReason: string;
  hostnameLabel: string;
  osLabel: string;
  cpuLabel: string;
  ramLabel: string;
  refresh: string;
}

export const PANEL_HOST_PLACEHOLDER_COPY_EN: PanelHostPlaceholderCopy = {
  title: 'Panel host',
  body:
    'This panel manages the machine it runs on. That machine has not been registered as a managed node yet, so its details are not available. Check again in a moment; if it stays this way, the local agent is not reporting.',
  localBadge: 'Panel host',
  localBadgeTitle: 'The machine this panel runs on.',
  statusLabel: 'Status',
  statusValue: 'registering',
  unavailableReason:
    'Not available: the panel host has not been registered as a managed node yet, so nothing has reported this figure.',
  hostnameLabel: 'Address',
  osLabel: 'OS',
  cpuLabel: 'CPU',
  ramLabel: 'RAM',
  refresh: 'Check again',
};

export const PANEL_HOST_PLACEHOLDER_COPY_VI: PanelHostPlaceholderCopy = {
  title: 'Máy cài panel',
  body:
    'Panel quản lý chính máy mà nó đang chạy. Máy này chưa được đăng ký thành node quản lý nên chưa có thông số. Hãy thử lại sau giây lát; nếu vẫn vậy thì agent nội bộ chưa gửi dữ liệu.',
  localBadge: 'Máy cài panel',
  localBadgeTitle: 'Máy đang chạy panel.',
  statusLabel: 'Trạng thái',
  statusValue: 'đang đăng ký',
  unavailableReason:
    'Chưa có dữ liệu: máy cài panel chưa được đăng ký thành node quản lý nên chưa có nguồn nào báo cáo số liệu này.',
  hostnameLabel: 'Địa chỉ',
  osLabel: 'Hệ điều hành',
  cpuLabel: 'CPU',
  ramLabel: 'RAM',
  refresh: 'Kiểm tra lại',
};

export interface PanelHostPlaceholderProps {
  copy: PanelHostPlaceholderCopy;
  onRefresh?: () => void;
}

export default function PanelHostPlaceholder({
  copy,
  onRefresh,
}: PanelHostPlaceholderProps) {
  // The address the operator reached the panel on is the one thing the browser
  // genuinely knows about this machine. Read after mount so server and client
  // render the same markup.
  const [host, setHost] = useState('');
  useEffect(() => {
    try {
      setHost(window.location.hostname || '');
    } catch {
      setHost('');
    }
  }, []);

  const rows: { label: string; value: string | null }[] = [
    { label: copy.hostnameLabel, value: host || null },
    { label: copy.osLabel, value: null },
    { label: copy.cpuLabel, value: null },
    { label: copy.ramLabel, value: null },
  ];

  return (
    <div className="rounded-lg border border-gray-200 bg-white p-5 shadow-sm">
      <div className="flex items-start justify-between gap-3">
        <div className="flex min-w-0 items-center gap-3">
          <div className="rounded-md border border-gray-200 bg-gray-50 p-2">
            <Server className="text-gray-600" size={18} aria-hidden="true" />
          </div>
          <div className="min-w-0">
            <div className="flex flex-wrap items-center gap-2">
              <h3 className="truncate text-sm font-semibold text-gray-900">{copy.title}</h3>
              <LocalNodeBadge label={copy.localBadge} title={copy.localBadgeTitle} />
            </div>
            <p className="text-xs text-gray-500" suppressHydrationWarning>
              {host || ' '}
            </p>
          </div>
        </div>
        <span className="inline-flex shrink-0 items-center rounded-md bg-amber-50 px-2 py-0.5 text-xs font-medium text-amber-700">
          {copy.statusValue}
        </span>
      </div>

      <p className="mt-4 text-sm text-gray-600">{copy.body}</p>

      <dl className="mt-4 grid grid-cols-2 gap-4">
        {rows.map((row) => (
          <div key={row.label}>
            <dt className="text-xs text-gray-500">{row.label}</dt>
            <dd className="text-sm text-gray-900" suppressHydrationWarning>
              {row.value ? (
                <span className="font-mono">{row.value}</span>
              ) : (
                <Unavailable reason={copy.unavailableReason} />
              )}
            </dd>
          </div>
        ))}
      </dl>

      {onRefresh && (
        <button
          type="button"
          onClick={onRefresh}
          className="mt-4 inline-flex items-center gap-2 rounded-md border border-gray-300 bg-white px-3 py-1.5 text-sm font-medium text-gray-700 hover:bg-gray-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500"
        >
          <RefreshCw size={14} aria-hidden="true" />
          {copy.refresh}
        </button>
      )}
    </div>
  );
}
