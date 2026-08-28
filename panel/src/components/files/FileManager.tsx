'use client';

/**
 * The file manager screen.
 *
 * Two panes, the shape every panel uses: the directory tree on the left, one
 * directory's contents on the right. Every control on this page maps to a route
 * that is mounted in core/internal/handler/router.go under /api/v1/files. Where
 * the API has no route - changing an owner, compressing a directory, a batch
 * delete - there is no button, and the places where an operator would reach for
 * one say why instead.
 *
 * The current directory lives in the URL as ?path=, so a directory can be
 * bookmarked and a reload comes back to it rather than to the root.
 *
 * On safety: the API is the boundary. service.ResolvePath re-anchors every path
 * under the jail root, refuses symlinks that leave it and refuses the sensitive
 * system directories outright, and nothing on this page can loosen that. What
 * the page does instead is make the root visible at all times, and pass the
 * server's own refusal through to the operator rather than replacing it with
 * "something went wrong".
 */

import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useRouter, useSearchParams } from 'next/navigation';
import {
  ArrowUpFromLine,
  ChevronRight,
  CornerLeftUp,
  Copy as CopyIcon,
  Download,
  FilePlus2,
  FolderPlus,
  HardDrive,
  RefreshCw,
  Search,
  ShieldCheck,
  Trash2,
  Move,
  X,
} from 'lucide-react';

import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import type { DiskUsage, FileEntry, FileSortKey, SortDirection, UploadTask } from '@/types/files';

import DeleteDialog from './DeleteDialog';
import DirectoryTree from './DirectoryTree';
import FileEditor, { type EditorTarget } from './FileEditor';
import FileTable from './FileTable';
import Modal from './Modal';
import NameDialog from './NameDialog';
import PermissionsDialog from './PermissionsDialog';
import TransferDialog from './TransferDialog';
import UploadPanel from './UploadPanel';
import {
  blockedUploadReason,
  fileErrorMessage,
  filesApi,
  MAX_UPLOAD_BYTES,
} from './api';
import { formatBytes, validateSearchPattern } from './format';
import { crumbsWithin, dirName, joinPath, normalizePath, parentWithin } from './paths';

const ROOT_LABEL = 'Root';

type DialogKind =
  | { kind: 'none' }
  | { kind: 'new-folder' }
  | { kind: 'new-file' }
  | { kind: 'rename'; entry: FileEntry }
  | { kind: 'copy'; items: FileEntry[] }
  | { kind: 'move'; items: FileEntry[] }
  | { kind: 'chmod'; items: FileEntry[] }
  | { kind: 'delete'; items: FileEntry[] }
  | { kind: 'disk-usage' };

/** One request per item, every failure reported against the name it belongs to. */
async function runPerItem(
  items: FileEntry[],
  run: (entry: FileEntry) => Promise<void>,
  fallback: string
): Promise<string[]> {
  const failures: string[] = [];
  for (const entry of items) {
    try {
      await run(entry);
    } catch (err) {
      failures.push(`${entry.name}: ${fileErrorMessage(err, fallback)}`);
    }
  }
  return failures;
}

