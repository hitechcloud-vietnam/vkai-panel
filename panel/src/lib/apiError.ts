/**
 * Turning any API failure into a string that is safe to render.
 *
 * The API answers a failure with:
 *
 *   { "success": false, "error": { "code": "UNAUTHORIZED", "message": "invalid credentials" } }
 *
 * `error` is an OBJECT. Pages were reading it as if it were a string:
 *
 *   setError(err?.response?.data?.error || 'Failed to load deployments')
 *
 * An object is truthy, so the fallback never ran, the object went into state,
 * and rendering it threw React error #31 - "Objects are not valid as a React
 * child" - which blanks the whole page. Deployments, SSL, Websites and the
 * dashboard all did this, so any API failure took the page down instead of
 * showing a message. The same pattern was copied from page to page, which is
 * why one helper replaces fifteen call sites.
 *
 * Everything here returns a string. There is no code path that hands a caller
 * an object, because that is the bug this file exists to make impossible.
 */

/** Shape of the error envelope the API returns. Every field is optional on purpose. */
interface ApiErrorEnvelope {
  code?: unknown;
  message?: unknown;
  details?: unknown;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null;
}

function firstString(...values: unknown[]): string {
  for (const value of values) {
    if (typeof value === 'string' && value.trim() !== '') {
      return value.trim();
    }
  }
  return '';
}

/**
 * errorMessage extracts something a human can read from whatever the caller
 * caught: an axios error, a bare Error, a string, or nothing at all.
 *
 * `fallback` is returned only when no message could be found anywhere. Callers
 * should pass one that says what failed, because "Something went wrong" tells
 * an operator nothing about which of the six requests on the page it was.
 */
export function errorMessage(err: unknown, fallback = 'Unexpected error'): string {
  if (typeof err === 'string' && err.trim() !== '') {
    return err.trim();
  }

  const response = isRecord(err) && isRecord(err.response) ? err.response : undefined;
  const data = response && isRecord(response.data) ? response.data : undefined;

  // { error: { code, message } } - the shape this API actually returns.
  if (data && isRecord(data.error)) {
    const envelope = data.error as ApiErrorEnvelope;
    const message = firstString(envelope.message);
    if (message) return message;

    // A code without a message is not friendly, but it is still better than a
    // blank page, and it is something an operator can search for.
    const code = firstString(envelope.code);
    if (code) return code;
  }

  // { error: "..." } - some endpoints answer with a plain string.
  const flat = data ? firstString(data.error, data.message) : '';
  if (flat) return flat;

  // Network failures never reach the server, so there is no envelope at all.
  const status = response && typeof response.status === 'number' ? response.status : 0;
  if (status === 0 && isRecord(err) && firstString(err.code) === 'ECONNABORTED') {
    return 'The request timed out before the panel answered.';
  }

  const native = isRecord(err) ? firstString(err.message) : '';
  if (native) return native;

  return fallback;
}

/**
 * errorCode returns the machine-readable code when there is one, for the rare
 * caller that needs to branch on it. It never returns an object either.
 */
export function errorCode(err: unknown): string {
  const response = isRecord(err) && isRecord(err.response) ? err.response : undefined;
  const data = response && isRecord(response.data) ? response.data : undefined;
  if (data && isRecord(data.error)) {
    return firstString((data.error as ApiErrorEnvelope).code);
  }
  return '';
}

/** httpStatus is 0 when the request never reached the server. */
export function httpStatus(err: unknown): number {
  const response = isRecord(err) && isRecord(err.response) ? err.response : undefined;
  return response && typeof response.status === 'number' ? response.status : 0;
}
