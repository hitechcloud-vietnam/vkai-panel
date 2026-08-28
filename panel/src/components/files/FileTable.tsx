'use client';

/**
 * The right pane: one directory, or one set of search results.
 *
 * The columns are exactly what GET /files/list sends - name, size, mode, owner,
 * mod_time - and nothing more. A group column would be blank on every row,
 * because the API reads only the owner (`stat -c %U`), so there is no group
 * column. Search results come from `find` and carry no owner at all, which is
 * why that column shows an em dash rather than a guess when `searchMode` is on.
 */

import {
  ArrowDown,
  ArrowUp,
  Download,
  FileText,
  Folder,
  Pencil,
  ShieldCheck,
  Tag,
  Trash2,
} from 'lucide-react';

import type { FileEntry, FileSortKey, SortDirection } from '@/types/files';
import { formatBytes, formatModTime, octalFromMode, permissionChars, UNKNOWN } from './format';

export interface FileTableProps {
  entries: FileEntry[];
  loading: boolean;
  /** True while the table is showing search results rather than a directory. */
  searchMode: boolean;
  sortKey: FileSortKey;
  sortDirection: SortDirection;
  onSort: (key: FileSortKey) => void;
  selected: string[];
  onToggleSelected: (path: string) => void;
  onToggleAll: () => void;
  onOpenDirectory: (path: string) => void;
  onEdit: (entry: FileEntry) => void;
  onDownload: (entry: FileEntry) => void;
  onRename: (entry: FileEntry) => void;
  onPermissions: (entry: FileEntry) => void;
  onDelete: (entry: FileEntry) => void;
  /** Path currently being downloaded, so its button can say so. */
  busyPath: string | null;
}

const HEADER_CELL =
  'px-4 py-3 text-left text-xs font-semibold uppercase tracking-wide text-gray-500';

const COLUMNS: { key: FileSortKey; label: string; className?: string }[] = [
  { key: 'name', label: 'Name' },
  { key: 'size', label: 'Size' },
  { key: 'mode', label: 'Permissions' },
  { key: 'owner', label: 'Owner' },
  { key: 'mod_time', label: 'Modified' },
];

function SortHeader({
  column,
  sortKey,
  sortDirection,
  onSort,
}: {
  column: { key: FileSortKey; label: string };
  sortKey: FileSortKey;
  sortDirection: SortDirection;
  onSort: (key: FileSortKey) => void;
}) {
  const active = sortKey === column.key;
  return (
    <th scope="col" className={HEADER_CELL} aria-sort={active ? (sortDirection === 'asc' ? 'ascending' : 'descending') : 'none'}>
      <button
        type="button"
        onClick={() => onSort(column.key)}
        className="inline-flex items-center gap-1 uppercase tracking-wide text-gray-500 hover:text-gray-900 focus:outline-none focus:ring-2 focus:ring-brand-500"
      >
        {column.label}
        {active ? (
          sortDirection === 'asc' ? (
            <ArrowUp size={12} aria-hidden="true" />
          ) : (
            <ArrowDown size={12} aria-hidden="true" />
          )
        ) : null}
      </button>
    </th>
  );
}

function RowAction({
  label,
  onClick,
  disabled,
  danger,
  children,
}: {
  label: string;
  onClick: () => void;
  disabled?: boolean;
  danger?: boolean;
  children: React.ReactNode;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      disabled={disabled}
      title={label}
      aria-label={label}
      className={`rounded-md border border-gray-300 bg-white p-1.5 focus:outline-none focus:ring-2 focus:ring-brand-500 disabled:opacity-50 ${
        danger ? 'text-red-700 hover:bg-red-50' : 'text-gray-600 hover:bg-gray-50 hover:text-gray-900'
      }`}
    >
      {children}
    </button>
  );
}

