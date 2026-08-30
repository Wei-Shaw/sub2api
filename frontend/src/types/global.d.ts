import type { PublicSettings } from '@/types'

declare global {
  interface Window {
    __APP_CONFIG__?: PublicSettings
    Sub2API?: {
      getLocale: () => string
      setLocale: (locale: string) => Promise<void>
      getPublicSettings: () => PublicSettings | null
      destroy?: () => void
    }
  }
}

export {}
