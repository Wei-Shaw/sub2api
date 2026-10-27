export function buildProxyConsoleURL(consoleURL: string): string {
  const url = new URL(consoleURL)
  if (url.protocol !== 'http:' && url.protocol !== 'https:') {
    throw new Error('Proxy console URL must use HTTP or HTTPS')
  }
  if (url.username || url.password || url.searchParams.has('secret')) {
    throw new Error('Proxy console URL must not contain credentials or a secret')
  }

  const normalizedPath = url.pathname.replace(/\/+$/, '')
  if (!normalizedPath.endsWith('/ui')) {
    return url.toString()
  }

  url.pathname = `${normalizedPath}/`
  url.search = ''
  url.searchParams.set('hostname', url.hostname)
  if (url.port) {
    url.searchParams.set('port', url.port)
  }
  url.searchParams.set(url.protocol === 'https:' ? 'https' : 'http', '1')
  url.hash = '/proxies'
  return url.toString()
}
