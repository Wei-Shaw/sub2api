/**
 * 统一生成页面标题，避免多处写入 document.title 产生覆盖冲突。
 */
export function resolveDocumentTitle(routeTitle: unknown, siteName?: string): string {
  const normalizedSiteName = typeof siteName === 'string' && siteName.trim() ? siteName.trim() : 'Sub2API'

  if (typeof routeTitle === 'string' && routeTitle.trim()) {
    return `${routeTitle.trim()REDACTED - ${normalizedSiteNameREDACTED`
  REDACTED

  return normalizedSiteName
REDACTED
