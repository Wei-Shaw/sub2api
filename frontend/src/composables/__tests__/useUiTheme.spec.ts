import { beforeEach, describe, expect, it } from 'vitest'

import { initUiTheme, setUiTheme, useUiTheme } from '../useUiTheme'

describe('useUiTheme', () => {
  beforeEach(() => {
    localStorage.clear()
    delete document.documentElement.dataset.uiTheme
    initUiTheme()
  })

  it('uses the original interface by default', () => {
    expect(useUiTheme().uiTheme.value).toBe('original')
    expect(document.documentElement.dataset.uiTheme).toBe('original')
  })

  it('restores the saved simple interface', () => {
    localStorage.setItem('ui-theme', 'simple')

    initUiTheme()

    expect(useUiTheme().uiTheme.value).toBe('simple')
    expect(document.documentElement.dataset.uiTheme).toBe('simple')
  })

  it('persists and applies interface changes', () => {
    setUiTheme('simple')

    expect(localStorage.getItem('ui-theme')).toBe('simple')
    expect(document.documentElement.dataset.uiTheme).toBe('simple')
  })

  it('falls back to the original interface for an unknown stored value', () => {
    localStorage.setItem('ui-theme', 'unknown')

    initUiTheme()

    expect(useUiTheme().uiTheme.value).toBe('original')
    expect(document.documentElement.dataset.uiTheme).toBe('original')
  })
})
