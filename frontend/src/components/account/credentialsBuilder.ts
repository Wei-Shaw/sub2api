export function applyInterceptWarmup(
  credentials: Record<string, unknown>,
  enabled: boolean,
  mode: 'create' | 'edit'
): void {
  if (enabled) {
    credentials.intercept_warmup_requests = true
  } else if (mode === 'edit') {
    delete credentials.intercept_warmup_requests
  }
}

export function applyRefreshTokenFallback(
  credentials: Record<string, unknown>,
  refreshToken: string
): void {
  const fallback = refreshToken.trim()
  const current =
    typeof credentials.refresh_token === 'string' ? credentials.refresh_token.trim() : ''

  if (!current && fallback) {
    credentials.refresh_token = fallback
  }
}
