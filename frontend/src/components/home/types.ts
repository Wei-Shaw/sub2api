export type HomeStyle =
  | 'classic'
  | 'compact'
  | 'studio'

export interface HomeStyleContext {
  siteName: string
  siteLogo: string
  siteSubtitle: string
  docUrl: string
  showModelPlazaEntry: boolean
  isAuthenticated: boolean
  dashboardPath: string
  userInitial: string
  isDark: boolean
  currentYear: number
  toggleTheme: () => void
}
