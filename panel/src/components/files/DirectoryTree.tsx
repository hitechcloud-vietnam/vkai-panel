'use client';

/**
 * The left pane: directories only, loaded one level at a time.
 *
 * There is no tree endpoint, and there is no recursive listing, so the tree is
 * assembled from GET /files/list calls - one per directory, made the first time
 * that directory is opened. That is also why only directories appear here: the
 * files are in the right pane, and duplicating them would double every request
 * for nothing.
 */

import { useCallback, useEffect, useRef, useState } from 'react';
import { ChevronDown, ChevronRight, Folder, FolderOpen, RefreshCw } from 'lucide-react';

import type { FileEntry } from '@/types/files';
import { filesApi, fileErrorMessage } from './api';
import { crumbsWithin } from './paths';

export interface DirectoryTreeProps {
  /** The jail root, as the API reports it. */
  rootPath: string;
  /** What to call the root on screen. */
  rootLabel: string;
  currentPath: string;
  onNavigate: (path: string) => void;
  /** Bumped by the page after a change, to make the tree re-read what it holds. */
  reloadToken: number;
}

interface NodeProps {
  path: string;
  label: string;
  depth: number;
  currentPath: string;
  childrenByPath: Record<string, FileEntry[]>;
  expanded: Record<string, boolean>;
  loadingPaths: Record<string, boolean>;
  errorPaths: Record<string, string>;
  onToggle: (path: string) => void;
  onNavigate: (path: string) => void;
}

function TreeNode({
  path,
  label,
  depth,
  currentPath,
  childrenByPath,
  expanded,
  loadingPaths,
  errorPaths,
  onToggle,
  onNavigate,
}: NodeProps) {
  const isOpen = Boolean(expanded[path]);
  const isCurrent = currentPath === path;
  const children = childrenByPath[path];
  const isLoading = Boolean(loadingPaths[path]);
  const error = errorPaths[path];

  return (
    <li>
      <div
        className={`flex items-center gap-1 rounded-md pr-2 ${
          isCurrent ? 'bg-brand-50' : 'hover:bg-gray-50'
        }`}
        style={{ paddingLeft: `${depth * 12 + 4}px` }}
      >
        <button
          type="button"
          onClick={() => onToggle(path)}
          aria-label={isOpen ? `Collapse ${label}` : `Expand ${label}`}
          aria-expanded={isOpen}
          className="shrink-0 rounded p-0.5 text-gray-500 hover:text-gray-900 focus:outline-none focus:ring-2 focus:ring-brand-500"
        >
          {isLoading ? (
            <RefreshCw size={14} className="animate-spin" aria-hidden="true" />
          ) : isOpen ? (
            <ChevronDown size={14} aria-hidden="true" />
          ) : (
            <ChevronRight size={14} aria-hidden="true" />
          )}
        </button>
        <button
          type="button"
          onClick={() => onNavigate(path)}
          title={path}
          className={`flex min-w-0 flex-1 items-center gap-1.5 py-1.5 text-left text-sm focus:outline-none focus:ring-2 focus:ring-brand-500 ${
            isCurrent ? 'font-medium text-brand-700' : 'text-gray-700'
          }`}
        >
          {isOpen ? (
            <FolderOpen size={14} className="shrink-0 text-gray-400" aria-hidden="true" />
          ) : (
            <Folder size={14} className="shrink-0 text-gray-400" aria-hidden="true" />
          )}
          <span className="truncate">{label}</span>
        </button>
      </div>

      {isOpen ? (
        <>
          {error ? (
            <p
              className="py-1 text-xs text-red-700"
              style={{ paddingLeft: `${depth * 12 + 26}px` }}
            >
              {error}
            </p>
          ) : null}
          {!error && children && children.length === 0 && !isLoading ? (
            <p
              className="py-1 text-xs text-gray-500"
              style={{ paddingLeft: `${depth * 12 + 26}px` }}
            >
              No subdirectories
            </p>
          ) : null}
          {children && children.length > 0 ? (
            <ul>
              {children.map((child) => (
                <TreeNode
                  key={child.path}
                  path={child.path}
                  label={child.name}
                  depth={depth + 1}
                  currentPath={currentPath}
                  childrenByPath={childrenByPath}
                  expanded={expanded}
                  loadingPaths={loadingPaths}
                  errorPaths={errorPaths}
                  onToggle={onToggle}
                  onNavigate={onNavigate}
                />
              ))}
            </ul>
          ) : null}
        </>
      ) : null}
    </li>
  );
}

