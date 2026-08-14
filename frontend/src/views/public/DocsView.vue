<template>
  <div data-testid="docs-view" class="flex min-h-screen flex-col bg-canvas text-ink">
    <header class="border-b border-line">
      <div class="mx-auto flex max-w-6xl items-center justify-between gap-4 px-6 py-3">
        <RouterLink to="/home" class="flex min-w-0 items-center gap-3 rounded">
          <template v-if="settings">
            <img
              :src="siteLogo || '/logo.svg'"
              alt=""
              class="h-7 w-7 shrink-0 rounded object-contain"
            />
            <span class="min-w-0 truncate text-sm font-semibold [overflow-wrap:anywhere]">
              {{ siteName }}
            </span>
          </template>
          <template v-else>
            <span class="skeleton h-7 w-7 shrink-0 rounded" aria-hidden="true"></span>
            <span class="skeleton h-4 w-28" aria-hidden="true"></span>
          </template>
        </RouterLink>

        <Button to="/login" tone="accent" variant="solid" size="md" class="shrink-0">
          {{ t('home.login') }}
        </Button>
      </div>
    </header>

    <main class="flex-1">
      <div
        class="mx-auto max-w-6xl gap-16 px-6 py-12 lg:grid lg:grid-cols-[12rem_minmax(0,1fr)] lg:py-16"
      >
        <!--
          The page list is navigation, not a table of contents, so it ships on
          every page and at every width. On small screens it collapses to a
          single labelled row above the article rather than a drawer: six links
          are cheaper to render than a disclosure nobody opens.
        -->
        <nav
          data-testid="docs-nav"
          :aria-label="t('docs.sectionsLabel')"
          class="mb-10 border-b border-line pb-8 lg:mb-0 lg:border-b-0 lg:pb-0"
        >
          <div class="lg:sticky lg:top-10">
            <p class="text-2xs uppercase tracking-[0.04em] text-ink-tertiary">
              {{ t('docs.title') }}
            </p>
            <ul class="mt-4 space-y-px">
              <li v-for="item in navigation" :key="item.slug">
                <RouterLink
                  :to="`/docs/${item.slug}`"
                  :data-testid="`docs-nav-${item.slug}`"
                  :aria-current="item.slug === activeSlug ? 'page' : undefined"
                  class="block py-1.5 text-sm transition-colors duration-fast"
                  :class="
                    item.slug === activeSlug
                      ? 'font-medium text-ink'
                      : 'text-ink-tertiary hover:text-ink'
                  "
                >
                  {{ t(item.titleKey) }}
                </RouterLink>
              </li>
            </ul>
          </div>
        </nav>

        <div class="min-w-0">
          <!-- Bars on the reading measure show the shape of what is arriving. -->
          <div v-if="loading" data-testid="docs-loading" class="max-w-[68ch] space-y-4">
            <span class="skeleton block h-3 w-24" aria-hidden="true"></span>
            <span class="skeleton block h-8 w-2/3" aria-hidden="true"></span>
            <span class="skeleton block h-4 w-full" aria-hidden="true"></span>
            <span class="skeleton block h-4 w-11/12" aria-hidden="true"></span>
            <span class="skeleton block h-4 w-4/5" aria-hidden="true"></span>
            <span class="sr-only">{{ t('common.loading') }}</span>
          </div>

          <section
            v-else-if="!page"
            data-testid="docs-not-found"
            class="max-w-[68ch] border border-line bg-surface p-8"
          >
            <h1 class="text-lg font-semibold text-ink">{{ t('docs.notFound') }}</h1>
            <p class="mt-2 text-sm leading-6 text-ink-secondary">
              {{ t('docs.notFoundDescription') }}
            </p>
          </section>

          <!-- `danger` is a status token, so it may carry a status. The accent may not. -->
          <section
            v-else-if="loadError"
            data-testid="docs-load-error"
            class="max-w-[68ch] border border-line bg-surface p-8"
          >
            <p class="font-mono text-2xs uppercase tracking-[0.04em] text-danger">
              {{ t('common.error') }}
            </p>
            <h1 class="mt-3 text-lg font-semibold text-ink">{{ t('docs.loadFailed') }}</h1>
            <p class="mt-2 text-sm text-ink-secondary">{{ t('docs.retryLater') }}</p>
          </section>

          <div
            v-else
            data-testid="docs-document"
            :class="showTableOfContents ? 'xl:grid xl:grid-cols-[minmax(0,1fr)_12rem] xl:gap-12' : ''"
          >
            <article class="min-w-0 max-w-[68ch]">
              <header class="border-b border-line pb-8">
                <p class="text-2xs uppercase tracking-[0.04em] text-ink-tertiary">
                  {{ t('docs.title') }}
                </p>
                <h1 class="mt-4 text-3xl font-semibold [overflow-wrap:anywhere]">
                  {{ t(page.titleKey) }}
                </h1>
                <p class="mt-4 max-w-[60ch] text-sm leading-6 text-ink-secondary">
                  {{ t(page.summaryKey) }}
                </p>
              </header>

              <!--
                `v-html` over markdown that `renderedDocument` has already run
                through DOMPurify. The click handler exists so an in-document
                `/docs/...` link routes instead of reloading the application.
              -->
              <div
                data-testid="docs-prose"
                class="long-form-prose mt-10"
                @click="onProseClick"
                v-html="renderedHtml"
              ></div>

              <footer
                v-if="nextPage"
                class="mt-16 border-t border-line pt-8"
              >
                <p class="text-2xs uppercase tracking-[0.04em] text-ink-tertiary">
                  {{ t('docs.next') }}
                </p>
                <RouterLink
                  :to="`/docs/${nextPage.slug}`"
                  data-testid="docs-next"
                  class="mt-3 block text-md font-medium text-accent underline-offset-2 transition-colors duration-fast hover:text-accent-hover hover:underline"
                >
                  {{ t(nextPage.titleKey) }}
                </RouterLink>
              </footer>
            </article>

            <!--
              Earned, not decorative: a page has to reach TOC_MIN_HEADINGS
              sections before this renders, and it only appears at `xl`, where
              the page list already has its own column.
            -->
            <nav
              v-if="showTableOfContents"
              data-testid="docs-toc"
              :aria-label="t('docs.tableOfContents')"
              class="hidden xl:block"
            >
              <div class="sticky top-10">
                <p class="text-2xs uppercase tracking-[0.04em] text-ink-tertiary">
                  {{ t('docs.tableOfContents') }}
                </p>
                <ul class="mt-4 border-l border-line-subtle">
                  <li v-for="item in tableOfContents" :key="item.id">
                    <a
                      :href="`#${item.id}`"
                      class="-ml-px block border-l border-transparent py-1.5 text-xs text-ink-tertiary transition-colors duration-fast hover:border-line-strong hover:text-ink"
                      :class="item.level === 3 ? 'pl-6' : 'pl-4'"
                    >
                      {{ item.text }}
                    </a>
                  </li>
                </ul>
              </div>
            </nav>
          </div>
        </div>
      </div>
    </main>
  </div>
