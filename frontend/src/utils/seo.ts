import type { RouteLocationNormalizedLoaded } from 'vue-router'
import type { PublicSettings } from '@/types'
import { i18n } from '@/i18n'

const DEFAULT_SITE_NAME = 'Sub2API'
const DEFAULT_DESCRIPTION =
  'Sub2API is an AI API gateway platform for unified model access, account pooling, routing, billing, and operations management.'

export type SEOInput = {
  title: string
  description: string
  canonicalUrl?: string
  imageUrl?: string
  robots?: string
  type?: string
  jsonLD?: Record<string, unknown> | null
}

export function updateRouteSEO(
  route: RouteLocationNormalizedLoaded,
  settings: PublicSettings | null | undefined
): void {
  if (typeof document === 'undefined') {
    return
  }

  const seo = resolveRouteSEO(route, settings)
  document.title = seo.title

  setMetaByName('description', seo.description)
  setMetaByName('robots', seo.robots || 'noindex, nofollow')
  setLinkCanonical(seo.canonicalUrl)
  setMetaByProperty('og:type', seo.type || 'website')
  setMetaByProperty('og:title', seo.title)
  setMetaByProperty('og:description', seo.description)
  setMetaByProperty('og:url', seo.canonicalUrl)
  setMetaByProperty('og:site_name', getSiteName(settings))
  setMetaByName('twitter:card', 'summary_large_image')
  setMetaByName('twitter:title', seo.title)
  setMetaByName('twitter:description', seo.description)

  if (seo.imageUrl) {
    setMetaByProperty('og:image', seo.imageUrl)
    setMetaByName('twitter:image', seo.imageUrl)
  } else {
    removeMetaByProperty('og:image')
    removeMetaByName('twitter:image')
  }

  setJSONLD(seo.jsonLD)
}

