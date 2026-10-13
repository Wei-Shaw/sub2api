const marker = 'data-custom-head-script'

// Production HTML already contains these scripts. This covers standalone/dev pages.
export async function injectCustomHeadScript(source?: string): Promise<void> {
  if (!source?.trim() || document.head.querySelector(`script[${marker}]`)) return

  const scripts: HTMLScriptElement[] = []
  if (source.trimStart().startsWith('<')) {
    const template = document.createElement('template')
    template.innerHTML = source
    for (const node of template.content.childNodes) {
      if (node instanceof HTMLScriptElement) {
        scripts.push(node)
      } else if (node.nodeType !== Node.COMMENT_NODE && node.textContent?.trim()) {
        return
      } else if (node.nodeType === Node.ELEMENT_NODE) {
        return
      }
    }
  } else {
    const script = document.createElement('script')
    script.textContent = source
    scripts.push(script)
  }

  const nonce = document.querySelector<HTMLScriptElement>('script[nonce]')?.nonce || ''
  for (const original of scripts) {
    const script = document.createElement('script')
    for (const attr of original.attributes) {
      if (attr.name !== 'nonce') script.setAttribute(attr.name, attr.value)
    }
    script.setAttribute(marker, '')
    if (nonce) script.nonce = nonce
    script.textContent = original.textContent
    // Dynamic external scripts are async by default; preserve snippet ordering.
    const ignoredNoModule = script.hasAttribute('nomodule') && 'noModule' in script
    const waitForLoad = script.hasAttribute('src') && !script.hasAttribute('async') && !ignoredNoModule
    const loaded = waitForLoad
      ? new Promise<void>((resolve) => {
          script.addEventListener('load', () => resolve(), { once: true })
          script.addEventListener('error', () => resolve(), { once: true })
        })
      : null
    if (waitForLoad) script.async = false
    document.head.appendChild(script)
    if (loaded) await loaded
  }
}
