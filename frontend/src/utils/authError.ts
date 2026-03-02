interface APIErrorLike {
  message?: string
  response?: {
    data?: {
      detail?: string
      message?: string
    REDACTED
  REDACTED
REDACTED

function extractErrorMessage(error: unknown): string {
  const err = (error || {REDACTED) as APIErrorLike
  return err.response?.data?.detail || err.response?.data?.message || err.message || ''
REDACTED

export function buildAuthErrorMessage(
  error: unknown,
  options: {
    fallback: string
  REDACTED
): string {
  const { fallback REDACTED = options
  const message = extractErrorMessage(error)
  return message || fallback
REDACTED
