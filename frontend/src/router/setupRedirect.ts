export function resolveCompletedSetupRedirectPath(isAuthenticated: boolean, isAdmin: boolean): string {
  if (!isAuthenticated) {
    return '/login'
  }

  return isAdmin ? '/admin/dashboard' : '/dashboard'
}
