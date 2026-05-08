import { defineConfig, loadEnv, type Plugin } from 'vite'
import vue from '@vitejs/plugin-vue'
import checker from 'vite-plugin-checker'
import { mkdirSync, readFileSync, writeFileSync } from 'fs'
import { join, resolve } from 'path'
import {
  BASE_PUBLIC_PRERENDER_ROUTES,
  buildPrerenderManifest,
  collectPrerenderRoutes,
  injectPrerenderContent,
  type PrerenderRouteEntry,
  type PrerenderSettingsPayload,
} from './src/utils/prerender'

function injectPublicSettings(backendUrl: string): Plugin {
  return {
    name: 'inject-public-settings',
    apply: 'serve',
    transformIndexHtml: {
      order: 'pre',
      async handler(html) {
        try {
          const response = await fetch(`${backendUrl}/api/v1/settings/public`, {
            signal: AbortSignal.timeout(2000),
          })
          if (!response.ok) {
            return html
          }
          const data = await response.json()
          if (data.code === 0 && data.data) {
            const script = `<script>window.__APP_CONFIG__=${JSON.stringify(data.data)};</script>`
            return html.replace('</head>', `${script}\n</head>`)
          }
        } catch (error) {
          console.warn(
            '[vite] failed to inject public settings in dev mode:',
            (error as Error).message
          )
        }
        return html
      },
    },
  }
}

function escapeHtml(value: string): string {
  return value
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;')
}

function normalizeSiteBaseUrl(frontendUrl?: string): string {
  const raw = String(frontendUrl ?? '').trim()
  if (!raw) return ''
  try {
    const parsed = new URL(raw)
    parsed.hash = ''
    parsed.search = ''
    return parsed.toString().replace(/\/+$/, '')
  } catch {
    return ''
  }
}

function resolveAbsoluteUrl(baseUrl: string, raw?: string, fallbackPath = ''): string {
  const value = String(raw ?? '').trim()
  if (value) {
    if (/^https?:\/\//i.test(value) || value.startsWith('data:image/')) {
      return value
    }
    if (baseUrl) {
      return `${baseUrl}${value.startsWith('/') ? value : `/${value}`}`
    }
    return value
  }
  if (!baseUrl || !fallbackPath) {
    return ''
  }
  return `${baseUrl}${fallbackPath.startsWith('/') ? fallbackPath : `/${fallbackPath}`}`
}

function normalizeText(value?: string): string {
  return String(value ?? '')
    .replace(/<[^>]+>/g, ' ')
    .replace(/\s+/g, ' ')
    .trim()
}

function buildCanonicalUrl(baseUrl: string, route: string): string {
  if (!baseUrl) return ''
  const path = route === '/' ? '/' : `/${route.replace(/^\/+/, '')}`
  return `${baseUrl}${path === '/' ? '/' : path}`
}

function buildJsonLd(type: 'WebSite' | 'Article' | 'WebPage', title: string, description: string, canonical: string, image: string) {
  const payload: Record<string, unknown> = {
    '@context': 'https://schema.org',
    '@type': type,
    description,
  }
  if (type === 'Article') {
    payload.headline = title
    payload.mainEntityOfPage = canonical
  } else if (type === 'WebSite') {
    payload.name = title
    payload.url = canonical
  } else {
    payload.name = title
    payload.url = canonical
  }
  if (image) {
    payload.image = image
  }
  return JSON.stringify(payload)
}

