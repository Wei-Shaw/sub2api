import createDOMPurify from 'dompurify'
import { JSDOM } from 'jsdom'

export type PrerenderSettingsPayload = {
  code?: number
  data?: {
    frontend_url?: string
    site_name?: string
    site_subtitle?: string
    seo_default_title?: string
    seo_home_title?: string
    seo_default_description?: string
    seo_home_description?: string
    seo_default_og_image?: string
    seo_default_robots?: string
    seo_home_robots?: string
    login_agreement_documents?: Array<{ id?: string; title?: string; content_md?: string }>
    custom_menu_items?: Array<{
      id?: string
      label?: string
      url?: string
      page_slug?: string
      seo_title?: string
      seo_description?: string
      seo_og_image?: string
      seo_robots?: string
      visibility?: string
    }>
  }
}

export type PrerenderRouteEntry = {
  route: string
  title: string
  html?: string
  markdown?: string
  markdownSlug?: string
  source?: 'base' | 'legal' | 'custom-markdown' | 'tutorial'
  seoTitle?: string
  seoDescription?: string
  seoOGImage?: string
  seoRobots?: string
}

export const BASE_PUBLIC_PRERENDER_ROUTES = [
  '/home',
  '/docs/tutorial',
]

const prerenderWindow = new JSDOM('').window
const prerenderDOMPurify = createDOMPurify(
  prerenderWindow as unknown as Parameters<typeof createDOMPurify>[0]
)

function escapeHTML(value: string): string {
  return value
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;')
}

function sanitizePrerenderHTML(raw: string): string {
  return prerenderDOMPurify.sanitize(raw, {
    ADD_ATTR: ['style', 'target', 'rel', 'class'],
  })
}

export function collectPrerenderRoutes(
  payload: PrerenderSettingsPayload | null | undefined
): PrerenderRouteEntry[] {
  const routes = new Map<string, PrerenderRouteEntry>()
  for (const route of BASE_PUBLIC_PRERENDER_ROUTES) {
    routes.set(route, { route, title: '', source: 'base' })
  }
  const data = payload?.data

  routes.set('/docs/tutorial', {
    route: '/docs/tutorial',
    title: '教程文档',
    markdownSlug: 'tutorial',
    source: 'tutorial',
  })

  for (const item of data?.login_agreement_documents ?? []) {
    const id = String(item?.id ?? '').trim()
    if (id) {
      routes.set(`/legal/${id}`, {
        route: `/legal/${id}`,
        title: String(item?.title ?? '').trim(),
        markdown: String(item?.content_md ?? ''),
        source: 'legal',
      })
    }
  }

  for (const item of data?.custom_menu_items ?? []) {
    const id = String(item?.id ?? '').trim()
    const label = String(item?.label ?? '').trim()
    const visibility = String(item?.visibility ?? '').trim()
    const pageSlug = String(item?.page_slug ?? '').trim()
    const url = String(item?.url ?? '').trim()
    const isMarkdown = Boolean(pageSlug || url.startsWith('md:'))
    if (id && visibility !== 'admin' && isMarkdown) {
      routes.set(`/custom/${id}`, {
        route: `/custom/${id}`,
        title: label,
        markdownSlug: pageSlug || url.replace(/^md:/, '').trim(),
        source: 'custom-markdown',
      })
    }
  }

  return Array.from(routes.values()).sort((a, b) => a.route.localeCompare(b.route))
}

