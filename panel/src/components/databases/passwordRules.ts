/**
 * The password rules, copied from the two places the backend enforces them, so
 * an operator learns about a bad password while typing rather than from a 500.
 *
 *   models.CreateDBEntryRequest  `binding:"required,min=12,max=128"`
 *   utils.QuoteSQLLiteral        rejects backslash, NUL and control characters,
 *                                because MySQL and PostgreSQL disagree about
 *                                how a backslash escapes inside a literal.
 */

import { PASSWORD_MAX_LENGTH, PASSWORD_MIN_LENGTH } from '@/types/databases';

const CONTROL_CHARS = /[\u0000-\u001F\u007F]/;

/** Null when the password is acceptable, otherwise the reason it is not. */
export function passwordProblem(password: string): string | null {
  if (password.length < PASSWORD_MIN_LENGTH) {
    return `The password must be at least ${PASSWORD_MIN_LENGTH} characters.`;
  }
  if (password.length > PASSWORD_MAX_LENGTH) {
    return `The password must be at most ${PASSWORD_MAX_LENGTH} characters.`;
  }
  if (password.includes('\\')) {
    return 'The password cannot contain a backslash. MySQL and PostgreSQL quote it differently, so the server refuses it.';
  }
  if (CONTROL_CHARS.test(password)) {
    return 'The password cannot contain control characters.';
  }
  return null;
}

/**
 * A password built from the characters the backend will certainly accept: no
 * backslash, no quote, nothing a shell or a SQL literal argues about.
 */
export function generatePassword(length = 20): string {
  const alphabet =
    'abcdefghijkmnopqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789-_.=+@#%^';
  const out: string[] = [];
  if (typeof crypto !== 'undefined' && crypto.getRandomValues) {
    const bytes = new Uint32Array(length);
    crypto.getRandomValues(bytes);
    for (let i = 0; i < length; i += 1) {
      out.push(alphabet[bytes[i] % alphabet.length]);
    }
  } else {
    for (let i = 0; i < length; i += 1) {
      out.push(alphabet[Math.floor(Math.random() * alphabet.length)]);
    }
  }
  return out.join('');
}

/** Null when the identifier is acceptable, otherwise the reason it is not. */
export function identifierProblem(value: string, fieldName: string): string | null {
  if (!value) return `A ${fieldName} is required.`;
  if (value.length > 63) return `A ${fieldName} can be at most 63 characters.`;
  if (!/^[A-Za-z_][A-Za-z0-9_]*$/.test(value)) {
    return `A ${fieldName} must start with a letter or underscore and contain only letters, digits and underscores.`;
  }
  return null;
}
