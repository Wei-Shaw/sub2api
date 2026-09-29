<template>
  <AppLayout>
    <div class="custom-page-layout">
      <div class="card flex-1 min-h-0 overflow-hidden">
        <div v-if="loading" class="flex h-full items-center justify-center py-12">
          <div
            class="h-8 w-8 animate-spin rounded-full border-2 border-accent border-t-transparent"
          ></div>
        </div>

        <div
          v-else-if="!menuItem"
          class="flex h-full items-center justify-center p-10 text-center"
        >
          <div class="max-w-md">
            <Icon name="link" size="lg" class="mx-auto mb-3 text-border-strong" />
            <h3 class="text-h3 font-bold text-accent-strong">
              {{ t('customPage.notFoundTitle') }}
            </h3>
            <p class="mt-2 text-body text-fg-muted">
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
              <button class="toc-close-btn" :aria-label="t('common.close')" @click="tocVisible = false">
                <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M15 18l-6-6 6-6"/></svg>
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
            class="toc-toggle-btn"
            @click="tocVisible = true"
          >
            <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M3 12h18M3 6h18M3 18h18"/></svg>
            <span class="ml-1 text-meta">{{ t('customPage.tableOfContents') }}</span>
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
            <Icon name="link" size="lg" class="mx-auto mb-3 text-border-strong" />
            <h3 class="text-h3 font-bold text-accent-strong">
              {{ t('customPage.notConfiguredTitle') }}
            </h3>
            <p class="mt-2 text-body text-fg-muted">
              {{ t('customPage.notConfiguredDesc') }}
            </p>
          </div>
        </div>

        <!-- Iframe embed mode -->
        <div v-else ref="embedShell" class="custom-embed-shell">
          <a
            v-if="!menuItem?.hide_open_button"
            ref="openButton"
            :href="embeddedUrl"
            target="_blank"
            rel="noopener noreferrer"
            class="btn btn-secondary btn-sm custom-open-fab"
            :style="openButtonPosition ? { left: `${openButtonPosition.x}px`, top: `${openButtonPosition.y}px`, right: 'auto' } : undefined"
            @pointerdown="startButtonDrag"
            @pointermove="moveButtonDrag"
            @pointerup="endButtonDrag"
            @pointercancel="endButtonDrag"
            @lostpointercapture="endButtonDrag"
            @dragstart.prevent
            @click="handleOpenButtonClick"
          >
            <Icon name="externalLink" size="sm" class="mr-1.5" :stroke-width="2" />
            {{ t('customPage.openInNewTab') }}
          </a>
          <!-- referrerpolicy 与 iframeSanitize.ts 对 Markdown 内嵌 iframe 的处理保持一致：
               不向第三方页面泄露当前面板地址。此处不加 sandbox —— 没有 allow-same-origin
               的 sandbox 会让该 frame 变成不透明来源并发送 Origin: null，被后端 CORS 拒绝，
               需要回调的接入方会直接不可用。 -->
          <iframe
            :src="embeddedUrl"
            class="custom-embed-frame"
            referrerpolicy="no-referrer"
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
import { useResizeObserver } from '@vueuse/core'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores'
import { useAuthStore } from '@/stores/auth'
import { useAdminSettingsStore } from '@/stores/adminSettings'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import { buildApiUrl } from '@/api/client'
import { refreshAuthTokens } from '@/api/tokenRefresh'
import { buildEmbeddedUrl, detectTheme } from '@/utils/embedded-url'
import { marked } from 'marked'
import { resolveAllowedIframeHosts, sanitizeCustomPageHtml } from '@/utils/iframeSanitize'

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

const embedShell = ref<HTMLElement | null>(null)
const openButton = ref<HTMLAnchorElement | null>(null)
const openButtonPosition = ref<{ x: number; y: number } | null>(null)
let buttonDrag: { pointerId: number; startX: number; startY: number; x: number; y: number } | null = null
let suppressButtonClick = false

function setOpenButtonPosition(x: number, y: number) {
  const shell = embedShell.value
  const button = openButton.value
  if (!shell || !button) return
  openButtonPosition.value = {
    x: Math.max(0, Math.min(x, shell.clientWidth - button.offsetWidth)),
    y: Math.max(0, Math.min(y, shell.clientHeight - button.offsetHeight)),
  }
}

