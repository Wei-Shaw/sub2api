export function resolveCompletedSetupRedirectPath(isAuthenticated: boolean, isAdmin: boolean): string {
  if (!isAuthenticated) {
    return '/login'
  REDACTED

  return isAdmin ? '/admin/dashboard' : '/dashboard'
REDACTED
