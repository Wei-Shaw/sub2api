import { afterEach, describe, expect, it } from 'vitest'
import { applyAppearance } from '../appearance'

afterEach(() => {
  document.getElementById('site-custom-css')?.remove()
  delete document.documentElement.dataset.siteTheme
  history.replaceState(null, '', '/')
})

describe('site appearance', () => {
  it('applies and replaces the configured theme and CSS', () => {
    applyAppearance('violet', 'body { color: red }')
    expect(document.documentElement.dataset.siteTheme).toBe('violet')
    expect(document.getElementById('site-custom-css')?.textContent).toBe('body { color: red }')
    applyAppearance('blue', 'body { color: blue }')
    expect(document.documentElement.dataset.siteTheme).toBe('blue')
    expect(document.querySelectorAll('#site-custom-css')).toHaveLength(1)
    expect(document.getElementById('site-custom-css')?.textContent).toBe('body { color: blue }')
    applyAppearance('unknown', '')
    expect(document.documentElement.dataset.siteTheme).toBe('teal')
    expect(document.getElementById('site-custom-css')).toBeNull()
  })

  it('disables custom CSS for recovery', () => {
    applyAppearance('rose', 'body { display: none }')
    history.replaceState(null, '', '/admin/settings?disable_custom_css=1')
    applyAppearance('rose', 'body { display: none }')
    expect(document.documentElement.dataset.siteTheme).toBe('rose')
    expect(document.getElementById('site-custom-css')).toBeNull()
  })
})