export function resolveRouteSEO(
  route: RouteLocationNormalizedLoaded,
  settings: PublicSettings | null | undefined
): SEOInput {
  const siteName = getSiteName(settings)
  const baseUrl = getFrontendBaseURL(settings)
  const canonicalUrl = resolveCanonicalUrl(baseUrl, route.fullPath || route.path)
  const imageUrl = resolveRouteImageUrl(route, baseUrl, settings?.site_logo)
  const defaultTitle = settings?.seo_default_title?.trim()
  const homeTitle = settings?.seo_home_title?.trim()
  const defaultDescription = settings?.seo_default_description?.trim()
  const homeDescription = settings?.seo_home_description?.trim()
  const publicDefaultRobots = settings?.seo_default_robots?.trim() || 'index, follow'
  const defaultRobots = publicDefaultRobots
  const homeRobots = settings?.seo_home_robots?.trim() || publicDefaultRobots

  if (route.name === 'Home') {
    const description =
      homeDescription ||
      defaultDescription ||
      extractTextSummary(settings?.home_content) ||
      settings?.site_subtitle?.trim() ||
      i18n.global.t('home.heroDescription') ||
      DEFAULT_DESCRIPTION
    return {
      title: homeTitle || defaultTitle || `${siteName} - AI API Gateway`,
      description,
      canonicalUrl,
      imageUrl: settings?.seo_default_og_image?.trim() || imageUrl,
      robots: homeRobots,
      type: 'website',
      jsonLD: {
        '@context': 'https://schema.org',
        '@type': 'WebSite',
        name: siteName,
        description,
        url: canonicalUrl,
      },
    }
  }

  if (route.name === 'LegalDocument') {
    const documentId = String(route.params.documentId || '').trim()
    const matchedDocument = settings?.login_agreement_documents?.find((item) => item.id === documentId)
    const pageTitle = matchedDocument?.title?.trim()
      || (typeof route.meta.title === 'string' ? route.meta.title : 'Legal Document')
    const title = matchedDocument?.seo_title?.trim() || buildPageTitle(pageTitle, defaultTitle, siteName)
    const description =
      matchedDocument?.seo_description?.trim() ||
      defaultDescription ||
      extractTextSummary(matchedDocument?.content_md) ||
      `Read ${pageTitle} on ${siteName}.`
    return {
      title,
      description,
      canonicalUrl,
      imageUrl: matchedDocument?.seo_og_image?.trim() || imageUrl,
      robots: matchedDocument?.seo_robots?.trim() || defaultRobots || 'index, follow',
      type: 'article',
    }
  }

  if (route.name === 'TutorialDocument') {
    const pageTitle = '教程文档'
    const description =
      defaultDescription ||
      `阅读 ${pageTitle}，了解 ${siteName} 的接入说明、使用方式与常见问题。`
    return {
      title: buildPageTitle(pageTitle, defaultTitle, siteName),
      description,
      canonicalUrl,
      imageUrl: settings?.seo_default_og_image?.trim() || imageUrl || (baseUrl ? `${baseUrl}/og/custom-tutorial.svg` : undefined),
      robots: defaultRobots || 'index, follow',
      type: 'article',
      jsonLD: {
        '@context': 'https://schema.org',
        '@type': 'Article',
        name: pageTitle,
        description,
        url: canonicalUrl,
      },
    }
  }

  if (route.name === 'CustomPage') {
    const pageId = String(route.params.id || '').trim()
    const page = settings?.custom_menu_items?.find((item) => item.id === pageId && item.visibility !== 'admin')
    const pageTitle = page?.label?.trim() || 'Custom Page'
    const description =
      page?.seo_description?.trim() ||
      defaultDescription ||
      `Learn more about ${pageTitle} on ${siteName}.`
    const isMarkdown = Boolean(page?.page_slug || page?.url?.startsWith('md:'))
    return {
      title: page?.seo_title?.trim() || buildPageTitle(pageTitle, defaultTitle, siteName),
      description,
      canonicalUrl,
      imageUrl: page?.seo_og_image?.trim() || imageUrl,
      robots: page?.seo_robots?.trim() || defaultRobots || 'index, follow',
      type: isMarkdown ? 'article' : 'website',
      jsonLD: {
        '@context': 'https://schema.org',
        '@type': isMarkdown ? 'Article' : 'WebPage',
        name: pageTitle,
        description,
        url: canonicalUrl,
      },
    }
  }

  if (route.name === 'NotFound') {
    const translatedTitle = i18n.global.t('errors.pageNotFound')
    const translatedDescription = i18n.global.t('errors.pageNotFoundDescription')
    const titleLabel = translatedTitle && translatedTitle !== 'errors.pageNotFound'
      ? translatedTitle
      : 'Page not found'
    const description = translatedDescription && translatedDescription !== 'errors.pageNotFoundDescription'
      ? translatedDescription
      : 'Page not found.'
    const title = `${siteName} - ${titleLabel}`
    return {
      title,
      description,
      canonicalUrl,
      imageUrl: settings?.seo_default_og_image?.trim() || imageUrl,
      robots: 'noindex, nofollow',
      type: 'website',
      jsonLD: {
        '@context': 'https://schema.org',
        '@type': 'WebPage',
        name: title,
        description,
        url: canonicalUrl,
      },
    }
  }

  const pageTitle = resolveTitle(route, siteName)
  const description = resolveDescription(route, settings)

  return {
    title: pageTitle || defaultTitle || siteName,
    description: description || defaultDescription || DEFAULT_DESCRIPTION,
    canonicalUrl,
    imageUrl: settings?.seo_default_og_image?.trim() || imageUrl,
    robots: route.meta.requiresAuth === false ? publicDefaultRobots : 'noindex, nofollow',
    type: 'website',
  }
}

function buildPageTitle(pageTitle: string, defaultTitle: string | undefined, siteName: string): string {
  const cleanPageTitle = pageTitle.trim()
  const cleanDefaultTitle = defaultTitle?.trim() || ''
  if (cleanDefaultTitle) {
    if (cleanPageTitle && cleanPageTitle !== cleanDefaultTitle && !cleanDefaultTitle.includes(cleanPageTitle)) {
      return `${cleanPageTitle} - ${cleanDefaultTitle}`
    }
    return cleanDefaultTitle
  }
  if (cleanPageTitle) {
    return `${cleanPageTitle} - ${siteName}`
  }
  return siteName
}