function startButtonDrag(event: PointerEvent) {
  if (event.button !== 0 || !event.isPrimary || !openButton.value) return
  const button = openButton.value
  suppressButtonClick = false
  buttonDrag = {
    pointerId: event.pointerId, startX: event.clientX, startY: event.clientY,
    x: button.offsetLeft, y: button.offsetTop,
  }
  // Capture keeps receiving moves when the pointer crosses the embedded iframe.
  button.setPointerCapture(event.pointerId)
}

function moveButtonDrag(event: PointerEvent) {
  if (!buttonDrag || event.pointerId !== buttonDrag.pointerId) return
  const dx = event.clientX - buttonDrag.startX
  const dy = event.clientY - buttonDrag.startY
  if (!suppressButtonClick && Math.hypot(dx, dy) < 4) return
  suppressButtonClick = true
  setOpenButtonPosition(buttonDrag.x + dx, buttonDrag.y + dy)
  event.preventDefault()
}

function endButtonDrag(event: PointerEvent) {
  if (!buttonDrag || event.pointerId !== buttonDrag.pointerId) return
  buttonDrag = null
  if (openButton.value?.hasPointerCapture(event.pointerId)) {
    openButton.value.releasePointerCapture(event.pointerId)
  }
}

function handleOpenButtonClick(event: MouseEvent) {
  if (suppressButtonClick && event.detail !== 0) event.preventDefault()
  suppressButtonClick = false
}

useResizeObserver([embedShell, openButton], () => {
  if (openButtonPosition.value) {
    setOpenButtonPosition(openButtonPosition.value.x, openButtonPosition.value.y)
  }
})

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

// iframe 主机白名单：取后端公开设置下发的 `custom_page_iframe_hosts`。
// 字段缺失（旧后端）时 resolveAllowedIframeHosts 回落默认列表；
// 下发空数组时则是运维显式锁死，此页不允许任何 iframe。
const allowedIframeHosts = computed(() =>
  resolveAllowedIframeHosts(appStore.cachedPublicSettings?.custom_page_iframe_hosts),
)