export function renderSimpleMarkdownHTML(markdown: string): string {
  const lines = markdown.replace(/\r\n/g, '\n').split('\n')
  const html: string[] = []
  let inList = false
  let inOrderedList = false
  let inCodeBlock = false
  let paragraph: string[] = []

  const inline = (value: string) =>
    escapeHTML(value)
      .replace(/`([^`]+)`/g, '<code>$1</code>')
      .replace(/\*\*([^*]+)\*\*/g, '<strong>$1</strong>')
      .replace(/\*([^*]+)\*/g, '<em>$1</em>')
      .replace(/!\[([^\]]*)\]\(([^)]+)\)/g, '<img src="$2" alt="$1">')
      .replace(/\[([^\]]+)\]\(([^)]+)\)/g, '<a href="$2">$1</a>')

  const flushParagraph = () => {
    if (paragraph.length > 0) {
      html.push(`<p>${inline(paragraph.join(' '))}</p>`)
      paragraph = []
    }
  }

  const closeLists = () => {
    if (inList) {
      html.push('</ul>')
      inList = false
    }
    if (inOrderedList) {
      html.push('</ol>')
      inOrderedList = false
    }
  }

  for (const rawLine of lines) {
    const line = rawLine.trimEnd()
    const trimmed = line.trim()

    if (trimmed.startsWith('```')) {
      flushParagraph()
      closeLists()
      html.push(inCodeBlock ? '</code></pre>' : '<pre><code>')
      inCodeBlock = !inCodeBlock
      continue
    }
    if (inCodeBlock) {
      html.push(line.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;'))
      continue
    }
    if (!trimmed) {
      flushParagraph()
      closeLists()
      continue
    }

    const heading = trimmed.match(/^(#{1,4})\s+(.+)$/)
    if (heading) {
      flushParagraph()
      closeLists()
      const level = heading[1].length
      html.push(`<h${level}>${inline(heading[2])}</h${level}>`)
      continue
    }

    if (/^[-*]\s+/.test(trimmed)) {
      flushParagraph()
      if (inOrderedList) {
        html.push('</ol>')
        inOrderedList = false
      }
      if (!inList) {
        html.push('<ul>')
        inList = true
      }
      html.push(`<li>${inline(trimmed.replace(/^[-*]\s+/, ''))}</li>`)
      continue
    }

    if (/^\d+\.\s+/.test(trimmed)) {
      flushParagraph()
      if (inList) {
        html.push('</ul>')
        inList = false
      }
      if (!inOrderedList) {
        html.push('<ol>')
        inOrderedList = true
      }
      html.push(`<li>${inline(trimmed.replace(/^\d+\.\s+/, ''))}</li>`)
      continue
    }

    paragraph.push(trimmed)
  }

  flushParagraph()
  closeLists()
  if (inCodeBlock) {
    html.push('</code></pre>')
  }
  return html.join('\n')
}

export function rewriteRelativeMarkdownImages(markdown: string, pageSlug?: string): string {
  const slug = String(pageSlug ?? '').trim()
  if (!slug) {
    return markdown
  }
  return markdown.replace(/!\[([^\]]*)\]\(([^)]+)\)/g, (_match, alt: string, src: string) => {
    if (!isRelativeMarkdownAsset(src)) {
      return `![${alt}](${src})`
    }
    return `![${alt}](${buildPageImageURL(slug, src)})`
  })
}

function isRelativeMarkdownAsset(src: string): boolean {
  const trimmed = String(src ?? '').trim()
  if (!trimmed || trimmed.startsWith('/') || trimmed.startsWith('//') || trimmed.includes('\\')) {
    return false
  }
  if (/^[a-zA-Z][a-zA-Z0-9+.-]*:/.test(trimmed)) {
    return false
  }
  return !trimmed.split('/').some((part) => part === '..')
}

export function buildPageImageURL(pageSlug: string, src: string): string {
  const trimmed = String(src ?? '').trim()
  const [pathPart, queryPart] = trimmed.split('?', 2)
  const encodedParts = pathPart
    .split('/')
    .filter((part) => part && part !== '.')
    .map((part) => encodeURIComponent(part))
  const query = queryPart ? `?${queryPart}` : ''
  return `/api/v1/pages/${encodeURIComponent(pageSlug)}/images/${encodedParts.join('/')}${query}`
}

export function injectPrerenderContent(indexHTML: string, entry: PrerenderRouteEntry): string {
  if (!entry.markdown && !entry.html) {
    return indexHTML
  }
  const safeTitle = entry.title || 'Document'
  const rewrittenMarkdown = rewriteRelativeMarkdownImages(entry.markdown || '', entry.markdownSlug)
  const contentHTML = sanitizePrerenderHTML(entry.html || renderSimpleMarkdownHTML(rewrittenMarkdown))
  const body = `
  <div class="min-h-screen bg-gray-50 text-gray-900">
    <main class="mx-auto max-w-6xl px-4 py-8 sm:px-6 lg:py-10">
      <article class="overflow-hidden rounded-2xl border border-gray-200 bg-white shadow-sm">
        <div class="border-b border-gray-200 px-6 py-5">
          <h1 class="mt-2 text-3xl font-bold text-gray-950">${escapeHTML(safeTitle)}</h1>
        </div>
        <div class="public-markdown-content p-6 md:p-10">${contentHTML}</div>
      </article>
    </main>
  </div>`
  return indexHTML.replace('<div id="app"></div>', `<div id="app">${body}</div>`)
}

export function buildPrerenderManifest(entries: PrerenderRouteEntry[]) {
  return {
    generated_at: new Date().toISOString(),
    total_routes: entries.length,
    routes: entries.map((entry) => ({
      route: entry.route,
      title: entry.title,
      source: entry.source || 'base',
      has_markdown: Boolean(entry.markdown),
      has_html: Boolean(entry.html),
      markdown_slug: entry.markdownSlug || '',
      output: `${entry.route.replace(/^\/+/, '') || 'index.html'}/index.html`,
    })),
  }
}
