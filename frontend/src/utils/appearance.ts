const themes = new Set(['teal', 'blue', 'violet', 'rose', 'green'])

export function applyAppearance(theme?: string, css?: string): void {
  document.documentElement.dataset.siteTheme = theme && themes.has(theme) ? theme : 'teal'
  let style = document.getElementById('site-custom-css')
  const disabled = new URLSearchParams(window.location.search).get('disable_custom_css') === '1'
  if (!css || disabled) {
    style?.remove()
    return
  }
  if (!style) {
    style = document.createElement('style')
    style.id = 'site-custom-css'
    document.head.appendChild(style)
  }
  style.textContent = css
}