function extractTextSummary(value: string | undefined | null): string {
  const text = String(value ?? '')
    .replace(/```[\s\S]*?```/g, ' ')
    .replace(/!\[[^\]]*]\(([^)]+)\)/g, ' ')
    .replace(/\[([^\]]+)]\(([^)]+)\)/g, '$1')
    .replace(/<[^>]+>/g, ' ')
    .replace(/[#>*`_-]/g, ' ')
    .replace(/\s+/g, ' ')
    .trim()
  if (!text) {
    return ''
  }
  return text
}

export function getFrontendBaseURL(settings: PublicSettings | null | undefined): string {
  const configured = settings?.frontend_url?.trim()
  if (configured) {
    return configured.replace(/\/+$/, '')
  }
  if (typeof window !== 'undefined' && window.location?.origin) {
    return window.location.origin.replace(/\/+$/, '')
  }
  return ''
}

function getSiteName(settings: PublicSettings | null | undefined): string {
  return settings?.site_name?.trim() || DEFAULT_SITE_NAME
}

function resolveCanonicalUrl(baseUrl: string, fullPath: string): string | undefined {
  if (!baseUrl) {
    return undefined
  }
  const [pathname] = (fullPath || '/').split(/[?#]/, 1)
  const normalizedPath = pathname === '/home' ? '/' : pathname
  return `${baseUrl}${normalizedPath.startsWith('/') ? normalizedPath : `/${normalizedPath}`}`
}

function resolveImageUrl(baseUrl: string, logo?: string): string | undefined {
  const trimmed = logo?.trim()
  if (trimmed) {
    if (/^https?:\/\//i.test(trimmed)) {
      return trimmed
    }
    if (baseUrl) {
      return `${baseUrl}${trimmed.startsWith('/') ? trimmed : `/${trimmed}`}`
    }
    return trimmed
  }
  if (!baseUrl) {
    return undefined
  }
  return `${baseUrl}/og/home.svg`
}

function resolveRouteImageUrl(
  route: RouteLocationNormalizedLoaded,
  baseUrl: string,
  logo?: string
): string | undefined {
  if (baseUrl) {
    if (route.name === 'LegalDocument') {
      const id = String(route.params.documentId || '').trim()
      if (id) return `${baseUrl}/og/legal-${id}.svg`
    }
    if (route.name === 'CustomPage') {
      const id = String(route.params.id || '').trim()
      if (id) return `${baseUrl}/og/custom-${id}.svg`
    }
  }
  return resolveImageUrl(baseUrl, logo)
}

function resolveTitle(route: RouteLocationNormalizedLoaded, siteName: string): string {
  const titleKey = route.meta.titleKey as string | undefined
  if (titleKey) {
    const translated = i18n.global.t(titleKey)
    if (translated && translated !== titleKey) {
      return `${translated} - ${siteName}`
    }
  }
  const rawTitle = typeof route.meta.title === 'string' ? route.meta.title.trim() : ''
  if (rawTitle) {
    return `${rawTitle} - ${siteName}`
  }
  return siteName
}

function resolveDescription(
  route: RouteLocationNormalizedLoaded,
  settings: PublicSettings | null | undefined
): string {
  const descriptionKey = route.meta.descriptionKey as string | undefined
  if (descriptionKey) {
    const translated = i18n.global.t(descriptionKey)
    if (translated && translated !== descriptionKey) {
      return translated
    }
  }
  const rawDescription = typeof route.meta.description === 'string' ? route.meta.description.trim() : ''
  if (rawDescription) {
    return rawDescription
  }
  return (
    settings?.seo_default_description?.trim() ||
    settings?.site_subtitle?.trim() ||
    DEFAULT_DESCRIPTION
  )
}

function setMetaByName(name: string, content?: string): void {
  if (!content) {
    removeMetaByName(name)
    return
  }
  let tag = document.head.querySelector<HTMLMetaElement>(`meta[name="${name}"]`)
  if (!tag) {
    tag = document.createElement('meta')
    tag.setAttribute('name', name)
    document.head.appendChild(tag)
  }
  tag.setAttribute('content', content)
}

function removeMetaByName(name: string): void {
  document.head.querySelector(`meta[name="${name}"]`)?.remove()
}

function setMetaByProperty(property: string, content?: string): void {
  if (!content) {
    removeMetaByProperty(property)
    return
  }
  let tag = document.head.querySelector<HTMLMetaElement>(`meta[property="${property}"]`)
  if (!tag) {
    tag = document.createElement('meta')
    tag.setAttribute('property', property)
    document.head.appendChild(tag)
  }
  tag.setAttribute('content', content)
}

function removeMetaByProperty(property: string): void {
  document.head.querySelector(`meta[property="${property}"]`)?.remove()
}

function setLinkCanonical(href?: string): void {
  if (!href) {
    document.head.querySelector('link[rel="canonical"]')?.remove()
    return
  }
  let link = document.head.querySelector<HTMLLinkElement>('link[rel="canonical"]')
  if (!link) {
    link = document.createElement('link')
    link.setAttribute('rel', 'canonical')
    document.head.appendChild(link)
  }
  link.setAttribute('href', href)
}

function setJSONLD(payload: Record<string, unknown> | null | undefined): void {
  const existing = document.getElementById('route-jsonld')
  if (!payload) {
    existing?.remove()
    return
  }
  const script = existing ?? document.createElement('script')
  script.id = 'route-jsonld'
  script.setAttribute('type', 'application/ld+json')
  script.textContent = JSON.stringify(payload)
  if (!existing) {
    document.head.appendChild(script)
  }
}
