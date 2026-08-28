'use client';

/**
 * "You already have one machine. Here is how to add another, if you ever need
 * one."
 *
 * The panel manages the machine it is installed on, so an operator is never
 * blocked waiting for a second server. Extra machines, clustering and high
 * availability are a layer added on top when a fleet grows - not a step on the
 * way to a working install. This block says that in the two places an operator
 * would look for it, so the cluster menu item stops reading as an unexplained
 * requirement.
 */

import { useState } from 'react';
import Link from 'next/link';
import { ChevronDown, ChevronRight, Network } from 'lucide-react';

import { cn } from '@/lib/utils';

export interface AddNodeCopy {
  title: string;
  /** One line: this panel already manages its own host. */
  lead: string;
  /** Label on the disclosure control. */
  toggleShow: string;
  toggleHide: string;
  /** Ordered steps for enrolling another machine. */
  steps: string[];
  /** Shell shown with the steps. */
  command: string;
  /** The optional-layer sentence about clustering and HA. */
  optionalNote: string;
  clustersLink: string;
  docsLink: string;
}

export const ADD_NODE_COPY_VI: AddNodeCopy = {
  title: 'Thêm máy chủ khác (tùy chọn)',
  lead:
    'Panel đang quản lý chính máy mà nó được cài. Bạn có thể tạo website, cơ sở dữ liệu và chứng chỉ ngay trên máy này mà không cần thêm máy nào khác.',
  toggleShow: 'Xem cách thêm máy chủ',
  toggleHide: 'Ẩn hướng dẫn',
  steps: [
    'Tạo mã ghi danh dùng một lần trong màn hình Thêm máy chủ.',
    'Cài agent trên máy mới rồi khởi động agent với địa chỉ panel và mã ghi danh đó.',
    'Agent tự sinh khóa, lấy chứng chỉ từ panel và xuất hiện trong danh sách máy chủ.',
  ],
  command:
    'curl -sSL https://install.vkai.vn/agent.sh | bash\n' +
    'VKAI_PANEL_URL=https://panel.example.vn \\\n' +
    'VKAI_AGENT_ENROLMENT_TOKEN=vkai-enrol.v1.... \\\n' +
    '  sudo systemctl start vkai-agent',
  optionalNote:
    'Cụm và HA là lớp tùy chọn dành cho nhiều máy: chỉ thiết lập khi bạn cần chịu tải cao hoặc dự phòng chuyển đổi. Một máy duy nhất vẫn chạy đầy đủ mọi tính năng hosting.',
  clustersLink: 'Tìm hiểu Cụm & HA',
  docsLink: 'Tài liệu',
};

export const ADD_NODE_COPY_EN: AddNodeCopy = {
  title: 'Add another machine (optional)',
  lead:
    'This panel manages the machine it was installed on. You can create websites, databases and certificates there without adding a second machine.',
  toggleShow: 'Show how to add a machine',
  toggleHide: 'Hide instructions',
  steps: [
    'Mint a single-use enrolment token on the Add Server screen.',
    'Install the agent on the new machine and start it with the panel URL and that token.',
    'The agent generates its own key, collects a certificate from the panel, and appears in this list.',
  ],
  command:
    'curl -sSL https://install.vkai.vn/agent.sh | bash\n' +
    'VKAI_PANEL_URL=https://panel.example.vn \\\n' +
    'VKAI_AGENT_ENROLMENT_TOKEN=vkai-enrol.v1.... \\\n' +
    '  sudo systemctl start vkai-agent',
  optionalNote:
    'Clustering and HA are an optional layer for several machines: set them up when you need to spread load or fail over, not before. A single machine runs every hosting feature on its own.',
  clustersLink: 'About Clusters & HA',
  docsLink: 'Documentation',
};

export interface AddNodeCalloutProps {
  copy: AddNodeCopy;
  docsHref?: string;
  className?: string;
}

export default function AddNodeCallout({
  copy,
  docsHref = 'https://hitechcloud.vn/docs',
  className,
}: AddNodeCalloutProps) {
  const [open, setOpen] = useState(false);

  return (
    <section
      className={cn('rounded-lg border border-gray-200 bg-white shadow-sm', className)}
      aria-label={copy.title}
    >
      <div className="flex flex-wrap items-start gap-3 px-5 py-4">
        <div className="mt-0.5 rounded-md border border-gray-200 bg-gray-50 p-2 text-gray-600">
          <Network size={18} aria-hidden="true" />
        </div>
        <div className="min-w-0 flex-1">
          <h2 className="text-sm font-semibold text-gray-900">{copy.title}</h2>
          <p className="mt-1 text-sm text-gray-600">{copy.lead}</p>
          <p className="mt-2 text-sm text-gray-600">{copy.optionalNote}</p>

          <div className="mt-3 flex flex-wrap items-center gap-3">
            <button
              type="button"
              onClick={() => setOpen((prev) => !prev)}
              aria-expanded={open}
              className="inline-flex items-center gap-1.5 rounded-md border border-gray-300 bg-white px-3 py-1.5 text-sm font-medium text-gray-700 hover:bg-gray-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500"
            >
              {open ? (
                <ChevronDown size={14} aria-hidden="true" />
              ) : (
                <ChevronRight size={14} aria-hidden="true" />
              )}
              {open ? copy.toggleHide : copy.toggleShow}
            </button>
            <Link
              href="/clusters"
              className="rounded-md text-sm font-medium text-brand-700 hover:text-brand-800 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500"
            >
              {copy.clustersLink}
            </Link>
            <a
              href={docsHref}
              target="_blank"
              rel="noreferrer"
              className="rounded-md text-sm font-medium text-brand-700 hover:text-brand-800 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500"
            >
              {copy.docsLink}
            </a>
          </div>

          {open && (
            <div className="mt-4 border-t border-gray-200 pt-4">
              <ol className="list-decimal space-y-1.5 pl-5 text-sm text-gray-700">
                {copy.steps.map((step) => (
                  <li key={step}>{step}</li>
                ))}
              </ol>
              <pre className="mt-3 overflow-x-auto rounded-md border border-gray-200 bg-gray-50 p-3 font-mono text-xs text-gray-700">
                {copy.command}
              </pre>
            </div>
          )}
        </div>
      </div>
    </section>
  );
}
