'use client';

/**
 * Read a file, change it, save it.
 *
 * There is no syntax highlighter in this codebase, so the language is named
 * rather than coloured: the header says "PHP" or "nginx configuration" so the
 * operator can tell at a glance that they opened what they meant to.
 *
 * The editor refuses to open binary content instead of filling the screen with
 * control characters, and it says which check refused it. That decision is made
 * twice - once from the mime type in the listing, before the request, and again
 * from the bytes that came back, because .conf, .env and Dockerfile all arrive
 * as application/octet-stream and are ordinary text.
 *
 * One limit worth stating in the interface rather than discovering: POST
 * /files/write binds `content` with binding:"required", so the API rejects an
 * empty body. A file cannot be saved empty, and a new file cannot be created
 * empty. The editor says so instead of sending a request that will bounce.
 */

import { useCallback, useEffect, useState } from 'react';
import { AlertTriangle, FileText, RefreshCw } from 'lucide-react';

import { Button } from '@/components/ui/button';
import type { FileEntry } from '@/types/files';
import Modal from './Modal';
import { filesApi, fileErrorMessage, MAX_READ_BYTES } from './api';
import {
  binaryReasonFromContent,
  binaryReasonFromEntry,
  formatBytes,
  guessLanguage,
} from './format';

export interface EditorTarget {
  /** Absolute path being edited. */
  path: string;
  name: string;
  /** The listing row, when there is one. Absent for a file being created. */
  entry?: FileEntry;
  /** True when this file does not exist yet. */
  isNew: boolean;
}

export interface FileEditorProps {
  open: boolean;
  target: EditorTarget | null;
  onClose: () => void;
  onSaved: (path: string) => void;
}

export default function FileEditor({ open, target, onClose, onSaved }: FileEditorProps) {
  const [content, setContent] = useState('');
  const [original, setOriginal] = useState('');
  const [loading, setLoading] = useState(false);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [refusal, setRefusal] = useState<string | null>(null);
  const [saveError, setSaveError] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);
  const [confirmDiscard, setConfirmDiscard] = useState(false);

  const load = useCallback(async (item: EditorTarget) => {
    setContent('');
    setOriginal('');
    setLoadError(null);
    setRefusal(null);
    setSaveError(null);
    setConfirmDiscard(false);

    if (item.isNew) {
      setLoading(false);
      return;
    }

    const entry = item.entry;
    if (entry) {
      const preReason = binaryReasonFromEntry(entry);
      if (preReason) {
        setRefusal(preReason);
        setLoading(false);
        return;
      }
      if (entry.size > MAX_READ_BYTES) {
        setRefusal(
          `This file is ${formatBytes(entry.size)}. The server refuses to read anything over 10 MB, so it cannot be opened here. Download it instead.`
        );
        setLoading(false);
        return;
      }
    }

    setLoading(true);
    try {
      const file = await filesApi.read(item.path);
      const postReason = binaryReasonFromContent(file.content);
      if (postReason) {
        setRefusal(postReason);
        return;
      }
      setContent(file.content);
      setOriginal(file.content);
    } catch (err) {
      setLoadError(fileErrorMessage(err, 'This file could not be read.'));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    if (!open || !target) return;
    void load(target);
  }, [open, target, load]);

  if (!open || !target) return null;

  const dirty = content !== original;
  const language = guessLanguage(target.name);
  const canEdit = !refusal && !loadError && !loading;

  const save = async () => {
    if (content === '') {
      setSaveError(
        'The API rejects a file with no content, so this cannot be saved empty. Write at least one character, or delete the file instead.'
      );
      return;
    }
    setSaving(true);
    setSaveError(null);
    try {
      await filesApi.write(target.path, content);
      setOriginal(content);
      onSaved(target.path);
    } catch (err) {
      setSaveError(fileErrorMessage(err, 'This file could not be saved.'));
    } finally {
      setSaving(false);
    }
  };

  const requestClose = () => {
    if (dirty && !confirmDiscard) {
      setConfirmDiscard(true);
      return;
    }
    onClose();
  };

  return (
    <Modal
      open={open}
      size="xl"
      title={target.isNew ? `New file — ${target.name}` : target.name}
      description={
        <span className="block truncate font-mono text-xs" title={target.path}>
          {target.path}
        </span>
      }
      onClose={requestClose}
      footer={
        <>
          {confirmDiscard ? (
            <span className="mr-auto text-sm text-amber-700">
              There are unsaved changes. Closing loses them.
            </span>
          ) : null}
          <Button type="button" variant="secondary" onClick={requestClose} disabled={saving}>
            {confirmDiscard ? 'Close without saving' : 'Close'}
          </Button>
          <Button type="button" onClick={save} disabled={!canEdit || saving || (!dirty && !target.isNew)}>
            {saving ? 'Saving...' : 'Save'}
          </Button>
        </>
      }
    >
      <div className="space-y-3">
        <div className="flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-gray-600">
          <span className="inline-flex items-center gap-1.5">
            <FileText size={14} className="text-gray-400" aria-hidden="true" />
            {language}
          </span>
          <span>{content.length.toLocaleString()} characters</span>
          <span>{content ? content.split('\n').length : 0} lines</span>
          {dirty ? <span className="font-medium text-amber-700">Unsaved changes</span> : null}
        </div>

        {loading ? (
          <div className="flex h-64 items-center justify-center rounded-md border border-gray-200 bg-gray-50">
            <RefreshCw className="h-5 w-5 animate-spin text-gray-400" aria-hidden="true" />
            <span className="ml-2 text-sm text-gray-600">Reading file...</span>
          </div>
        ) : refusal ? (
          <div
            className="flex items-start gap-2 rounded-md border border-amber-200 bg-amber-50 px-3 py-3"
            role="alert"
          >
            <AlertTriangle size={16} className="mt-0.5 shrink-0 text-amber-700" aria-hidden="true" />
            <div>
              <p className="text-sm font-medium text-amber-700">This file was not opened.</p>
              <p className="mt-0.5 text-sm text-amber-700">{refusal}</p>
            </div>
          </div>
        ) : loadError ? (
          <div className="rounded-md border border-red-200 bg-red-50 px-3 py-3 text-sm text-red-700" role="alert">
            {loadError}
          </div>
        ) : (
          <textarea
            value={content}
            onChange={(event) => {
              setContent(event.target.value);
              setSaveError(null);
              setConfirmDiscard(false);
            }}
            spellCheck={false}
            aria-label={`Contents of ${target.name}`}
            className="h-[26rem] w-full rounded-md border border-gray-300 bg-white p-3 font-mono text-xs leading-relaxed text-gray-900 focus:border-brand-500 focus:outline-none focus:ring-1 focus:ring-brand-500"
          />
        )}

        {saveError ? (
          <p className="text-sm text-red-700" role="alert">
            {saveError}
          </p>
        ) : null}
      </div>
    </Modal>
  );
}
