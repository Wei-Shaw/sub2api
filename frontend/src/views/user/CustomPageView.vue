<template>
  <AppLayout>
    <div class="custom-page-layout">
      <div class="card flex-1 min-h-0 overflow-hidden">
        <!--
          HOST CHROME ONLY. Everything below styles the frame around the
          embedded page: the loading state, the two "nothing to show" states,
          the table of contents, and the open-in-new-tab control. What the
          iframe renders, and the sanitised markdown pipeline in the script,
          are not this pass's business — `DOMPurify`, the relative-asset
          rewriter and the `http(s)`-only `isValidUrl` gate are a security
          surface, not a visual one, and are untouched.
        -->
        <div v-if="loading" class="h-full space-y-3 p-6">
          <div class="skeleton h-3 w-40"></div>
          <div class="skeleton h-3 w-full"></div>
          <div class="skeleton h-3 w-4/5"></div>
        </div>

        <div
          v-else-if="!menuItem"
          class="flex h-full items-center justify-center p-10 text-center"
        >
          <div class="max-w-md">
            <Icon name="link" size="lg" class="mx-auto mb-3 text-ink-disabled" />
            <h3 class="text-sm font-medium text-ink">
              {{ t('customPage.notFoundTitle') }}
            </h3>
            <p class="mt-1 text-xs text-ink-tertiary">
              {{ t('customPage.notFoundDesc') }}
            </p>
          </div>
        </div>

        <!-- Markdown mode with TOC -->
        <div v-else-if="isMarkdownMode" class="flex h-full overflow-hidden">
          <!-- TOC Sidebar -->
          <aside
            v-show="tocVisible"
            class="toc-sidebar"
          >
            <div class="toc-header">
              <span class="toc-title">{{ t('customPage.tableOfContents') }}</span>
              <!-- Icon-only, so it needs an accessible name of its own. -->
              <button
                type="button"
                class="toc-close-btn"
                :aria-label="t('common.close')"
                :title="t('common.close')"
                @click="tocVisible = false"
              >
                <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M15 18l-6-6 6-6"/></svg>
              </button>
            </div>
            <nav class="toc-nav">
              <a
                v-for="item in tocItems"
                :key="item.id"
                :href="'#' + item.id"
                class="toc-item"
                :class="[
                  `toc-level-${item.level}`,
                  { 'toc-active': activeHeadingId === item.id }
                ]"
                @click.prevent="scrollToHeading(item.id)"
              >
                {{ item.text }}
              </a>
            </nav>
          </aside>

          <!-- TOC Toggle Button (when collapsed) -->
          <button
            v-show="!tocVisible && tocItems.length > 0"
            type="button"
            class="toc-toggle-btn"
            @click="tocVisible = true"
          >
            <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M3 12h18M3 6h18M3 18h18"/></svg>
            <span class="ml-1.5">{{ t('customPage.tableOfContents') }}</span>
          </button>

          <!-- Content -->
          <div
            ref="markdownContainer"
            class="markdown-page-content flex-1 h-full overflow-auto p-6 md:p-10"
            v-html="renderedHtml"
            @scroll="onContentScroll"
          ></div>
        </div>

        <!-- URL not configured -->
        <div v-else-if="!isValidUrl" class="flex h-full items-center justify-center p-10 text-center">
          <div class="max-w-md">
            <Icon name="link" size="lg" class="mx-auto mb-3 text-ink-disabled" />
            <h3 class="text-sm font-medium text-ink">
              {{ t('customPage.notConfiguredTitle') }}
            </h3>
            <p class="mt-1 text-xs text-ink-tertiary">
              {{ t('customPage.notConfiguredDesc') }}
            </p>
          </div>
        </div>

        <!-- Iframe embed mode -->
        <div v-else class="custom-embed-shell">
          <!--
            The sweep pass deleted this control's `backdrop-blur`, which left it
            translucent over whatever the embedded page happened to render
            underneath. It sits on an opaque surface with a hairline now — a
            floating control has to be readable against content it does not own.
          -->
          <a
            :href="embeddedUrl"
            target="_blank"
            rel="noopener noreferrer"
            class="btn btn-secondary btn-sm custom-open-fab"
          >
            <Icon name="externalLink" size="xs" class="mr-1.5" :stroke-width="2" />
            {{ t('customPage.openInNewTab') }}
          </a>
          <iframe
            :src="embeddedUrl"
            class="custom-embed-frame"
            allowfullscreen
          ></iframe>
        </div>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores'
import { useAuthStore } from '@/stores/auth'
import { useAdminSettingsStore } from '@/stores/adminSettings'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import { buildApiUrl } from '@/api/client'
import { buildEmbeddedUrl, detectTheme } from '@/utils/embedded-url'
import { marked } from 'marked'
import DOMPurify from 'dompurify'