export default function FileTable({
  entries,
  loading,
  searchMode,
  sortKey,
  sortDirection,
  onSort,
  selected,
  onToggleSelected,
  onToggleAll,
  onOpenDirectory,
  onEdit,
  onDownload,
  onRename,
  onPermissions,
  onDelete,
  busyPath,
}: FileTableProps) {
  const selectedSet = new Set(selected);
  const allSelected = entries.length > 0 && entries.every((entry) => selectedSet.has(entry.path));

  return (
    <div className="overflow-x-auto">
      <table className="w-full min-w-[52rem]">
        <thead className="bg-gray-50">
          <tr className="[&_th]:border-b [&_th]:border-gray-200">
            <th scope="col" className="w-10 px-4 py-3">
              <input
                type="checkbox"
                checked={allSelected}
                onChange={onToggleAll}
                disabled={entries.length === 0}
                aria-label={allSelected ? 'Clear selection' : 'Select every row'}
                className="h-4 w-4 rounded border-gray-300 text-brand-600 focus:ring-brand-500"
              />
            </th>
            {COLUMNS.map((column) => (
              <SortHeader
                key={column.key}
                column={column}
                sortKey={sortKey}
                sortDirection={sortDirection}
                onSort={onSort}
              />
            ))}
            <th scope="col" className={`${HEADER_CELL} text-right`}>
              Actions
            </th>
          </tr>
        </thead>
        <tbody className="[&_td]:border-b [&_td]:border-gray-100">
          {loading ? (
            <tr>
              <td colSpan={7} className="px-4 py-12 text-center text-sm text-gray-600">
                Reading directory...
              </td>
            </tr>
          ) : entries.length === 0 ? (
            <tr>
              <td colSpan={7} className="px-4 py-12 text-center text-sm text-gray-600">
                {searchMode ? 'Nothing matched that pattern.' : 'This directory is empty.'}
              </td>
            </tr>
          ) : (
            entries.map((entry) => {
              const octal = octalFromMode(entry.mode);
              const symbolic = permissionChars(entry.mode);
              const isSelected = selectedSet.has(entry.path);
              return (
                <tr key={entry.path} className={isSelected ? 'bg-brand-50' : 'hover:bg-gray-50'}>
                  <td className="px-4 py-3">
                    <input
                      type="checkbox"
                      checked={isSelected}
                      onChange={() => onToggleSelected(entry.path)}
                      aria-label={`Select ${entry.name}`}
                      className="h-4 w-4 rounded border-gray-300 text-brand-600 focus:ring-brand-500"
                    />
                  </td>
                  <td className="px-4 py-3">
                    <div className="flex min-w-0 items-center gap-2">
                      {entry.is_dir ? (
                        <Folder size={16} className="shrink-0 text-gray-400" aria-hidden="true" />
                      ) : (
                        <FileText size={16} className="shrink-0 text-gray-400" aria-hidden="true" />
                      )}
                      {entry.is_dir ? (
                        <button
                          type="button"
                          onClick={() => onOpenDirectory(entry.path)}
                          className="truncate text-sm font-medium text-brand-700 hover:underline focus:outline-none focus:ring-2 focus:ring-brand-500"
                          title={entry.path}
                        >
                          {entry.name}
                        </button>
                      ) : (
                        <button
                          type="button"
                          onClick={() => onEdit(entry)}
                          className="truncate text-sm text-gray-900 hover:text-brand-700 hover:underline focus:outline-none focus:ring-2 focus:ring-brand-500"
                          title={entry.path}
                        >
                          {entry.name}
                        </button>
                      )}
                    </div>
                    {searchMode ? (
                      <p className="mt-0.5 truncate text-xs text-gray-500" title={entry.path}>
                        {entry.path}
                      </p>
                    ) : null}
                  </td>
                  <td className="px-4 py-3 text-sm text-gray-600">
                    {entry.is_dir ? UNKNOWN : formatBytes(entry.size)}
                  </td>
                  <td className="px-4 py-3 text-sm text-gray-600">
                    {symbolic ? (
                      <span className="font-mono text-xs">
                        {symbolic}
                        <span className="ml-1.5 text-gray-500">{octal}</span>
                      </span>
                    ) : (
                      UNKNOWN
                    )}
                  </td>
                  <td className="px-4 py-3 text-sm text-gray-600">{entry.owner || UNKNOWN}</td>
                  <td className="px-4 py-3 text-sm text-gray-600">{formatModTime(entry.mod_time)}</td>
                  <td className="px-4 py-3">
                    <div className="flex items-center justify-end gap-1.5">
                      {entry.is_dir ? null : (
                        <RowAction label={`Edit ${entry.name}`} onClick={() => onEdit(entry)}>
                          <Pencil size={14} aria-hidden="true" />
                        </RowAction>
                      )}
                      {entry.is_dir ? null : (
                        <RowAction
                          label={
                            busyPath === entry.path
                              ? `Downloading ${entry.name}`
                              : `Download ${entry.name}`
                          }
                          onClick={() => onDownload(entry)}
                          disabled={busyPath === entry.path}
                        >
                          <Download size={14} aria-hidden="true" />
                        </RowAction>
                      )}
                      <RowAction label={`Rename ${entry.name}`} onClick={() => onRename(entry)}>
                        <Tag size={14} aria-hidden="true" />
                      </RowAction>
                      <RowAction
                        label={`Change permissions on ${entry.name}`}
                        onClick={() => onPermissions(entry)}
                      >
                        <ShieldCheck size={14} aria-hidden="true" />
                      </RowAction>
                      <RowAction label={`Delete ${entry.name}`} onClick={() => onDelete(entry)} danger>
                        <Trash2 size={14} aria-hidden="true" />
                      </RowAction>
                    </div>
                  </td>
                </tr>
              );
            })
          )}
        </tbody>
      </table>
    </div>
  );
}
