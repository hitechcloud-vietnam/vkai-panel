/**
 * The shapes the file manager API actually returns.
 *
 * Every field here was read off core/internal/service/file_manager.go and
 * core/internal/handler/file_manager.go. Fields the API does not send are not
 * declared, so no screen can render a column the backend never fills.
 *
 * Two things are worth knowing before reading further:
 *
 *  - `path` is the ABSOLUTE path on the server, already resolved through the
 *    jail. The UI navigates with these values rather than building its own,
 *    because the API re-anchors anything else under the root anyway.
 *  - `owner` and `mime_type` are populated by /files/list but NOT by
 *    /files/search, which builds its rows from `find` output and leaves both
 *    empty. They are optional for that reason.
 */

/** One row of a directory listing. Mirrors service.FileInfo. */
export interface FileEntry {
  name: string;
  /** Absolute path on the server, inside the file manager root. */
  path: string;
  size: number;
  /** Go's FileMode string, e.g. "-rw-r--r--" or "drwxr-xr-x". */
  mode: string;
  is_dir: boolean;
  /** "2006-01-02 15:04:05" in the server's local time. No zone is sent. */
  mod_time: string;
  /** From `stat -c %U`; "unknown" when stat fails. Absent on search results. */
  owner?: string;
  /** Guessed from the extension by the API. Absent on search results. */
  mime_type?: string;
}

/** GET /api/v1/files/read. `size` is the byte length the API measured. */
export interface FileContent {
  path: string;
  content: string;
  size: number;
}

/** GET /api/v1/files/disk-usage. `size` is `du -sh`; `filesystem` is raw `df -h` output. */
export interface DiskUsage {
  size?: string;
  filesystem?: string;
}

/** Columns the listing can be ordered by. Directories always sort before files. */
export type FileSortKey = 'name' | 'size' | 'mode' | 'owner' | 'mod_time';

export type SortDirection = 'asc' | 'desc';

/** One file being uploaded, tracked so it can show progress and be cancelled. */
export interface UploadTask {
  id: string;
  name: string;
  /** Absolute destination path the file is being written to. */
  destination: string;
  size: number;
  /** 0-100, or null while the browser has not reported a total yet. */
  progress: number | null;
  status: 'uploading' | 'done' | 'error' | 'cancelled';
  error?: string;
}
