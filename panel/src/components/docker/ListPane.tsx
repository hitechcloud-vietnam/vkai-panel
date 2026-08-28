'use client';

/**
 * The list panes - Container, Local image, Compose, Network, Volume - differ
 * only in their columns and their cells. Everything around them is identical:
 * search, refresh, skeleton, error, empty, and the stub notice that replaces
 * "empty" while the handler behind the route is a literal.
 *
 * Writing that five times is how five slightly different empty states appear,
 * so it is written once here and each pane supplies its columns.
 */

import { useMemo, useState } from 'react';

import { CapabilityGaps, StubResultNotice } from './CapabilityNotice';
import {
  EmptyState,
  ErrorBlock,
  Panel,
  PaneToolbar,
  SectionHeader,
  TableScroller,
  TableSkeleton,
  TH_CLASS,
} from './PaneChrome';
import type { AsyncResource } from './useDockerData';
import type { CapabilityGap } from '@/types/docker';

export interface ListColumn {
  key: string;
  label: string;
  className?: string;
}

export interface ListPaneProps<T> {
  title: string;
  description: string;
  searchPlaceholder: string;
  columns: ListColumn[];
  resource: AsyncResource<T[]>;
  rowKey: (row: T) => string;
  filter: (row: T, term: string) => boolean;
  renderRow: (row: T) => React.ReactNode;
  errorTitle: string;
  emptyTitle: string;
  emptyDescription: string;
  /**
   * Set when the route's handler is known to return a hardcoded empty slice.
   * An empty result then reads as "the panel is not looking", not "there is
   * nothing there" - opposite facts that must never share a screen.
   */
  stub?: { resource: string; handler: string; detail: string };
  gapsTitle: string;
  gapsIntro: string;
  gaps: CapabilityGap[];
  /** Extra controls to the right of Refresh. */
  toolbarExtra?: React.ReactNode;
  /** Anything that belongs above the table, such as a prune summary. */
  children?: React.ReactNode;
}

export function ListPane<T>({
  title,
  description,
  searchPlaceholder,
  columns,
  resource,
  rowKey,
  filter,
  renderRow,
  errorTitle,
  emptyTitle,
  emptyDescription,
  stub,
  gapsTitle,
  gapsIntro,
  gaps,
  toolbarExtra,
  children,
}: ListPaneProps<T>) {
  const [search, setSearch] = useState('');
  const { data, loading, refreshing, error, reload } = resource;

  const visible = useMemo(() => {
    const term = search.trim();
    if (!term) return data;
    return data.filter((row) => filter(row, term));
  }, [data, search, filter]);

  return (
    <div className="space-y-4">
      {children}

      <Panel>
        <SectionHeader title={title} description={description} />

        <div className="border-b border-gray-200 px-4 py-3">
          <PaneToolbar
            search={search}
            onSearchChange={setSearch}
            searchPlaceholder={searchPlaceholder}
            onRefresh={reload}
            refreshing={loading || refreshing}
          >
            {toolbarExtra}
          </PaneToolbar>
        </div>

        {loading ? (
          <TableSkeleton columns={columns.length} />
        ) : error ? (
          <ErrorBlock title={errorTitle} message={error} onRetry={reload} />
        ) : data.length === 0 && stub ? (
          <StubResultNotice
            resource={stub.resource}
            handler={stub.handler}
            detail={stub.detail}
          />
        ) : data.length === 0 ? (
          <EmptyState title={emptyTitle} description={emptyDescription} />
        ) : visible.length === 0 ? (
          <EmptyState
            title="Nothing matches that search"
            description={`No row here contains "${search.trim()}". Clear the box to see all ${data.length} again.`}
          />
        ) : (
          <TableScroller>
            <table className="w-full min-w-[720px] border-collapse">
              <thead className="bg-gray-50">
                <tr className="border-b border-gray-200">
                  {columns.map((column) => (
                    <th key={column.key} scope="col" className={`${TH_CLASS} ${column.className || ''}`}>
                      {column.label}
                    </th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {visible.map((row) => (
                  <tr key={rowKey(row)} className="border-b border-gray-100 last:border-b-0 hover:bg-gray-50">
                    {renderRow(row)}
                  </tr>
                ))}
              </tbody>
            </table>
          </TableScroller>
        )}
      </Panel>

      <CapabilityGaps title={gapsTitle} intro={gapsIntro} gaps={gaps} />
    </div>
  );
}

export default ListPane;
