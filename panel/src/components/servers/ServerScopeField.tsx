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
import { useT } from '@/i18n';
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

/**
 * Deprecated. Every string now comes from the dictionary; this export remains
 * only so the screens that still pass `copy` keep compiling, and passing it
 * changes nothing. Drop the prop at the call site and this goes with it.
 */
export const SERVER_SCOPE_COPY_EN: Partial<ServerScopeCopy> = {};

/**
 * The value that means "every server".
 *
 * A sentinel rather than the empty string, because Radix's Select refuses an
 * empty item value outright - it reserves that for "nothing selected" and throws
 * at render. It is deliberately not a UUID shape, so a caller that forgets to
 * translate it sends something the API rejects immediately instead of a value
 * that looks plausible.
 */
export const ALL_SERVERS = '*';

export interface ServerScopeFieldProps {
  id: string;
  servers: ManagedServer[];
  value: string;
  onChange: (serverId: string) => void;
  /** Deprecated per-field override; omit it and the dictionary is used. */
  copy?: Partial<ServerScopeCopy>;
  /** Set when the caller renders its own label above the field. */
  hideLabel?: boolean;
  /**
   * Offer an "every server" choice, whose value is the empty string.
   *
   * Off by default: most callers need one server, and a screen that silently
   * accepted "all" where the backend wants one id would fail at submit rather
   * than at selection. Only pass this where the caller has actually implemented
   * what "all" means.
   */
  allowAll?: boolean;
  /** Label for the "every server" choice. Ignored unless allowAll is set. */
  allLabel?: string;
  className?: string;
}

export default function ServerScopeField({
  id,
  servers,
  allowAll = false,
  allLabel,
  value,
  onChange,
  copy,
  hideLabel = false,
  className,
}: ServerScopeFieldProps) {
  const t = useT();
  const list = Array.isArray(servers) ? servers : [];

  const label = copy?.label ?? t('common.field.server');
  const singleNodeHint = copy?.singleNodeHint ?? t('servers.scope.singleNodeHint');
  const placeholder = copy?.placeholder ?? t('servers.scope.placeholder');
  const localBadge = copy?.localBadge ?? t('servers.localBadge');
  const localBadgeTitle = copy?.localBadgeTitle ?? t('servers.scope.localBadgeTitle');
  const noNode = copy?.noNode ?? t('servers.scope.noNode');

  if (list.length === 0) {
    return (
      <div className={className}>
        {!hideLabel && (
          <span className="mb-1.5 block text-sm font-medium text-gray-700">{label}</span>
        )}
        <p className="flex items-start gap-2 rounded-md border border-amber-200 bg-amber-50 px-3 py-2 text-sm text-amber-700">
          <AlertTriangle size={16} className="mt-0.5 shrink-0" aria-hidden="true" />
          <span>{noNode}</span>
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
          <span className="mb-1.5 block text-sm font-medium text-gray-700">{label}</span>
        )}
        <div className="rounded-md border border-gray-200 bg-gray-50 px-3 py-2">
          <div className="flex flex-wrap items-center gap-2">
            <span className="truncate text-sm font-medium text-gray-900">
              {serverLabel(only)}
            </span>
            {isLocalNode(only) && (
              <LocalNodeBadge label={localBadge} title={localBadgeTitle} />
            )}
          </div>
          <p className="mt-0.5 text-xs text-gray-500">{singleNodeHint}</p>
        </div>
      </div>
    );
  }

  return (
    <div className={className}>
      {!hideLabel && (
        <label htmlFor={id} className="mb-1.5 block text-sm font-medium text-gray-700">
          {label}
        </label>
      )}
      <Select value={value} onValueChange={onChange}>
        <SelectTrigger id={id} aria-label={label}>
          <SelectValue placeholder={placeholder} />
        </SelectTrigger>
        <SelectContent className="border-gray-200 bg-white">
          {allowAll && (
            <SelectItem value={ALL_SERVERS}>
              {allLabel ?? 'Every server'}
            </SelectItem>
          )}
          {list.map((server) => (
            <SelectItem key={server.id} value={server.id}>
              {serverLabel(server)}
              {isLocalNode(server) ? ` (${localBadge})` : ''}
              {server.ip_address ? ` — ${server.ip_address}` : ''}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  );
}