export default function FileManager() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const pathParam = searchParams.get('path') || '';

  const [rootPath, setRootPath] = useState('');
  const [rootError, setRootError] = useState<string | null>(null);
  const [rootResolved, setRootResolved] = useState(false);

  const currentPath = normalizePath(pathParam || rootPath || '/');

  const [entries, setEntries] = useState<FileEntry[]>([]);
  const [listLoading, setListLoading] = useState(true);
  const [listError, setListError] = useState<string | null>(null);
  const [refreshToken, setRefreshToken] = useState(0);

  const [sortKey, setSortKey] = useState<FileSortKey>('name');
  const [sortDirection, setSortDirection] = useState<SortDirection>('asc');
  const [selected, setSelected] = useState<string[]>([]);

  const [patternInput, setPatternInput] = useState('');
  const [searchResults, setSearchResults] = useState<FileEntry[] | null>(null);
  const [searchLabel, setSearchLabel] = useState('');
  const [searchBusy, setSearchBusy] = useState(false);

  const [dialog, setDialog] = useState<DialogKind>({ kind: 'none' });
  const [editorTarget, setEditorTarget] = useState<EditorTarget | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);
  const [busyPath, setBusyPath] = useState<string | null>(null);

  const [uploads, setUploads] = useState<UploadTask[]>([]);
  const uploadControllers = useRef<Map<string, AbortController>>(new Map());
  const fileInputRef = useRef<HTMLInputElement | null>(null);

  const [diskUsage, setDiskUsage] = useState<DiskUsage | null>(null);
  const [diskUsageError, setDiskUsageError] = useState<string | null>(null);
  const [diskUsageBusy, setDiskUsageBusy] = useState(false);

  /* ---------------------------------------------------------------------- */
  /* Where the root is                                                       */
  /* ---------------------------------------------------------------------- */

  // The API never states its jail root, but every listing row carries an
  // absolute path, so the root is the parent of anything listed at "/". That is
  // worth one request at start-up: without it the breadcrumb and the tree would
  // have to pretend the root is "/", which is exactly the impression this
  // screen must not give.
  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        const rows = await filesApi.list('/');
        if (cancelled) return;
        const first = rows[0];
        setRootPath(first ? dirName(first.path) : '/');
      } catch (err) {
        if (cancelled) return;
        setRootPath('/');
        setRootError(
          fileErrorMessage(err, 'The file manager root could not be read, so paths below are shown as the server reports them.')
        );
      } finally {
        if (!cancelled) setRootResolved(true);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, []);

  /* ---------------------------------------------------------------------- */
  /* The listing                                                             */
  /* ---------------------------------------------------------------------- */

  useEffect(() => {
    let cancelled = false;
    setListLoading(true);
    setListError(null);
    filesApi
      .list(currentPath)
      .then((rows) => {
        if (cancelled) return;
        setEntries(rows);
        setSelected([]);
      })
      .catch((err) => {
        if (cancelled) return;
        setEntries([]);
        setSelected([]);
        setListError(fileErrorMessage(err, 'This directory could not be read.'));
      })
      .finally(() => {
        if (!cancelled) setListLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [currentPath, refreshToken]);

  const navigate = useCallback(
    (path: string) => {
      setSearchResults(null);
      setSearchLabel('');
      setNotice(null);
      setActionError(null);
      router.push(`/files?path=${encodeURIComponent(normalizePath(path))}`);
    },
    [router]
  );

  const refresh = useCallback(() => setRefreshToken((token) => token + 1), []);

  /* ---------------------------------------------------------------------- */
  /* Sorting and selection                                                   */
  /* ---------------------------------------------------------------------- */

  const rows = searchResults ?? entries;

  const visible = useMemo(() => {
    const copy = [...rows];
    const factor = sortDirection === 'asc' ? 1 : -1;
    copy.sort((a, b) => {
      // Directories stay above files whichever column is sorted; that is what
      // every file manager does and what an operator scans for first.
      if (a.is_dir !== b.is_dir) return a.is_dir ? -1 : 1;
      switch (sortKey) {
        case 'size':
          return factor * ((a.size || 0) - (b.size || 0));
        case 'mode':
          return factor * String(a.mode || '').localeCompare(String(b.mode || ''));
        case 'owner':
          return factor * String(a.owner || '').localeCompare(String(b.owner || ''));
        case 'mod_time':
          // The API's timestamp format sorts correctly as text.
          return factor * String(a.mod_time || '').localeCompare(String(b.mod_time || ''));
        case 'name':
        default:
          return factor * a.name.localeCompare(b.name);
      }
    });
    return copy;
  }, [rows, sortKey, sortDirection]);

  const selectedEntries = useMemo(
    () => visible.filter((entry) => selected.includes(entry.path)),
    [visible, selected]
  );

  // Written without a nested setState on purpose: React re-runs an updater
  // under StrictMode, and a toggle inside one fires twice and cancels itself.
  const onSort = useCallback(
    (key: FileSortKey) => {
      if (key === sortKey) {
        setSortDirection((dir) => (dir === 'asc' ? 'desc' : 'asc'));
        return;
      }
      setSortKey(key);
      setSortDirection('asc');
    },
    [sortKey]
  );

  const toggleSelected = useCallback((path: string) => {
    setSelected((prev) =>
      prev.includes(path) ? prev.filter((item) => item !== path) : [...prev, path]
    );
  }, []);

  const toggleAll = useCallback(() => {
    setSelected((prev) => (prev.length === visible.length ? [] : visible.map((e) => e.path)));
  }, [visible]);

  /* ---------------------------------------------------------------------- */
  /* Search                                                                  */
  /* ---------------------------------------------------------------------- */

  const runSearch = useCallback(async () => {
    const invalid = validateSearchPattern(patternInput);
    if (invalid) {
      setActionError(invalid);
      return;
    }
    setSearchBusy(true);
    setActionError(null);
    setNotice(null);
    try {
      const found = await filesApi.search(currentPath, patternInput.trim());
      setSearchResults(found);
      setSelected([]);
      setSearchLabel(`${found.length} match${found.length === 1 ? '' : 'es'} for "${patternInput.trim()}"`);
    } catch (err) {
      setSearchResults(null);
      setActionError(fileErrorMessage(err, 'The search did not run.'));
    } finally {
      setSearchBusy(false);
    }
  }, [patternInput, currentPath]);

  const clearSearch = useCallback(() => {
    setSearchResults(null);
    setSearchLabel('');
    setPatternInput('');
    setSelected([]);
  }, []);

  /* ---------------------------------------------------------------------- */
  /* Download                                                                */
  /* ---------------------------------------------------------------------- */

  const downloadOne = useCallback(async (entry: FileEntry) => {
    setBusyPath(entry.path);
    setActionError(null);
    try {
      const blob = await filesApi.download(entry.path);
      const url = window.URL.createObjectURL(blob);
      const anchor = document.createElement('a');
      anchor.href = url;
      anchor.download = entry.name;
      document.body.appendChild(anchor);
      anchor.click();
      anchor.remove();
      // Revoking in the same tick can cancel the download in some browsers.
      window.setTimeout(() => window.URL.revokeObjectURL(url), 1000);
    } catch (err) {
      setActionError(`${entry.name}: ${fileErrorMessage(err, 'This file could not be downloaded.')}`);
    } finally {
      setBusyPath(null);
    }
  }, []);

  const downloadSelected = useCallback(async () => {
    const files = selectedEntries.filter((entry) => !entry.is_dir);
    if (files.length === 0) {
      setActionError('Downloading a directory is not something this API offers. Select files instead.');
      return;
    }
    for (const file of files) {
      // eslint-disable-next-line no-await-in-loop
      await downloadOne(file);
    }
  }, [selectedEntries, downloadOne]);

  /* ---------------------------------------------------------------------- */
  /* Upload                                                                  */
  /* ---------------------------------------------------------------------- */

  const startUploads = useCallback(
    (files: FileList) => {
      setActionError(null);
      const directory = currentPath;
      Array.from(files).forEach((file) => {
        const id = `${Date.now()}-${file.name}-${Math.random().toString(36).slice(2, 8)}`;
        const destination = joinPath(directory, file.name);

        const blocked = blockedUploadReason(file.name);
        if (blocked) {
          setUploads((prev) => [
            ...prev,
            { id, name: file.name, destination, size: file.size, progress: null, status: 'error', error: blocked },
          ]);
          return;
        }
        if (file.size > MAX_UPLOAD_BYTES) {
          setUploads((prev) => [
            ...prev,
            {
              id,
              name: file.name,
              destination,
              size: file.size,
              progress: null,
              status: 'error',
              error: `This file is ${formatBytes(file.size)}. The server refuses anything over ${formatBytes(MAX_UPLOAD_BYTES)}.`,
            },
          ]);
          return;
        }

        const controller = new AbortController();
        uploadControllers.current.set(id, controller);
        setUploads((prev) => [
          ...prev,
          { id, name: file.name, destination, size: file.size, progress: 0, status: 'uploading' },
        ]);

        filesApi
          .upload(destination, file, {
            signal: controller.signal,
            onProgress: (percent) => {
              setUploads((prev) =>
                prev.map((task) => (task.id === id ? { ...task, progress: percent } : task))
              );
            },
          })
          .then(() => {
            setUploads((prev) =>
              prev.map((task) =>
                task.id === id ? { ...task, status: 'done', progress: 100 } : task
              )
            );
            refresh();
          })
          .catch((err: unknown) => {
            const cancelled = controller.signal.aborted;
            setUploads((prev) =>
              prev.map((task) =>
                task.id === id
                  ? {
                      ...task,
                      status: cancelled ? 'cancelled' : 'error',
                      error: cancelled
                        ? undefined
                        : fileErrorMessage(err, 'The server rejected this upload.'),
                    }
                  : task
              )
            );
          })
          .finally(() => {
            uploadControllers.current.delete(id);
          });
      });
    },
    [currentPath, refresh]
  );

  const cancelUpload = useCallback((id: string) => {
    uploadControllers.current.get(id)?.abort();
  }, []);

  const dismissFinishedUploads = useCallback(() => {
    setUploads((prev) => prev.filter((task) => task.status === 'uploading'));
  }, []);

  useEffect(
    () => () => {
      uploadControllers.current.forEach((controller) => controller.abort());
      uploadControllers.current.clear();
    },
    []
  );

  /* ---------------------------------------------------------------------- */
  /* Disk usage                                                              */
  /* ---------------------------------------------------------------------- */

  const openDiskUsage = useCallback(async () => {
    setDialog({ kind: 'disk-usage' });
    setDiskUsage(null);
    setDiskUsageError(null);
    setDiskUsageBusy(true);
    try {
      setDiskUsage(await filesApi.diskUsage(currentPath));
    } catch (err) {
      setDiskUsageError(fileErrorMessage(err, 'Disk usage could not be read for this directory.'));
    } finally {
      setDiskUsageBusy(false);
    }
  }, [currentPath]);

  /* ---------------------------------------------------------------------- */
  /* Mutations                                                               */
  /* ---------------------------------------------------------------------- */

  const afterChange = useCallback(
    (message: string) => {
      setNotice(message);
      setActionError(null);
      setSelected([]);
      refresh();
    },
    [refresh]
  );

  const createFolder = useCallback(
    async (name: string) => {
      const target = joinPath(currentPath, name);
      try {
        await filesApi.mkdir(target);
      } catch (err) {
        throw new Error(fileErrorMessage(err, 'The directory could not be created.'));
      }
      setDialog({ kind: 'none' });
      afterChange(`Created ${target}`);
    },
    [currentPath, afterChange]
  );

  const startNewFile = useCallback(
    async (name: string) => {
      setDialog({ kind: 'none' });
      setEditorTarget({ path: joinPath(currentPath, name), name, isNew: true });
    },
    [currentPath]
  );

  const renameEntry = useCallback(
    async (entry: FileEntry, name: string) => {
      const target = joinPath(dirName(entry.path), name);
      try {
        await filesApi.rename(entry.path, target);
      } catch (err) {
        throw new Error(fileErrorMessage(err, 'The rename did not go through.'));
      }
      setDialog({ kind: 'none' });
      afterChange(`Renamed to ${name}`);
    },
    [afterChange]
  );

  const transfer = useCallback(
    async (
      mode: 'copy' | 'move',
      targets: { entry: FileEntry; destination: string }[]
    ): Promise<string[]> => {
      const failures: string[] = [];
      for (const target of targets) {
        try {
          if (mode === 'copy') {
            // eslint-disable-next-line no-await-in-loop
            await filesApi.copy(target.entry.path, target.destination);
          } else {
            // eslint-disable-next-line no-await-in-loop
            await filesApi.rename(target.entry.path, target.destination);
          }
        } catch (err) {
          failures.push(
            `${target.entry.name}: ${fileErrorMessage(err, mode === 'copy' ? 'could not be copied' : 'could not be moved')}`
          );
        }
      }
      if (failures.length < targets.length) {
        afterChange(
          `${targets.length - failures.length} of ${targets.length} ${mode === 'copy' ? 'copied' : 'moved'}.`
        );
      }
      return failures;
    },
    [afterChange]
  );

  const applyChmod = useCallback(
    async (mode: string, items: FileEntry[]): Promise<string[]> => {
      const failures = await runPerItem(
        items,
        (entry) => filesApi.chmod(entry.path, mode),
        'the mode could not be changed'
      );
      if (failures.length < items.length) {
        afterChange(`Set ${mode} on ${items.length - failures.length} of ${items.length} items.`);
      }
      return failures;
    },
    [afterChange]
  );

  const confirmDelete = useCallback(
    async (items: FileEntry[]): Promise<string[]> => {
      const failures = await runPerItem(
        items,
        (entry) => filesApi.remove(entry.path),
        'it could not be deleted'
      );
      if (failures.length < items.length) {
        afterChange(`Deleted ${items.length - failures.length} of ${items.length} items.`);
      }
      return failures;
    },
    [afterChange]
  );

  /* ---------------------------------------------------------------------- */
  /* Render                                                                  */
  /* ---------------------------------------------------------------------- */

  const crumbs = crumbsWithin(rootPath || '/', currentPath, ROOT_LABEL);
  const parent = parentWithin(rootPath || '/', currentPath);
  const atRoot = parent === currentPath;
  const searchMode = searchResults !== null;

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0">
          <h1 className="text-xl font-semibold text-gray-900">Files</h1>
          <p className="mt-1 text-sm text-gray-600">
            Everything below is inside the file manager root. The server refuses any path outside
            it, and refuses /etc, /root, /proc, /sys, /dev, /boot and the panel’s own directories
            even when they sit inside it.
          </p>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <Button type="button" variant="secondary" onClick={openDiskUsage}>
            <HardDrive size={16} aria-hidden="true" />
            Disk usage
          </Button>
          <Button type="button" variant="secondary" onClick={refresh}>
            <RefreshCw size={16} aria-hidden="true" />
            Refresh
          </Button>
        </div>
      </div>

      <div className="rounded-md border border-gray-200 bg-white px-4 py-3 shadow-sm">
        <p className="text-xs font-medium uppercase tracking-wide text-gray-500">
          File manager root
        </p>
        <p className="mt-0.5 break-all font-mono text-sm text-gray-900">
          {rootResolved ? rootPath || '/' : 'Reading...'}
        </p>
        {rootError ? <p className="mt-1 text-sm text-amber-700">{rootError}</p> : null}
      </div>

      {listError ? (
        <div
          className="rounded-md border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700"
          role="alert"
        >
          {listError}
        </div>
      ) : null}

      {actionError ? (
        <div
          className="flex items-start justify-between gap-3 rounded-md border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700"
          role="alert"
        >
          <span>{actionError}</span>
          <button
            type="button"
            onClick={() => setActionError(null)}
            aria-label="Dismiss error"
            className="shrink-0 rounded p-0.5 hover:bg-red-100 focus:outline-none focus:ring-2 focus:ring-brand-500"
          >
            <X size={14} aria-hidden="true" />
          </button>
        </div>
      ) : null}

      {notice ? (
        <div
          className="flex items-start justify-between gap-3 rounded-md border border-emerald-200 bg-emerald-50 px-4 py-3 text-sm text-emerald-700"
          role="status"
        >
          <span>{notice}</span>
          <button
            type="button"
            onClick={() => setNotice(null)}
            aria-label="Dismiss message"
            className="shrink-0 rounded p-0.5 hover:bg-emerald-100 focus:outline-none focus:ring-2 focus:ring-brand-500"
          >
            <X size={14} aria-hidden="true" />
          </button>
        </div>
      ) : null}

      <UploadPanel
        tasks={uploads}
        onCancel={cancelUpload}
        onDismissFinished={dismissFinishedUploads}
      />

      <div className="grid grid-cols-1 gap-4 lg:grid-cols-[17rem_minmax(0,1fr)]">
        <DirectoryTree
          rootPath={rootPath || '/'}
          rootLabel={ROOT_LABEL}
          currentPath={currentPath}
          onNavigate={navigate}
          reloadToken={refreshToken}
        />

        <section className="min-w-0 rounded-lg border border-gray-200 bg-white shadow-sm">
          <div className="flex flex-wrap items-center gap-2 border-b border-gray-200 px-4 py-3">
            <Button
              type="button"
              variant="secondary"
              size="sm"
              onClick={() => navigate(parent)}
              disabled={atRoot || !rootResolved}
              title={atRoot ? 'This is the file manager root' : `Go to ${parent}`}
            >
              <CornerLeftUp size={14} aria-hidden="true" />
              Up
            </Button>
            <nav aria-label="Breadcrumb" className="flex min-w-0 flex-wrap items-center gap-1">
              {crumbs.map((crumb, index) => (
                <span key={crumb.path} className="flex items-center gap-1">
                  {index > 0 ? (
                    <ChevronRight size={14} className="text-gray-400" aria-hidden="true" />
                  ) : null}
                  {index === crumbs.length - 1 ? (
                    <span className="max-w-[16rem] truncate text-sm font-medium text-gray-900" title={crumb.path}>
                      {crumb.label}
                    </span>
                  ) : (
                    <button
                      type="button"
                      onClick={() => navigate(crumb.path)}
                      title={crumb.path}
                      className="max-w-[12rem] truncate text-sm text-brand-700 hover:underline focus:outline-none focus:ring-2 focus:ring-brand-500"
                    >
                      {crumb.label}
                    </button>
                  )}
                </span>
              ))}
            </nav>
          </div>

          <div className="flex flex-wrap items-center gap-2 border-b border-gray-200 px-4 py-3">
            <Button type="button" size="sm" onClick={() => setDialog({ kind: 'new-file' })}>
              <FilePlus2 size={14} aria-hidden="true" />
              New file
            </Button>
            <Button
              type="button"
              variant="secondary"
              size="sm"
              onClick={() => setDialog({ kind: 'new-folder' })}
            >
              <FolderPlus size={14} aria-hidden="true" />
              New folder
            </Button>
            <Button
              type="button"
              variant="secondary"
              size="sm"
              onClick={() => fileInputRef.current?.click()}
            >
              <ArrowUpFromLine size={14} aria-hidden="true" />
              Upload
            </Button>
            <input
              ref={fileInputRef}
              type="file"
              multiple
              className="hidden"
              onChange={(event) => {
                if (event.target.files) startUploads(event.target.files);
                event.target.value = '';
              }}
            />

            <span className="mx-1 h-5 w-px bg-gray-200" aria-hidden="true" />

            <Button
              type="button"
              variant="secondary"
              size="sm"
              disabled={selectedEntries.length === 0}
              onClick={() => void downloadSelected()}
            >
              <Download size={14} aria-hidden="true" />
              Download
            </Button>
            <Button
              type="button"
              variant="secondary"
              size="sm"
              disabled={selectedEntries.length === 0}
              onClick={() => setDialog({ kind: 'copy', items: selectedEntries })}
            >
              <CopyIcon size={14} aria-hidden="true" />
              Copy
            </Button>
            <Button
              type="button"
              variant="secondary"
              size="sm"
              disabled={selectedEntries.length === 0}
              onClick={() => setDialog({ kind: 'move', items: selectedEntries })}
            >
              <Move size={14} aria-hidden="true" />
              Move
            </Button>
            <Button
              type="button"
              variant="secondary"
              size="sm"
              disabled={selectedEntries.length === 0}
              onClick={() => setDialog({ kind: 'chmod', items: selectedEntries })}
            >
              <ShieldCheck size={14} aria-hidden="true" />
              Permissions
            </Button>
            <Button
              type="button"
              variant="danger"
              size="sm"
              disabled={selectedEntries.length === 0}
              onClick={() => setDialog({ kind: 'delete', items: selectedEntries })}
            >
              <Trash2 size={14} aria-hidden="true" />
              Delete
            </Button>

            <div className="ml-auto flex items-center gap-2">
              <label htmlFor="file-search" className="sr-only">
                Search this directory
              </label>
              <Input
                id="file-search"
                value={patternInput}
                onChange={(event) => setPatternInput(event.target.value)}
                onKeyDown={(event) => {
                  if (event.key === 'Enter') void runSearch();
                }}
                placeholder="*.php"
                spellCheck={false}
                autoComplete="off"
                className="h-8 w-40 text-xs"
              />
              <Button
                type="button"
                variant="secondary"
                size="sm"
                onClick={() => void runSearch()}
                disabled={searchBusy}
              >
                <Search size={14} aria-hidden="true" />
                {searchBusy ? 'Searching...' : 'Search'}
              </Button>
            </div>
          </div>

          {searchMode ? (
            <div className="flex flex-wrap items-center justify-between gap-2 border-b border-gray-200 bg-gray-50 px-4 py-2">
              <p className="text-sm text-gray-700">
                {searchLabel} under <span className="font-mono">{currentPath}</span>. The server
                searches five directory levels deep and reports no owner for a match.
              </p>
              <button
                type="button"
                onClick={clearSearch}
                className="text-sm text-brand-700 hover:underline focus:outline-none focus:ring-2 focus:ring-brand-500"
              >
                Back to the directory
              </button>
            </div>
          ) : null}

          {selectedEntries.length > 0 ? (
            <div className="border-b border-gray-200 bg-gray-50 px-4 py-2 text-sm text-gray-700">
              {selectedEntries.length} selected
            </div>
          ) : null}

          <FileTable
            entries={visible}
            loading={listLoading && !searchMode}
            searchMode={searchMode}
            sortKey={sortKey}
            sortDirection={sortDirection}
            onSort={onSort}
            selected={selected}
            onToggleSelected={toggleSelected}
            onToggleAll={toggleAll}
            onOpenDirectory={navigate}
            onEdit={(entry) =>
              setEditorTarget({ path: entry.path, name: entry.name, entry, isNew: false })
            }
            onDownload={(entry) => void downloadOne(entry)}
            onRename={(entry) => setDialog({ kind: 'rename', entry })}
            onPermissions={(entry) => setDialog({ kind: 'chmod', items: [entry] })}
            onDelete={(entry) => setDialog({ kind: 'delete', items: [entry] })}
            busyPath={busyPath}
          />
        </section>
      </div>

      <NameDialog
        open={dialog.kind === 'new-folder'}
        title="New folder"
        description={`It is created inside ${currentPath}.`}
        label="Folder name"
        initialValue=""
        confirmLabel="Create folder"
        onClose={() => setDialog({ kind: 'none' })}
        onSubmit={createFolder}
      />

      <NameDialog
        open={dialog.kind === 'new-file'}
        title="New file"
        description={`It is created inside ${currentPath} when you save. The API rejects an empty file, so it needs at least one character of content.`}
        label="File name"
        initialValue=""
        confirmLabel="Open editor"
        onClose={() => setDialog({ kind: 'none' })}
        onSubmit={startNewFile}
      />

      <NameDialog
        open={dialog.kind === 'rename'}
        title="Rename"
        description={
          dialog.kind === 'rename' ? (
            <span className="break-all font-mono text-xs">{dialog.entry.path}</span>
          ) : undefined
        }
        label="New name"
        initialValue={dialog.kind === 'rename' ? dialog.entry.name : ''}
        confirmLabel="Rename"
        onClose={() => setDialog({ kind: 'none' })}
        onSubmit={(name) =>
          dialog.kind === 'rename' ? renameEntry(dialog.entry, name) : Promise.resolve()
        }
      />

      <TransferDialog
        open={dialog.kind === 'copy' || dialog.kind === 'move'}
        mode={dialog.kind === 'move' ? 'move' : 'copy'}
        items={dialog.kind === 'copy' || dialog.kind === 'move' ? dialog.items : []}
        currentPath={currentPath}
        onClose={() => setDialog({ kind: 'none' })}
        onSubmit={(targets) => transfer(dialog.kind === 'move' ? 'move' : 'copy', targets)}
      />

      <PermissionsDialog
        open={dialog.kind === 'chmod'}
        items={dialog.kind === 'chmod' ? dialog.items : []}
        onClose={() => setDialog({ kind: 'none' })}
        onSubmit={applyChmod}
      />

      <DeleteDialog
        open={dialog.kind === 'delete'}
        items={dialog.kind === 'delete' ? dialog.items : []}
        onClose={() => setDialog({ kind: 'none' })}
        onConfirm={confirmDelete}
      />

      <Modal
        open={dialog.kind === 'disk-usage'}
        title="Disk usage"
        description={
          <span className="break-all font-mono text-xs">{currentPath}</span>
        }
        onClose={() => setDialog({ kind: 'none' })}
        footer={
          <Button type="button" variant="secondary" onClick={() => setDialog({ kind: 'none' })}>
            Close
          </Button>
        }
      >
        {diskUsageBusy ? (
          <p className="text-sm text-gray-600">Measuring this directory...</p>
        ) : diskUsageError ? (
          <p className="text-sm text-red-700" role="alert">
            {diskUsageError}
          </p>
        ) : (
          <div className="space-y-3">
            <div>
              <p className="text-xs font-medium uppercase tracking-wide text-gray-500">
                This directory
              </p>
              <p className="text-lg font-semibold text-gray-900">{diskUsage?.size || '—'}</p>
            </div>
            <div>
              <p className="text-xs font-medium uppercase tracking-wide text-gray-500">
                Filesystem
              </p>
              <pre className="mt-1 overflow-x-auto rounded-md border border-gray-200 bg-gray-50 p-3 text-xs text-gray-700">
                {diskUsage?.filesystem?.trim() || 'The server reported no filesystem detail.'}
              </pre>
            </div>
          </div>
        )}
      </Modal>

      <FileEditor
        open={editorTarget !== null}
        target={editorTarget}
        onClose={() => setEditorTarget(null)}
        onSaved={(path) => {
          setNotice(`Saved ${path}`);
          refresh();
        }}
      />
    </div>
  );
}
