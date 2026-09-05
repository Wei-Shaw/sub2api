export type AuthPersistence = 'persistent' | 'session'

export const AUTH_TOKEN_KEY = 'auth_token'
export const AUTH_USER_KEY = 'auth_user'
export const REFRESH_TOKEN_KEY = 'refresh_token'
export const TOKEN_EXPIRES_AT_KEY = 'token_expires_at'

const AUTH_KEYS = [AUTH_TOKEN_KEY, AUTH_USER_KEY, REFRESH_TOKEN_KEY, TOKEN_EXPIRES_AT_KEY]

export function getStorageForPersistence(persistence: AuthPersistence): Storage {
  return persistence === 'persistent' ? window.localStorage : window.sessionStorage
}

export function getActiveAuthStorage(): Storage {
  if (window.localStorage.getItem(AUTH_TOKEN_KEY) || window.localStorage.getItem(REFRESH_TOKEN_KEY)) {
    return window.localStorage
  }
  return window.sessionStorage
}

export function clearAuthStorage(): void {
  for (const storage of [window.localStorage, window.sessionStorage]) {
    for (const key of AUTH_KEYS) {
      storage.removeItem(key)
    }
  }
}
