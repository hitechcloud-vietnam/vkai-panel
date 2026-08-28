'use client';

/**
 * "Which machine does this happen on?"
 *
 * On a panel that drives one machine there is no question to ask, so no picker
 * appears: the field states the node the action will run on and moves out of
 * the way. The picker appears exactly when the operator has a real choice -
 * more than one node - which is the moment it starts being useful.
 */

import { AlertTriangle } from 'lucide-react';

import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import LocalNodeBadge from './LocalNodeBadge';
import { isLocalNode, serverLabel } from '@/lib/servers';
import type { ManagedServer } from '@/types/server';

export interface ServerScopeCopy {
  /** Field label, for example "Server". */
  label: string;
  /** Line under a single node, explaining why there is nothing to choose. */
  singleNodeHint: string;
  /** Placeholder in the picker when nothing is selected yet. */
  placeholder: string;
  /** Badge text on the panel host. */
  localBadge: string;
  /** Hover text on that badge. */
  localBadgeTitle: string;
  /** Shown when the API reports no node at all. */
  noNode: string;
}

/** Vietnamese wording, for the screens written in Vietnamese. */
export const SERVER_SCOPE_COPY_VI: ServerScopeCopy = {
  label: 'Máy chủ',
  singleNodeHint:
    'Panel đang quản lý duy nhất máy này, nên thao tác sẽ chạy ngay trên đó.',
  placeholder: 'Chọn máy chủ',
  localBadge: 'Máy cài panel',
  localBadgeTitle: 'Đây là máy đang chạy panel. Panel quản lý chính máy này.',
  noNode: 'Chưa có máy chủ nào được đăng ký, nên chưa thể thực hiện thao tác này.',
};

/** English wording, for the screens written in English. */
export const SERVER_SCOPE_COPY_EN: ServerScopeCopy = {
  label: 'Server',
  singleNodeHint: 'This panel manages one machine, so the action runs there.',
  placeholder: 'Select a server',
  localBadge: 'Panel host',
  localBadgeTitle: 'The machine this panel runs on. The panel manages it directly.',
  noNode: 'No server is registered yet, so this action has nowhere to run.',
};

export interface ServerScopeFieldProps {
  id: string;
  servers: ManagedServer[];
  value: string;
  onChange: (serverId: string) => void;
  copy: ServerScopeCopy;
  /** Set when the caller renders its own label above the field. */
  hideLabel?: boolean;
  className?: string;
}

export default function ServerScopeField({
  id,
  servers,
  value,
  onChange,
  copy,
  hideLabel = false,
  className,
}: ServerScopeFieldProps) {
  const list = Array.isArray(servers) ? servers : [];

  if (list.length === 0) {
    return (
      <div className={className}>
        {!hideLabel && (
          <span className="mb-1.5 block text-sm font-medium text-gray-700">{copy.label}</span>
        )}
        <p className="flex items-start gap-2 rounded-md border border-amber-200 bg-amber-50 px-3 py-2 text-sm text-amber-700">
          <AlertTriangle size={16} className="mt-0.5 shrink-0" aria-hidden="true" />
          <span>{copy.noNode}</span>
        </p>
      </div>
    );
  }

  // One node: state it, do not ask about it.
  if (list.length === 1) {
    const only = list[0];
    return (
      <div className={className}>
        {!hideLabel && (
          <span className="mb-1.5 block text-sm font-medium text-gray-700">{copy.label}</span>
        )}
        <div className="rounded-md border border-gray-200 bg-gray-50 px-3 py-2">
          <div className="flex flex-wrap items-center gap-2">
            <span className="truncate text-sm font-medium text-gray-900">
              {serverLabel(only)}
            </span>
            {isLocalNode(only) && (
              <LocalNodeBadge label={copy.localBadge} title={copy.localBadgeTitle} />
            )}
          </div>
          <p className="mt-0.5 text-xs text-gray-500">{copy.singleNodeHint}</p>
        </div>
      </div>
    );
  }

  return (
    <div className={className}>
      {!hideLabel && (
        <label htmlFor={id} className="mb-1.5 block text-sm font-medium text-gray-700">
          {copy.label}
        </label>
      )}
      <Select value={value} onValueChange={onChange}>
        <SelectTrigger id={id} aria-label={copy.label}>
          <SelectValue placeholder={copy.placeholder} />
        </SelectTrigger>
        <SelectContent className="border-gray-200 bg-white">
          {list.map((server) => (
            <SelectItem key={server.id} value={server.id}>
              {serverLabel(server)}
              {isLocalNode(server) ? ` (${copy.localBadge})` : ''}
              {server.ip_address ? ` — ${server.ip_address}` : ''}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  );
}