function buildSeoBlock(
  settingsPayload: PrerenderSettingsPayload | null | undefined,
  entry: PrerenderRouteEntry
): string {
  const data = settingsPayload?.data
  const baseUrl = normalizeSiteBaseUrl((data as Record<string, unknown> | undefined)?.frontend_url as string | undefined)
  const siteName = String((data as Record<string, unknown> | undefined)?.site_name ?? '').trim() || 'Sub2API'
  const defaultTitle = String((data as Record<string, unknown> | undefined)?.seo_default_title ?? '').trim()
  const homeTitle = String((data as Record<string, unknown> | undefined)?.seo_home_title ?? '').trim()
  const defaultDescription = normalizeText((data as Record<string, unknown> | undefined)?.seo_default_description as string | undefined)
  const homeDescription = normalizeText((data as Record<string, unknown> | undefined)?.seo_home_description as string | undefined)
  const defaultRobots = String((data as Record<string, unknown> | undefined)?.seo_default_robots ?? '').trim() || 'index, follow'
  const homeRobots = String((data as Record<string, unknown> | undefined)?.seo_home_robots ?? '').trim() || defaultRobots
  const defaultImage = resolveAbsoluteUrl(
    baseUrl,
    (data as Record<string, unknown> | undefined)?.seo_default_og_image as string | undefined,
    '/og/home.svg'
  )

  const canonical = buildCanonicalUrl(baseUrl, entry.route)
  let title = defaultTitle || `${siteName} - AI API Gateway`
  let description = defaultDescription || normalizeText((data as Record<string, unknown> | undefined)?.site_subtitle as string | undefined) || 'Sub2API is an AI API gateway platform.'
  let image = defaultImage
  let robots = defaultRobots
  let type: 'website' | 'article' = 'website'
  let jsonLdType: 'WebSite' | 'Article' | 'WebPage' = 'WebPage'

  if (entry.route === '/home') {
    title = homeTitle || defaultTitle || `${siteName} - AI API Gateway`
    description = homeDescription || defaultDescription || description
    image = defaultImage
    robots = homeRobots
    type = 'website'
    jsonLdType = 'WebSite'
  } else if (entry.route.startsWith('/legal/')) {
    title = entry.seoTitle?.trim() || `${entry.title} - ${siteName}`
    description = normalizeText(entry.seoDescription) || `Read ${entry.title} on ${siteName}.`
    image = resolveAbsoluteUrl(baseUrl, entry.seoOGImage, `/og/legal-${entry.route.replace('/legal/', '')}.svg`)
    robots = entry.seoRobots?.trim() || defaultRobots
    type = 'article'
    jsonLdType = 'Article'
  } else if (entry.route.startsWith('/custom/')) {
    title = entry.seoTitle?.trim() || `${entry.title} - ${siteName}`
    description = normalizeText(entry.seoDescription) || `Learn more about ${entry.title} on ${siteName}.`
    image = resolveAbsoluteUrl(baseUrl, entry.seoOGImage, `/og/custom-${entry.route.replace('/custom/', '')}.svg`)
    robots = entry.seoRobots?.trim() || defaultRobots
    type = entry.markdown ? 'article' : 'website'
    jsonLdType = entry.markdown ? 'Article' : 'WebPage'
  }

  const tags = [
    `<title>${escapeHtml(title)}</title>`,
    `<meta name="description" content="${escapeHtml(description)}">`,
    `<meta name="robots" content="${escapeHtml(robots)}">`,
    canonical ? `<link rel="canonical" href="${escapeHtml(canonical)}">` : '',
    `<meta property="og:type" content="${type}">`,
    `<meta property="og:title" content="${escapeHtml(title)}">`,
    `<meta property="og:description" content="${escapeHtml(description)}">`,
    canonical ? `<meta property="og:url" content="${escapeHtml(canonical)}">` : '',
    `<meta property="og:site_name" content="${escapeHtml(siteName)}">`,
    image ? `<meta property="og:image" content="${escapeHtml(image)}">` : '',
    `<meta name="twitter:card" content="summary_large_image">`,
    `<meta name="twitter:title" content="${escapeHtml(title)}">`,
    `<meta name="twitter:description" content="${escapeHtml(description)}">`,
    image ? `<meta name="twitter:image" content="${escapeHtml(image)}">` : '',
    `<script type="application/ld+json">${buildJsonLd(jsonLdType, entry.title || siteName, description, canonical, image)}</script>`,
  ].filter(Boolean)

  return tags.join('')
}

function replaceHeadSeo(indexHtml: string, seoBlock: string): string {
  let html = indexHtml.replace(/<title>[\s\S]*?<\/title>/i, '')
  html = html.replace(/<meta[^>]+name="description"[^>]*>/gi, '')
  html = html.replace(/<meta[^>]+name="robots"[^>]*>/gi, '')
  html = html.replace(/<meta[^>]+property="og:[^"]+"[^>]*>/gi, '')
  html = html.replace(/<meta[^>]+name="twitter:[^"]+"[^>]*>/gi, '')
  html = html.replace(/<link[^>]+rel="canonical"[^>]*>/gi, '')
  html = html.replace(/<script type="application\/ld\+json">[\s\S]*?<\/script>/gi, '')
  return html.replace('</head>', `${seoBlock}</head>`)
}

