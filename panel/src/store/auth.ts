'use client';

import { create } from 'zustand';
import { api, authApi, unwrap } from '@/services/api';

interface User {
  id: string;
  username: string;
  email: string;
  first_name: string;
  last_name: string;
  tenant_id: string;
  status: string;
}

/**
 * What the password step returned.
 *
 * `two_factor_required` means the password was right and the account owes a
 * second factor. No token was issued and none was stored: the challenge is not
 * a session, it authorises exactly one call to the exchange endpoint.
 */
export type LoginResult =
  | { status: 'authenticated' }
  | { status: 'two_factor_required'; challengeToken: string; challengeExpiresIn: number };

/** Why a two-factor exchange failed, and what the caller should do next. */
export type TwoFactorFailure =
  /** Wrong code, window still open: ask for the code again. */
  | 'retry'
  /** Challenge expired, spent or unknown: go back to the password step. */
  | 'expired'
  /** Rate limited or locked out: go back and wait. */
  | 'rate_limited'
  /** The panel cannot check second factors right now. */
  | 'unavailable'
  | 'unknown';

/**
 * TwoFactorError carries the replacement challenge when there is one. Every
 * attempt spends the challenge it was made with, so a client that keeps using
 * the old token after a wrong code would be refused on the next try.
 */
export class TwoFactorError extends Error {
  reason: TwoFactorFailure;
  challengeToken?: string;
  challengeExpiresIn?: number;

  constructor(reason: TwoFactorFailure, message: string, challengeToken?: string, challengeExpiresIn?: number) {
    super(message);
    this.name = 'TwoFactorError';
    this.reason = reason;
    this.challengeToken = challengeToken;
    this.challengeExpiresIn = challengeExpiresIn;
  }
}

interface AuthState {
  user: User | null;
  isAuthenticated: boolean;
  isLoading: boolean;
  login: (username: string, password: string) => Promise<LoginResult>;
  completeTwoFactor: (challengeToken: string, code: string) => Promise<void>;
  logout: () => void;
  loadUser: () => Promise<void>;
}

function storeTokens(accessToken: string, refreshToken: string) {
  localStorage.setItem('access_token', accessToken);
  localStorage.setItem('refresh_token', refreshToken);
}

/** Reads the panel's error envelope out of an axios failure. */
function errorCode(err: unknown): string {
  const response = (err as { response?: { data?: { error?: { code?: string } } } })?.response;
  return response?.data?.error?.code || '';
}

function errorText(err: unknown): string {
  const response = (err as { response?: { data?: { error?: { message?: string } } } })?.response;
  return response?.data?.error?.message || '';
}

function errorStatus(err: unknown): number {
  return (err as { response?: { status?: number } })?.response?.status || 0;
}

export const useAuthStore = create<AuthState>((set) => ({
  user: null,
  isAuthenticated: false,
  isLoading: false,

  login: async (username: string, password: string) => {
    set({ isLoading: true });
    try {
      const response = await api.post('/api/v1/auth/login', { username, password });
      const data = unwrap<any>(response, null) || {};

      // An account with a second factor gets a challenge and nothing else:
      // there is no token here to store, and storing one would be storing
      // something the server did not issue.
      if (data.two_factor_required) {
        set({ isLoading: false });
        return {
          status: 'two_factor_required',
          challengeToken: data.challenge_token,
          challengeExpiresIn: data.challenge_expires_in,
        };
      }

      storeTokens(data.access_token, data.refresh_token);
      set({ user: data.user, isAuthenticated: true, isLoading: false });
      return { status: 'authenticated' };
    } catch (error) {
      set({ isLoading: false });
      throw error;
    }
  },

  completeTwoFactor: async (challengeToken: string, code: string) => {
    set({ isLoading: true });
    try {
      const response = await authApi.twoFactor({ challenge_token: challengeToken, code });
      const data = unwrap<any>(response, null) || {};

      if (!data.access_token) {
        throw new TwoFactorError('unknown', 'Máy chủ không trả về phiên đăng nhập.');
      }

      storeTokens(data.access_token, data.refresh_token);
      set({ user: data.user, isAuthenticated: true, isLoading: false });
    } catch (error) {
      set({ isLoading: false });
      if (error instanceof TwoFactorError) throw error;

      const status = errorStatus(error);
      const failure = errorCode(error);
      const message = errorText(error);

      if (failure === 'TWO_FACTOR_VERIFICATION_FAILED') {
        // The attempt spent the challenge; the replacement rides along with
        // the failure and inherits the original deadline.
        const payload = (error as { response?: { data?: { data?: any } } })?.response?.data?.data;
        throw new TwoFactorError(
          'retry',
          'Mã xác thực không đúng. Kiểm tra lại ứng dụng xác thực và thử lại.',
          payload?.challenge_token,
          payload?.challenge_expires_in
        );
      }
      if (failure === 'TWO_FACTOR_CHALLENGE_INVALID') {
        throw new TwoFactorError(
          'expired',
          'Phiên xác thực đã hết hạn hoặc đã được dùng. Vui lòng đăng nhập lại.'
        );
      }
      if (status === 429) {
        throw new TwoFactorError(
          'rate_limited',
          message || 'Bạn đã thử quá nhiều lần. Vui lòng đợi rồi đăng nhập lại.'
        );
      }
      if (status === 503) {
        throw new TwoFactorError(
          'unavailable',
          message || 'Máy chủ tạm thời không kiểm tra được xác thực hai lớp. Liên hệ quản trị viên.'
        );
      }
      throw new TwoFactorError('unknown', message || 'Xác thực không thành công. Vui lòng đăng nhập lại.');
    }
  },

  logout: () => {
    localStorage.removeItem('access_token');
    localStorage.removeItem('refresh_token');
    set({ user: null, isAuthenticated: false });
  },

  loadUser: async () => {
    try {
      const response = await api.get('/api/v1/auth/me');
      set({ user: response.data.data, isAuthenticated: true });
    } catch {
      localStorage.removeItem('access_token');
      localStorage.removeItem('refresh_token');
      set({ user: null, isAuthenticated: false });
    }
  },
}));
