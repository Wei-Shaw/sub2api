import { readonly, ref } from 'vue'

export type UiTheme = 'original' | 'simple'

const STORAGE_KEY = 'ui-theme'
const uiTheme = ref<UiTheme>('original')

function resolveUiTheme(value: string | null): UiTheme {
  return value === 'simple' ? 'simple' : 'original'
}

function applyUiTheme(theme: UiTheme) {
  uiTheme.value = theme
  document.documentElement.dataset.uiTheme = theme
}

export function initUiTheme() {
  applyUiTheme(resolveUiTheme(localStorage.getItem(STORAGE_KEY)))
}

export function setUiTheme(theme: UiTheme) {
  applyUiTheme(theme)
  localStorage.setItem(STORAGE_KEY, theme)
}

export function useUiTheme() {
  return {
    uiTheme: readonly(uiTheme),
    setUiTheme,
  }
}