</template>

<script setup lang="ts">
import DOMPurify from 'dompurify'
import { marked } from 'marked'
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'

import Button from '@/components/common/Button.vue'
import { getLocale } from '@/i18n'
import { useAppStore } from '@/stores/app'
import { sanitizeUrl } from '@/utils/url'

import '@/styles/long-form-prose.css'
import { DEFAULT_DOCS_SLUG, DOCS_PAGES, findDocsPage, type DocsPage } from './docsPages'

interface TocEntry {
  id: string
  text: string
  level: 2 | 3
}

/** Below this, a table of contents is chrome rather than navigation. */
const TOC_MIN_HEADINGS = 4

/**
 * Markdown in `docs/public` is written against one deployment-agnostic token
 * so every `curl` on the page is copy-pasteable on whatever host is serving
 * it, rather than against a hardcoded domain that is wrong everywhere else.
 */
const SITE_ORIGIN_TOKEN = /\{\{SITE_ORIGIN\}\}/g

const route = useRoute()
const router = useRouter()
const { t, locale } = useI18n()
const appStore = useAppStore()

const settings = computed(() => appStore.cachedPublicSettings)
const siteName = computed(() => settings.value?.site_name || 'Sub2API')
const siteLogo = computed(() =>
  sanitizeUrl(settings.value?.site_logo || '', {
    allowRelative: true,
    allowDataUrl: true,
  })
)