interface TocItem {
  id: string
  text: string
  level: number
}

const { t, locale } = useI18n()
const route = useRoute()
const appStore = useAppStore()
const authStore = useAuthStore()
const adminSettingsStore = useAdminSettingsStore()

const loading = ref(false)
const pageTheme = ref<'light' | 'dark'>('light')
const renderedHtml = ref('')
const markdownContainer = ref<HTMLElement | null>(null)
const tocItems = ref<TocItem[]>([])
const tocVisible = ref(typeof window !== 'undefined' ? window.innerWidth > 768 : true)
const activeHeadingId = ref('')
let themeObserver: MutationObserver | null = null

const menuItemId = computed(() => route.params.id as string)

const menuItem = computed(() => {
  const id = menuItemId.value
  const publicItems = appStore.cachedPublicSettings?.custom_menu_items ?? []
  const found = publicItems.find((item) => item.id === id) ?? null
  if (found) return found
  if (authStore.isAdmin) {
    return adminSettingsStore.customMenuItems.find((item) => item.id === id) ?? null
  }
  return null
})

const markdownSlug = computed(() => {
  const item = menuItem.value
  if (!item) return ''
  if (item.page_slug) return item.page_slug
  if (item.url?.startsWith('md:')) return item.url.slice(3)
  return ''
})

const isMarkdownMode = computed(() => !!markdownSlug.value)

const embeddedUrl = computed(() => {
  if (!menuItem.value || isMarkdownMode.value) return ''
  return buildEmbeddedUrl(
    menuItem.value.url,
    authStore.user?.id,
    authStore.token,
    pageTheme.value,
    locale.value,
  )
})

const isValidUrl = computed(() => {
  if (isMarkdownMode.value) return false
  const url = embeddedUrl.value
  return url.startsWith('http://') || url.startsWith('https://')
})

function generateHeadingId(text: string, index: number): string {
  const base = text
    .toLowerCase()
    .replace(/[^\w一-鿿]+/g, '-')
    .replace(/^-+|-+$/g, '')
  return base ? `${base}-${index}` : `heading-${index}`
}