function staticPrerenderRoutes(settingsURL: string, publicPagesURL: string): Plugin {
  return {
    name: 'static-prerender-routes',
    apply: 'build',
    async closeBundle() {
      const outDir = resolve(__dirname, '../backend/internal/web/dist')
      const indexHtml = readFileSync(join(outDir, 'index.html'), 'utf8')
      let settingsPayload: PrerenderSettingsPayload | null = null
      let routes: PrerenderRouteEntry[] = BASE_PUBLIC_PRERENDER_ROUTES.map((route) => ({
        route,
        title: '',
        source: 'base',
      }))

      try {
        const response = await fetch(settingsURL, {
          signal: AbortSignal.timeout(3000),
          headers: { accept: 'application/json' },
        })
        if (response.ok) {
          settingsPayload = (await response.json()) as PrerenderSettingsPayload
          routes = collectPrerenderRoutes(settingsPayload)
          for (const entry of routes) {
            if (entry.markdown || !entry.markdownSlug) continue
            const markdownResponse = await fetch(
              `${publicPagesURL}/${encodeURIComponent(entry.markdownSlug)}`,
              {
                signal: AbortSignal.timeout(3000),
                headers: { accept: 'text/markdown' },
              }
            )
            if (markdownResponse.ok) {
              entry.markdown = await markdownResponse.text()
            }
          }
        }
      } catch (error) {
        console.warn(
          '[vite] failed to enumerate public prerender routes from settings API, only /home will be prerendered:',
          (error as Error).message
        )
      }

      const legalDocs = settingsPayload?.data?.login_agreement_documents ?? []
      const customPages = settingsPayload?.data?.custom_menu_items ?? []
      const legalById = new Map(
        legalDocs.map((item) => [
          String(item.id ?? '').trim(),
          item,
        ])
      )
      const customById = new Map(
        customPages.map((item) => [
          String(item.id ?? '').trim(),
          item,
        ])
      )

      for (const entry of routes) {
        if (entry.route.startsWith('/legal/')) {
          const doc = legalById.get(entry.route.replace('/legal/', ''))
          entry.title = entry.title || String((doc as Record<string, unknown> | undefined)?.title ?? '').trim()
          entry.markdown = entry.markdown || String((doc as Record<string, unknown> | undefined)?.content_md ?? '')
          entry.seoTitle = String((doc as Record<string, unknown> | undefined)?.seo_title ?? '').trim()
          entry.seoDescription = String((doc as Record<string, unknown> | undefined)?.seo_description ?? '').trim()
          entry.seoOGImage = String((doc as Record<string, unknown> | undefined)?.seo_og_image ?? '').trim()
          entry.seoRobots = String((doc as Record<string, unknown> | undefined)?.seo_robots ?? '').trim()
        }
        if (entry.route.startsWith('/custom/')) {
          const item = customById.get(entry.route.replace('/custom/', ''))
          entry.title = entry.title || String((item as Record<string, unknown> | undefined)?.label ?? '').trim()
          entry.seoTitle = String((item as Record<string, unknown> | undefined)?.seo_title ?? '').trim()
          entry.seoDescription = String((item as Record<string, unknown> | undefined)?.seo_description ?? '').trim()
          entry.seoOGImage = String((item as Record<string, unknown> | undefined)?.seo_og_image ?? '').trim()
          entry.seoRobots = String((item as Record<string, unknown> | undefined)?.seo_robots ?? '').trim()
        }

        const cleanRoute = entry.route.replace(/^\/+/, '')
        const targetDir = join(outDir, cleanRoute)
        mkdirSync(targetDir, { recursive: true })
        const withBody = injectPrerenderContent(indexHtml, entry)
        const withSeo = replaceHeadSeo(withBody, buildSeoBlock(settingsPayload, entry))
        writeFileSync(join(targetDir, 'index.html'), withSeo, 'utf8')
      }

      writeFileSync(
        join(outDir, 'prerender-manifest.json'),
        JSON.stringify(buildPrerenderManifest(routes), null, 2),
        'utf8'
      )
    },
  }
}

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd(), '')
  const backendUrl = env.VITE_DEV_PROXY_TARGET || 'http://localhost:8080'
  const devPort = Number(env.VITE_DEV_PORT || 3000)
  const prerenderSettingsURL =
    env.VITE_PRERENDER_SETTINGS_URL || `${backendUrl.replace(/\/+$/, '')}/api/v1/settings/public`
  const prerenderPublicPagesURL =
    env.VITE_PRERENDER_PUBLIC_PAGES_URL || `${backendUrl.replace(/\/+$/, '')}/api/v1/public/pages`

  return {
    plugins: [
      vue(),
      checker({
        vueTsc: true,
      }),
      injectPublicSettings(backendUrl),
      staticPrerenderRoutes(prerenderSettingsURL, prerenderPublicPagesURL),
    ],
    resolve: {
      alias: {
        '@': resolve(__dirname, 'src'),
        'vue-i18n': 'vue-i18n/dist/vue-i18n.runtime.esm-bundler.js',
      },
    },
    define: {
      __INTLIFY_JIT_COMPILATION__: true,
    },
    build: {
      outDir: '../backend/internal/web/dist',
      emptyOutDir: true,
      rollupOptions: {
        output: {
          manualChunks(id: string) {
            if (id.includes('node_modules')) {
              if (
                id.includes('/vue/') ||
                id.includes('/vue-router/') ||
                id.includes('/pinia/') ||
                id.includes('/@vue/')
              ) {
                return 'vendor-vue'
              }
              if (id.includes('/@vueuse/') || id.includes('/xlsx/')) {
                return 'vendor-ui'
              }
              if (id.includes('/chart.js/') || id.includes('/vue-chartjs/')) {
                return 'vendor-chart'
              }
              if (id.includes('/vue-i18n/') || id.includes('/@intlify/')) {
                return 'vendor-i18n'
              }
              return 'vendor-misc'
            }
            return undefined
          },
        },
      },
    },
    server: {
      host: '0.0.0.0',
      port: devPort,
      proxy: {
        '/api': {
          target: backendUrl,
          changeOrigin: true,
        },
        '/v1': {
          target: backendUrl,
          changeOrigin: true,
        },
        '/setup': {
          target: backendUrl,
          changeOrigin: true,
        },
      },
    },
  }
})