const navigation = DOCS_PAGES
const activeSlug = computed(() => String(route.params.slug || DEFAULT_DOCS_SLUG))
const page = computed<DocsPage | null>(() => findDocsPage(activeSlug.value))
const nextPage = computed<DocsPage | null>(() => {
  const index = DOCS_PAGES.findIndex((item) => item.slug === activeSlug.value)
  if (index < 0) {
    return null
  }
  return DOCS_PAGES[index + 1] ?? null
})

const loading = ref(true)
const loadError = ref(false)
const source = ref('')

marked.setOptions({
  breaks: false,
  gfm: true,
})

/**
 * Render, sanitize, then annotate — in that order. The annotation pass only
 * ever writes an `id` onto a heading DOMPurify has already cleared, so the
 * markdown pipeline itself is untouched and nothing re-enters the document
 * that sanitization did not approve.
 */
const renderedDocument = computed<{ html: string; toc: TocEntry[] }>(() => {
  const content = source.value.trim()
  if (!content) {
    return { html: '', toc: [] }
  }

  const origin = typeof window === 'undefined' ? '' : window.location.origin
  const safe = DOMPurify.sanitize(
    marked.parse(content.replace(SITE_ORIGIN_TOKEN, origin)) as string
  )
  const host = document.createElement('div')
  host.innerHTML = safe

  const toc: TocEntry[] = []
  host.querySelectorAll('h2, h3').forEach((heading, index) => {
    const text = heading.textContent?.trim() ?? ''
    if (!text) {
      return
    }
    // Positional ids rather than slugs: these pages also ship in Chinese, and a
    // CJK slug has to be percent-encoded at every `href` pointing at it.
    const id = `docs-section-${index + 1}`
    heading.id = id
    toc.push({ id, text, level: heading.tagName === 'H2' ? 2 : 3 })
  })

  return { html: host.innerHTML, toc }
})

const renderedHtml = computed(() => renderedDocument.value.html)
const tableOfContents = computed(() => renderedDocument.value.toc)
const showTableOfContents = computed(() => tableOfContents.value.length >= TOC_MIN_HEADINGS)

/**
 * Slug and locale can both change while a fetch is in flight, and the two
 * changes arrive together on a language switch. The token is what stops a
 * slow first response from overwriting the page the reader is now on.
 */
let loadToken = 0

async function loadPage() {
  const token = ++loadToken
  const target = page.value
  loadError.value = false

  if (!target) {
    source.value = ''
    loading.value = false
    return
  }

  loading.value = true
  try {
    const localeCode = getLocale()
    const loader = target.load[localeCode] ?? target.load.en
    const module = await loader()
    if (token !== loadToken) {
      return
    }
    source.value = module.default
  } catch {
    if (token !== loadToken) {
      return
    }
    source.value = ''
    loadError.value = true
  } finally {
    if (token === loadToken) {
      loading.value = false
    }
  }
}

/**
 * Markdown cross-links are plain `/docs/...` hrefs, which is what they have to
 * be for the file to read correctly outside the application. Routing them here
 * keeps the navigation client-side instead of reloading the bundle.
 */
function onProseClick(event: MouseEvent) {
  if (
    event.defaultPrevented ||
    event.button !== 0 ||
    event.metaKey ||
    event.ctrlKey ||
    event.shiftKey ||
    event.altKey
  ) {
    return
  }
  const target = event.target
  if (!(target instanceof Element)) {
    return
  }
  const anchor = target.closest('a')
  if (!anchor) {
    return
  }
  const href = anchor.getAttribute('href') ?? ''
  if (!href.startsWith('/docs')) {
    return
  }
  event.preventDefault()
  void router.push(href)
}

watch([activeSlug, locale], () => {
  void loadPage()
  if (typeof window !== 'undefined') {
    window.scrollTo({ top: 0 })
  }
})

onMounted(async () => {
  // Public settings only feed the header's name and logo, so the document is
  // fetched alongside them rather than behind them.
  void appStore.fetchPublicSettings()
  await loadPage()
})
</script>