function isRelativeMarkdownAsset(src: string): boolean {
  const trimmed = src.trim()
  if (!trimmed || /^[a-z][a-z0-9+.-]*:/i.test(trimmed) || trimmed.startsWith('//') || trimmed.startsWith('/')) {
    return false
  }
  const [pathPart] = trimmed.split(/([?#].*)/, 2)
  return pathPart
    .split('/')
    .filter((part) => part && part !== '.')
    .every((part) => part !== '..' && !part.includes('\\'))
}

function buildPageImageUrl(slug: string, src: string): string {
  const trimmed = src.trim()
  const [pathPart, suffix = ''] = trimmed.split(/([?#].*)/, 2)
  const encodedPath = pathPart
    .split('/')
    .filter((part) => part && part !== '.')
    .map((part) => encodeURIComponent(part))
    .join('/')
  return buildApiUrl(`/pages/${encodeURIComponent(slug)}/images/${encodedPath}${suffix}`)
}

async function fetchAndRenderMarkdown(slug: string) {
  loading.value = true
  tocItems.value = []
  activeHeadingId.value = ''
  try {
    const resp = await fetch(buildApiUrl(`/pages/${encodeURIComponent(slug)}`), {
      headers: authStore.token ? { Authorization: `Bearer ${authStore.token}` } : {},
    })
    if (!resp.ok) {
      renderedHtml.value = `<p class="text-danger">${t('common.pageNotFound')}</p>`
      return
    }
    let raw = await resp.text()

    raw = raw.replace(
      /!\[([^\]]*)\]\(([^)]+)\)/g,
      (match, alt, src) => isRelativeMarkdownAsset(src) ? `![${alt}](${buildPageImageUrl(slug, src)})` : match
    )

    const html = marked.parse(raw) as string
    const sanitized = DOMPurify.sanitize(html, {
      ADD_TAGS: ['iframe'],
      ADD_ATTR: ['allowfullscreen', 'frameborder', 'src'],
    })

    // Inject IDs into headings and build TOC
    const toc: TocItem[] = []
    let headingIndex = 0
    const withIds = sanitized.replace(
      /<(h[1-4])[^>]*>(.*?)<\/h[1-4]>/gi,
      (_, tag: string, content: string) => {
        const level = parseInt(tag[1])
        const text = content.replace(/<[^>]+>/g, '').trim()
        const id = generateHeadingId(text, headingIndex++)
        toc.push({ id, text, level })
        return `<${tag} id="${id}">${content}</${tag}>`
      }
    )

    renderedHtml.value = withIds
    tocItems.value = toc
  } catch {
    renderedHtml.value = `<p class="text-danger">${t('common.error')}</p>`
  } finally {
    loading.value = false
    await nextTick()
    await nextTick()
    injectCopyButtons()
  }
}

function scrollToHeading(id: string) {
  const container = markdownContainer.value
  if (!container) return
  const el = container.querySelector(`#${CSS.escape(id)}`)
  if (el) {
    el.scrollIntoView({ behavior: 'smooth', block: 'start' })
    activeHeadingId.value = id
    if (window.innerWidth <= 640) {
      tocVisible.value = false
    }
  }
}

let scrollRafId = 0
function onContentScroll() {
  if (scrollRafId) return
  scrollRafId = requestAnimationFrame(() => {
    scrollRafId = 0
    const container = markdownContainer.value
    if (!container || tocItems.value.length === 0) return

    const containerRect = container.getBoundingClientRect()
    let current = ''

    for (const item of tocItems.value) {
      const el = container.querySelector(`#${CSS.escape(item.id)}`) as HTMLElement | null
      if (el) {
        const elRect = el.getBoundingClientRect()
        if (elRect.top - containerRect.top <= 100) {
          current = item.id
        }
      }
    }
    activeHeadingId.value = current
  })
}

function injectCopyButtons() {
  const container = markdownContainer.value
  if (!container) return

  container.querySelectorAll('pre').forEach((pre) => {
    if (pre.querySelector('.copy-btn')) return
    const btn = document.createElement('button')
    btn.className = 'copy-btn'
    btn.textContent = t('customPage.copyCode')
    btn.addEventListener('click', async () => {
      const code = pre.querySelector('code')?.textContent ?? pre.textContent ?? ''
      try {
        await navigator.clipboard.writeText(code)
        btn.textContent = t('customPage.copiedCode')
        setTimeout(() => { btn.textContent = t('customPage.copyCode') }, 2000)
      } catch {
        btn.textContent = t('customPage.copyCodeFailed')
        setTimeout(() => { btn.textContent = t('customPage.copyCode') }, 2000)
      }
    })
    pre.style.position = 'relative'
    pre.appendChild(btn)
  })
}

watch(markdownSlug, (slug) => {
  if (slug) {
    fetchAndRenderMarkdown(slug)
  } else {
    renderedHtml.value = ''
    tocItems.value = []
  }
}, { immediate: true })

onMounted(async () => {
  pageTheme.value = detectTheme()

  if (typeof document !== 'undefined') {
    themeObserver = new MutationObserver(() => {
      pageTheme.value = detectTheme()
    })
    themeObserver.observe(document.documentElement, {
      attributes: true,
      attributeFilter: ['class'],
    })
  }

  if (appStore.publicSettingsLoaded) return
  loading.value = true
  try {
    await appStore.fetchPublicSettings()
  } finally {
    loading.value = false
  }
})

onUnmounted(() => {
  if (themeObserver) {
    themeObserver.disconnect()
    themeObserver = null
  }
})
</script>

<style scoped>
/* Header height and page padding were magic numbers; they are tokens now, so
   changing either does not silently mis-size this page by the difference. */
.custom-page-layout {
  @apply flex flex-col;
  height: calc(100vh - var(--ds-app-header-h) - (2 * var(--ds-page-pad)));
}

/*
 * Every rule below was a hardcoded `gray-*` / `dark-*` pair. They are Family B
 * tokens now, which flip on their own — so none of them carries a `dark:`
 * variant, and re-adding one would double-apply.
 */
.toc-sidebar {
  @apply flex h-full flex-col border-r border-line bg-surface-sunken;
  width: min(240px, 30%);
  min-width: 160px;
  max-width: 280px;
  overflow: hidden;
}

@media (max-width: 640px) {
  .toc-sidebar {
    position: absolute;
    left: 0;
    top: 0;
    z-index: 20;
    width: 70%;
    max-width: 240px;
    height: 100%;
    /* A drawer genuinely floats over the content, so it keeps an elevation —
       the popover token, not an ad-hoc rgba blur. */
    box-shadow: var(--ds-shadow-popover);
  }
}

.toc-header {
  @apply flex items-center justify-between gap-2 border-b border-line px-3 py-2;
}

.toc-title {
  @apply text-2xs font-medium uppercase text-ink-tertiary;
  letter-spacing: var(--ds-tr-2xs);
}

.toc-close-btn {
  @apply rounded p-1 text-ink-tertiary transition-colors duration-fast hover:bg-surface-hover hover:text-ink;
}

.toc-nav {
  @apply flex-1 overflow-y-auto px-2 py-2;
}

.toc-item {
  @apply block truncate rounded px-2 py-1 text-xs text-ink-secondary;
  @apply transition-colors duration-fast hover:bg-surface-hover hover:text-ink;
}

/* Selection — the one thing on this page the accent is allowed to mark. */
.toc-item.toc-active {
  @apply bg-accent-tint font-medium text-accent;
  box-shadow: inset 2px 0 0 0 rgb(var(--ds-accent));
}

.toc-level-1 { padding-left: 8px; }
.toc-level-2 { padding-left: 20px; }
.toc-level-3 { padding-left: 32px; }
.toc-level-4 { padding-left: 44px; }

.toc-toggle-btn {
  @apply absolute left-2 top-2 z-10 flex cursor-pointer items-center rounded px-2 text-xs font-medium;
  @apply h-7 border border-line bg-surface text-ink-secondary;
  @apply transition-colors duration-fast hover:border-line-strong hover:bg-surface-hover hover:text-ink;
}

/* Flat host frame. Was a 16px-radius well with a vertical gradient. */
.custom-embed-shell {
  @apply relative h-full w-full overflow-hidden bg-surface p-0;
}

.custom-open-fab {
  @apply absolute right-3 top-3 z-10;
}

.custom-embed-frame {
  display: block;
  margin: 0;
  width: 100%;
  height: 100%;
  border: 0;
  border-radius: 0;
  box-shadow: none;
  background: transparent;
}
</style>

<style>
.markdown-page-content {
  line-height: 1.7;
  color: inherit;
}
/*
 * Typography for the rendered markdown. The document body is authored
 * elsewhere; this is the house style applied to it, and like the chrome above
 * it now runs entirely on Family B tokens with no `dark:` pairs.
 */
.markdown-page-content h1 { @apply mb-4 mt-8 border-b border-line pb-2 text-2xl font-semibold text-ink; }
.markdown-page-content h2 { @apply mb-3 mt-6 text-xl font-semibold text-ink; }
.markdown-page-content h3 { @apply mb-2 mt-5 text-lg font-semibold text-ink; }
.markdown-page-content h4 { @apply mb-2 mt-4 text-md font-semibold text-ink; }
.markdown-page-content p { @apply mb-4 text-ink-secondary; }
.markdown-page-content ul { @apply mb-4 list-disc pl-6 text-ink-secondary; }
.markdown-page-content ol { @apply mb-4 list-decimal pl-6 text-ink-secondary; }
.markdown-page-content li { @apply mb-1; }
.markdown-page-content a { @apply text-accent underline underline-offset-2 hover:text-accent-hover; }
.markdown-page-content blockquote { @apply my-4 border-l-2 border-line-strong pl-4 text-ink-secondary; }
.markdown-page-content img { @apply my-4 h-auto max-w-full rounded; }
.markdown-page-content table { @apply my-4 w-full border-collapse text-sm; }
.markdown-page-content th { @apply border-b border-line-strong bg-surface-sunken px-3 py-2 text-left font-medium text-ink; }
.markdown-page-content td { @apply border-b border-line-subtle px-3 py-2 text-ink-secondary; }
.markdown-page-content code { @apply rounded-sm border border-line bg-surface-sunken px-1 py-0.5 text-xs text-ink; font-family: var(--ds-font-mono); }
.markdown-page-content pre { @apply relative my-4 overflow-x-auto rounded border border-line bg-surface-sunken p-3 text-xs text-ink; font-family: var(--ds-font-mono); }
.markdown-page-content pre code { @apply border-0 bg-transparent p-0 text-inherit; }
.markdown-page-content hr { @apply my-6 border-line; }

/*
 * Injected imperatively by `injectCopyButtons`, so it cannot use the `Button`
 * component — but it can use the same tokens. It was hardcoded white-on-alpha
 * for a code block that is no longer near-black.
 */
.copy-btn {
  position: absolute;
  top: 6px;
  right: 6px;
  height: 24px;
  padding: 0 8px;
  font-size: var(--ds-text-2xs);
  font-family: inherit;
  border-radius: var(--ds-radius);
  background: rgb(var(--ds-surface));
  color: rgb(var(--ds-ink-secondary));
  border: 1px solid rgb(var(--ds-line));
  cursor: pointer;
  opacity: 0;
  transition:
    opacity var(--ds-dur-fast) var(--ds-ease-std),
    background-color var(--ds-dur-fast) var(--ds-ease-std);
}
.copy-btn:hover { background: rgb(var(--ds-surface-hover)); }
pre:hover .copy-btn,
.copy-btn:focus-visible { opacity: 1; }
</style>