export default function DirectoryTree({
  rootPath,
  rootLabel,
  currentPath,
  onNavigate,
  reloadToken,
}: DirectoryTreeProps) {
  const [childrenByPath, setChildrenByPath] = useState<Record<string, FileEntry[]>>({});
  const [expanded, setExpanded] = useState<Record<string, boolean>>({});
  const [loadingPaths, setLoadingPaths] = useState<Record<string, boolean>>({});
  const [errorPaths, setErrorPaths] = useState<Record<string, string>>({});

  /** Directories already asked for, so opening a node twice is one request. */
  const requested = useRef<Set<string>>(new Set());
  const expandedRef = useRef<Record<string, boolean>>({});
  expandedRef.current = expanded;

  const loadDir = useCallback(async (path: string, force = false) => {
    if (!force && requested.current.has(path)) return;
    requested.current.add(path);
    setLoadingPaths((prev) => ({ ...prev, [path]: true }));
    setErrorPaths((prev) => {
      if (!(path in prev)) return prev;
      const next = { ...prev };
      delete next[path];
      return next;
    });
    try {
      const entries = await filesApi.list(path);
      const dirs = entries
        .filter((entry) => entry.is_dir)
        .sort((a, b) => a.name.localeCompare(b.name));
      setChildrenByPath((prev) => ({ ...prev, [path]: dirs }));
    } catch (err) {
      setChildrenByPath((prev) => ({ ...prev, [path]: [] }));
      setErrorPaths((prev) => ({
        ...prev,
        [path]: fileErrorMessage(err, 'This directory could not be read.'),
      }));
    } finally {
      setLoadingPaths((prev) => {
        const next = { ...prev };
        delete next[path];
        return next;
      });
    }
  }, []);

  // A change anywhere in the tree invalidates everything that is open, so the
  // whole open set is re-read rather than only the directory that changed.
  useEffect(() => {
    const openPaths = Object.keys(expandedRef.current).filter((p) => expandedRef.current[p]);
    requested.current = new Set();
    void loadDir(rootPath, true);
    openPaths.forEach((p) => {
      if (p !== rootPath) void loadDir(p, true);
    });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [rootPath, reloadToken, loadDir]);

  // Walking to a directory opens the branch that leads to it, so the tree
  // always shows where the listing on the right is standing.
  useEffect(() => {
    const chain = crumbsWithin(rootPath, currentPath, rootLabel).map((crumb) => crumb.path);
    setExpanded((prev) => {
      const next = { ...prev };
      chain.forEach((p) => {
        next[p] = true;
      });
      return next;
    });
    chain.forEach((p) => {
      void loadDir(p);
    });
  }, [rootPath, rootLabel, currentPath, reloadToken, loadDir]);

  // The request is fired outside the state updater: React re-runs an updater
  // under StrictMode, and a fetch started inside one runs twice.
  const onToggle = useCallback(
    (path: string) => {
      const open = !expandedRef.current[path];
      setExpanded((prev) => ({ ...prev, [path]: open }));
      if (open) void loadDir(path);
    },
    [loadDir]
  );

  return (
    <nav
      aria-label="Directory tree"
      className="rounded-lg border border-gray-200 bg-white shadow-sm"
    >
      <div className="border-b border-gray-200 px-4 py-3">
        <h2 className="text-sm font-semibold text-gray-900">Directories</h2>
        <p className="mt-0.5 truncate text-xs text-gray-500" title={rootPath}>
          {rootPath}
        </p>
      </div>
      <div className="max-h-[28rem] overflow-auto px-2 py-2 lg:max-h-[calc(100vh-16rem)]">
        <ul>
          <TreeNode
            path={rootPath}
            label={rootLabel}
            depth={0}
            currentPath={currentPath}
            childrenByPath={childrenByPath}
            expanded={expanded}
            loadingPaths={loadingPaths}
            errorPaths={errorPaths}
            onToggle={onToggle}
            onNavigate={onNavigate}
          />
        </ul>
      </div>
    </nav>
  );
}
