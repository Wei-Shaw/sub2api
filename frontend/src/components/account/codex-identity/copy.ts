import { useI18n } from 'vue-i18n'

export function useCodexIdentityCopy() {
  const { t, te } = useI18n()
  return (key: string, fallback: string): string =>
    typeof te === 'function' && te(key) ? String(t(key)) : fallback
}