const embeddedUrl = computed(() => {
  if (!menuItem.value || isMarkdownMode.value) return ''
  return buildEmbeddedUrl(
    menuItem.value.url,
    authStore.user?.id,
    authStore.token,
    pageTheme.value,
    locale.value,
    // 访问令牌默认不透传，只有该菜单项显式开启 pass_token 时才附加
    menuItem.value.pass_token === true,
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
    const url = buildApiUrl(`/pages/${encodeURIComponent(slug)}`)
    const load = (token: string | null) =>
      fetch(url, { headers: token ? { Authorization: `Bearer ${token}` } : {} })
    const token = authStore.token
    let resp = await load(token)
    // 原生 fetch 不走 apiClient 的 401 刷新：访问令牌过期时刷新一次后重试
    if (resp.status === 401 && token) {
      try {
        const tokens = await refreshAuthTokens({ failedAccessToken: token })
        resp = await load(tokens.access_token)
      } catch {
        // 刷新失败则沿用下方的 not-found 渲染
      }
    }
    if (!resp.ok) {
      renderedHtml.value = `<p class="text-danger-500">${t('common.pageNotFound')}</p>`
      return
    }
    let raw = await resp.text()

    raw = raw.replace(
      /!\[([^\]]*)\]\(([^)]+)\)/g,
      (match, alt, src) => isRelativeMarkdownAsset(src) ? `![${alt}](${buildPageImageUrl(slug, src)})` : match
    )

    const html = marked.parse(raw) as string
    // 自定义页面是全站唯一放开 iframe 的地方：只允许白名单主机的绝对 https 地址，
    // 且幸存的 iframe 由 sanitizeCustomPageHtml 强制加 sandbox / referrerpolicy。
    const sanitized = sanitizeCustomPageHtml(html, {
      allowedIframeHosts: allowedIframeHosts.value,
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
    renderedHtml.value = '<p class="text-danger-500">Failed to load page</p>'
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
.custom-page-layout {
  @apply flex flex-col;
  height: calc(100vh - 64px - 4rem);
}

.toc-sidebar {
  @apply flex flex-col h-full border-r border-border bg-surface;
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
  }
}

.toc-header {
  @apply flex items-center justify-between px-4 py-3;
  border-bottom: 2px solid rgb(var(--accent));
}

.toc-title {
  @apply text-label font-bold text-accent-strong;
}

.toc-close-btn {
  @apply p-1 rounded-sm text-fg-subtle hover:text-accent-strong hover:bg-accent-weak transition-colors;
}

.toc-nav {
  @apply flex-1 overflow-y-auto py-2 px-2;
}

.toc-item {
  @apply block border-l border-transparent px-2 py-1.5 text-label transition-colors truncate;
  @apply text-fg-muted hover:text-accent-strong hover:bg-accent-weak;
}

.toc-item.toc-active {
  @apply border-l-accent text-accent-strong bg-accent-weak font-semibold;
}

.toc-level-1 { padding-left: 8px; }
.toc-level-2 { padding-left: 20px; }
.toc-level-3 { padding-left: 32px; }
.toc-level-4 { padding-left: 44px; }

.toc-toggle-btn {
  @apply absolute left-2 top-2 z-10 flex items-center px-2 py-1.5 rounded-sm text-label;
  @apply bg-surface border border-border-strong;
  @apply text-fg-muted hover:text-accent-strong hover:border-accent;
  @apply transition-colors cursor-pointer;
}

.custom-embed-shell {
  @apply relative;
  @apply h-full w-full overflow-hidden;
  @apply bg-surface-sunken;
  @apply p-0;
}

.custom-open-fab {
  @apply absolute right-3 top-3 z-10 w-max max-w-full touch-none select-none transition-colors;
  @apply bg-surface;
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
.markdown-page-content h1 { @apply text-3xl font-bold text-accent-strong mt-8 mb-4 pb-2 border-b-2 border-accent; }
.markdown-page-content h2 { @apply text-2xl font-bold text-accent-strong mt-6 mb-3; }
.markdown-page-content h3 { @apply text-xl font-semibold mt-5 mb-2; }
.markdown-page-content h4 { @apply text-lg font-semibold mt-4 mb-2; }
.markdown-page-content p { @apply mb-4; }
.markdown-page-content ul { @apply list-disc pl-6 mb-4; }
.markdown-page-content ol { @apply list-decimal pl-6 mb-4; }
.markdown-page-content li { @apply mb-1; }
.markdown-page-content a { @apply text-accent hover:text-accent-strong underline; }
.markdown-page-content blockquote { @apply border border-accent/40 pl-4 text-fg-muted my-4; }
.markdown-page-content img { @apply max-w-full h-auto rounded-sm my-4; }
.markdown-page-content table { @apply w-full border-collapse my-4; }
.markdown-page-content th { @apply border border-border px-3 py-2 bg-accent-weak text-accent-strong font-semibold text-left; border-bottom: 2px solid rgb(var(--accent)); }
.markdown-page-content td { @apply border border-border px-3 py-2; }
.markdown-page-content code { @apply bg-surface-sunken border border-border px-1 py-px rounded-sm text-sm font-mono; }
.markdown-page-content pre { @apply bg-surface-sunken border border-border-strong text-fg p-4 rounded-sm overflow-x-auto my-4 relative; }
.markdown-page-content pre code { @apply bg-transparent border-0 p-0 text-inherit; }
.markdown-page-content hr { @apply my-6 border-border; }

.copy-btn {
  position: absolute;
  top: 8px;
  right: 8px;
  padding: 4px 10px;
  font-size: 12px;
  border-radius: 2px;
  background: rgb(var(--surface));
  color: rgb(var(--fg-muted));
  border: 1px solid rgb(var(--border-strong));
  cursor: pointer;
  opacity: 0;
  transition: opacity 0.2s, background 0.2s;
  font-family: inherit;
}
.copy-btn:hover { color: rgb(var(--accent-strong)); border-color: rgb(var(--accent)); }
pre:hover .copy-btn { opacity: 1; }
</style>
