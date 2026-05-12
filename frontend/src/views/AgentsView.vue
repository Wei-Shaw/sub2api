<template>
  <div class="agents-shell min-h-screen text-slate-950">
    <header class="relative z-20 border-b border-slate-200/70 bg-white/75 backdrop-blur">
      <nav class="mx-auto flex max-w-7xl items-center justify-between px-5 py-4">
        <router-link :to="homePath" class="flex min-w-0 items-center gap-3">
          <span :class="['brand-mark', siteLogo ? 'brand-mark-image' : '']">
            <img
              v-if="siteLogo"
              :src="siteLogo"
              alt=""
              class="brand-logo-image"
            />
            <span v-else>{{ logoLetters }}</span>
          </span>
          <span class="min-w-0">
            <span class="block truncate text-base font-semibold leading-5 text-slate-700">
              {{ siteName }} Agents Hub
            </span>
            <span class="block truncate text-sm leading-5 text-slate-500">
              {{ copy.headerSubtitle }}
            </span>
          </span>
        </router-link>

        <div class="flex items-center gap-3 sm:gap-5">
          <LocaleSwitcher />
          <router-link
            to="/docs"
            class="hidden text-sm font-medium text-slate-500 transition hover:text-slate-900 sm:inline"
          >
            {{ copy.docs }}
          </router-link>
          <router-link
            :to="homePath"
            class="hidden text-sm font-medium text-slate-500 transition hover:text-slate-900 md:inline"
          >
            {{ copy.mainSite }}
          </router-link>
          <router-link
            v-if="!isInternalMarketingHost"
            to="/login"
            class="nav-button"
          >
            {{ copy.signIn }}
          </router-link>
          <router-link
            v-else
            :to="{ path: homePath, hash: '#contact' }"
            class="nav-button"
          >
            {{ copy.contact }}
          </router-link>
        </div>
      </nav>
    </header>

    <main class="relative z-10">
      <section class="mx-auto max-w-7xl px-5 pb-8 pt-10 md:pb-10 md:pt-14">
        <div class="grid gap-8 lg:grid-cols-[minmax(0,1fr)_22rem] lg:items-end">
          <div class="max-w-4xl">
            <div class="eyebrow-pill">
              <span class="h-2 w-2 rounded-full bg-sky-500"></span>
              {{ copy.eyebrow }}
            </div>
            <h1 class="mt-6 max-w-4xl text-4xl font-black leading-tight tracking-normal text-slate-950 sm:text-6xl">
              {{ copy.titleLead }}
              <span class="hero-highlight">{{ copy.titleHighlight }}</span>
            </h1>
            <p class="mt-6 max-w-3xl text-lg leading-8 text-slate-600">
              {{ copy.description }}
            </p>
          </div>

          <dl class="stats-grid">
            <div class="stat-panel">
              <dt>{{ copy.agentCount }}</dt>
              <dd>{{ agents.length }}</dd>
            </div>
            <div class="stat-panel">
              <dt>{{ copy.divisionCount }}</dt>
              <dd>{{ divisions.length }}</dd>
            </div>
          </dl>
        </div>
      </section>

      <section class="controls-band border-y border-slate-200/75 bg-white/72">
        <div class="mx-auto max-w-7xl px-5 py-5">
          <div class="grid gap-4 lg:grid-cols-[minmax(0,1fr)_auto] lg:items-center">
            <label class="search-box">
              <span class="search-icon" aria-hidden="true">/</span>
              <input
                v-model="query"
                type="search"
                :placeholder="copy.searchPlaceholder"
                class="min-w-0 flex-1 bg-transparent text-sm font-semibold text-slate-900 outline-none placeholder:text-slate-400"
              />
            </label>

            <div class="flex flex-wrap gap-2">
              <button
                type="button"
                class="filter-chip"
                :class="{ 'filter-chip-active': activeDivision === 'all' }"
                @click="activeDivision = 'all'"
              >
                {{ copy.allDivisions }}
                <span>{{ agents.length }}</span>
              </button>
              <button
                v-for="division in divisions"
                :key="division.id"
                type="button"
                class="filter-chip"
                :class="{ 'filter-chip-active': activeDivision === division.id }"
                @click="activeDivision = division.id"
              >
                {{ division.label }}
                <span>{{ division.count }}</span>
              </button>
            </div>
          </div>

          <div class="mt-4 text-sm text-slate-500">
            <p>
              {{ resultText }}
            </p>
          </div>
        </div>
      </section>

      <section class="mx-auto max-w-7xl px-5 py-8">
        <div v-if="filteredAgents.length === 0" class="empty-state">
          <p class="text-base font-bold text-slate-950">{{ copy.emptyTitle }}</p>
          <p class="mt-2 text-sm leading-6 text-slate-600">{{ copy.emptyDescription }}</p>
          <button type="button" class="secondary-action mt-5" @click="clearFilters">
            {{ copy.clearFilters }}
          </button>
        </div>

        <div v-else class="agent-grid">
          <article
            v-for="agent in filteredAgents"
            :key="agent.id"
            class="agent-card"
            :style="agentAccentStyle(agent)"
          >
            <div class="flex items-start justify-between gap-4">
              <div class="flex min-w-0 items-start gap-3">
                <span class="agent-avatar">{{ agent.emoji }}</span>
                <div class="min-w-0">
                  <p class="truncate text-base font-black text-slate-950">{{ agent.name }}</p>
                  <p class="mt-1 text-xs font-bold uppercase tracking-[0.12em] text-slate-400">
                    {{ divisionLabel(agent.division) }}
                  </p>
                </div>
              </div>
              <span class="accent-dot" aria-hidden="true"></span>
            </div>

            <p class="mt-4 line-clamp-3 text-sm leading-6 text-slate-600">
              {{ agent.descriptionEn }}
            </p>
            <p v-if="isZh" class="mt-3 rounded-md bg-sky-50/75 p-3 text-sm leading-6 text-sky-900">
              {{ agent.summaryZh }}
            </p>
            <p v-if="agent.vibeEn" class="mt-3 line-clamp-2 text-xs font-semibold leading-5 text-slate-500">
              {{ agent.vibeEn }}
            </p>

            <div class="mt-5 flex gap-2">
              <button type="button" class="copy-action" @click="copyAgent(agent, 'card')">
                {{ copiedKey === `card:${agent.id}` ? copy.copied : copy.copyMarkdown }}
              </button>
              <button type="button" class="details-action" @click="openAgent(agent)">
                {{ copy.details }}
              </button>
            </div>
          </article>
        </div>
      </section>
    </main>

    <Teleport to="body">
      <transition name="drawer-fade">
        <div
          v-if="selectedAgent"
          class="drawer-backdrop"
          role="presentation"
          @click.self="closeAgent"
        >
          <aside
            class="drawer-panel"
            role="dialog"
            aria-modal="true"
            :aria-label="selectedAgent.name"
          >
            <header class="drawer-header">
              <div class="flex min-w-0 items-start gap-3">
                <span class="agent-avatar drawer-avatar">{{ selectedAgent.emoji }}</span>
                <div class="min-w-0">
                  <p class="break-words text-xl font-black text-slate-950">{{ selectedAgent.name }}</p>
                  <p class="mt-1 text-xs font-bold uppercase tracking-[0.12em] text-slate-400">
                    {{ divisionLabel(selectedAgent.division) }}
                  </p>
                </div>
              </div>
              <button type="button" class="close-button" :aria-label="copy.close" @click="closeAgent">
                ×
              </button>
            </header>

            <div class="drawer-meta">
              <p>{{ selectedAgent.descriptionEn }}</p>
              <div class="mt-4 flex flex-wrap gap-2">
                <button type="button" class="copy-action" @click="copyAgent(selectedAgent, 'drawer')">
                  {{ copiedKey === `drawer:${selectedAgent.id}` ? copy.copied : copy.copyFullMarkdown }}
                </button>
              </div>
            </div>

            <div class="markdown-body" v-html="selectedMarkdownHtml"></div>
          </aside>
        </div>
      </transition>
    </Teleport>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, type CSSProperties } from 'vue'
import { useI18n } from 'vue-i18n'
import { marked } from 'marked'
import DOMPurify from 'dompurify'
import { useAppStore } from '@/stores'
import { isInternalHomeHost, resolveHomePathForHost } from '@/utils/homeDomain'
import LocaleSwitcher from '@/components/common/LocaleSwitcher.vue'
import {
  AGENTS_HUB_DIVISIONS,
  AGENTS_HUB_ITEMS,
  type AgentHubItem,
} from '@/data/agentHub.generated'

type DivisionCopy = {
  en: string
  zh: string
}

const divisionCopy: Record<string, DivisionCopy> = {
  academic: { en: 'Academic', zh: '学术研究' },
  design: { en: 'Design', zh: '设计' },
  engineering: { en: 'Engineering', zh: '工程' },
  finance: { en: 'Finance', zh: '财务' },
  'game-development': { en: 'Game Dev', zh: '游戏开发' },
  marketing: { en: 'Marketing', zh: '营销增长' },
  'paid-media': { en: 'Paid Media', zh: '付费投放' },
  product: { en: 'Product', zh: '产品' },
  'project-management': { en: 'Project', zh: '项目管理' },
  sales: { en: 'Sales', zh: '销售' },
  'spatial-computing': { en: 'Spatial', zh: '空间计算' },
  specialized: { en: 'Specialized', zh: '专项能力' },
  strategy: { en: 'Strategy', zh: '战略' },
  support: { en: 'Support', zh: '客户支持' },
  testing: { en: 'Testing', zh: '测试验证' },
}

marked.setOptions({
  gfm: true,
  breaks: true,
})

const appStore = useAppStore()
const { locale } = useI18n()
const query = ref('')
const activeDivision = ref('all')
const selectedAgent = ref<AgentHubItem | null>(null)
const copiedKey = ref('')

const agents = AGENTS_HUB_ITEMS

const isZh = computed(() => locale.value.toLowerCase().startsWith('zh'))
const copy = computed(() => isZh.value ? zhCopy : enCopy)
const homePath = computed(() => resolveHomePathForHost(appStore.cachedPublicSettings ?? window.__APP_CONFIG__))
const isInternalMarketingHost = computed(() => isInternalHomeHost(appStore.cachedPublicSettings ?? window.__APP_CONFIG__))

const siteName = computed(() => {
  const configured = appStore.cachedPublicSettings?.site_name || appStore.siteName
  return configured?.trim() || 'OceanWay AI'
})
const siteLogo = computed(() => appStore.cachedPublicSettings?.site_logo || appStore.siteLogo || '')

const logoLetters = computed(() => {
  const uppercaseLetters = siteName.value.match(/[A-Z]/g)?.slice(0, 2).join('')
  return uppercaseLetters || siteName.value.slice(0, 2).toUpperCase()
})

const divisionCounts = computed(() => {
  const counts = new Map<string, number>()
  for (const agent of agents) {
    counts.set(agent.division, (counts.get(agent.division) || 0) + 1)
  }
  return counts
})

const divisions = computed(() => AGENTS_HUB_DIVISIONS.map((division) => ({
  id: division,
  label: divisionLabel(division),
  count: divisionCounts.value.get(division) || 0,
})))

const normalizedQuery = computed(() => query.value.trim().toLowerCase())

const filteredAgents = computed(() => {
  const q = normalizedQuery.value
  return agents.filter((agent) => {
    if (activeDivision.value !== 'all' && agent.division !== activeDivision.value) {
      return false
    }
    if (!q) return true

    const haystack = [
      agent.name,
      agent.division,
      divisionLabel(agent.division),
      agent.descriptionEn,
      agent.summaryZh,
      agent.vibeEn,
    ].join(' ').toLowerCase()

    return haystack.includes(q)
  })
})

const resultText = computed(() => {
  const count = filteredAgents.value.length
  if (isZh.value) {
    return `当前显示 ${count} 个 Agent，共 ${agents.length} 个。`
  }
  return `Showing ${count} of ${agents.length} agents.`
})

const selectedMarkdownHtml = computed(() => {
  if (!selectedAgent.value) return ''
  const html = marked.parse(stripFrontmatter(selectedAgent.value.rawMarkdown)) as string
  return DOMPurify.sanitize(html)
})

function divisionLabel(division: string) {
  const label = divisionCopy[division]
  if (!label) return division
  return isZh.value ? label.zh : label.en
}

function agentAccentStyle(agent: AgentHubItem): CSSProperties {
  return {
    '--agent-accent': agent.color || '#006fd6',
  } as CSSProperties
}

function openAgent(agent: AgentHubItem) {
  selectedAgent.value = agent
}

function closeAgent() {
  selectedAgent.value = null
}

function clearFilters() {
  query.value = ''
  activeDivision.value = 'all'
}

function stripFrontmatter(markdown: string) {
  return markdown.replace(/^---\n[\s\S]*?\n---\s*/, '')
}

async function copyAgent(agent: AgentHubItem, scope: 'card' | 'drawer') {
  const key = `${scope}:${agent.id}`
  try {
    await writeClipboard(agent.rawMarkdown)
    copiedKey.value = key
    window.setTimeout(() => {
      if (copiedKey.value === key) copiedKey.value = ''
    }, 1600)
  } catch {
    copiedKey.value = ''
  }
}

async function writeClipboard(text: string) {
  if (navigator.clipboard?.writeText) {
    try {
      await navigator.clipboard.writeText(text)
      return
    } catch {
      // Fall back to a selection-based copy path for embedded/local browsers.
    }
  }

  const textarea = document.createElement('textarea')
  textarea.value = text
  textarea.setAttribute('readonly', 'true')
  textarea.style.position = 'fixed'
  textarea.style.top = '0'
  textarea.style.left = '-9999px'
  textarea.style.opacity = '0'
  document.body.appendChild(textarea)
  textarea.focus()
  textarea.select()
  textarea.setSelectionRange(0, text.length)
  const copied = document.execCommand('copy')
  document.body.removeChild(textarea)
  if (!copied) {
    throw new Error('Clipboard write failed')
  }
}

function handleKeydown(event: KeyboardEvent) {
  if (event.key === 'Escape') {
    closeAgent()
  }
}

onMounted(() => {
  window.addEventListener('keydown', handleKeydown)
  if (!appStore.publicSettingsLoaded) {
    appStore.fetchPublicSettings()
  }
})

onBeforeUnmount(() => {
  window.removeEventListener('keydown', handleKeydown)
})

const zhCopy = {
  headerSubtitle: '可复制、可改造的专业 Agent 提示词',
  docs: '文档',
  mainSite: '主站',
  signIn: '登录',
  contact: '联系',
  eyebrow: 'Agents Hub',
  titleLead: '把专业 Agent 变成',
  titleHighlight: '可检索的工作台。',
  description:
    '这里内置一组专业 Agent Markdown 快照。你可以按方向筛选、查看原始定义，并一键复制完整 Markdown 到自己的工具或工作流里。',
  agentCount: 'Agents',
  divisionCount: '分类',
  searchPlaceholder: '搜索 Agent 名称、能力、分类或 vibe',
  allDivisions: '全部',
  emptyTitle: '没有找到匹配的 Agent',
  emptyDescription: '换一个关键词，或者清空分类筛选后再看一次。',
  clearFilters: '清空筛选',
  copyMarkdown: '复制',
  copyFullMarkdown: '复制完整 Markdown',
  copied: '已复制',
  details: '详情',
  close: '关闭',
}

const enCopy = {
  headerSubtitle: 'Copy-ready specialist agent prompts',
  docs: 'Docs',
  mainSite: 'Main site',
  signIn: 'Sign in',
  contact: 'Contact',
  eyebrow: 'Agents Hub',
  titleLead: 'A searchable hub for',
  titleHighlight: 'specialist agents.',
  description:
    'Browse the bundled specialist-agent library, filter by division, inspect the original Markdown, and copy complete agent definitions into your own tools or workflows.',
  agentCount: 'Agents',
  divisionCount: 'Divisions',
  searchPlaceholder: 'Search name, capability, division, or vibe',
  allDivisions: 'All',
  emptyTitle: 'No agents found',
  emptyDescription: 'Try another keyword, or clear the active division filter.',
  clearFilters: 'Clear filters',
  copyMarkdown: 'Copy',
  copyFullMarkdown: 'Copy full Markdown',
  copied: 'Copied',
  details: 'Details',
  close: 'Close',
}
</script>

<style scoped>
.agents-shell {
  position: relative;
  overflow: hidden;
  background:
    linear-gradient(180deg, rgba(242, 248, 255, 0.96) 0%, rgba(250, 252, 255, 0.94) 38%, rgba(232, 246, 255, 0.98) 100%);
}

.agents-shell::before {
  position: fixed;
  inset: 0;
  pointer-events: none;
  content: '';
  background:
    linear-gradient(110deg, rgba(255, 255, 255, 0.86) 0%, rgba(255, 255, 255, 0) 44%),
    linear-gradient(180deg, rgba(196, 222, 250, 0) 55%, rgba(196, 222, 250, 0.38) 100%);
}

.brand-mark {
  display: inline-flex;
  width: 2.375rem;
  height: 2.375rem;
  flex: 0 0 auto;
  align-items: center;
  justify-content: center;
  border-radius: 0.5rem;
  background: linear-gradient(135deg, #002080 0%, #001040 78%);
  color: rgba(255, 255, 255, 0.78);
  font-size: 0.75rem;
  font-weight: 700;
  letter-spacing: 0;
  box-shadow: 0 10px 26px rgba(15, 23, 42, 0.18);
}

.brand-mark-image {
  overflow: hidden;
  border-radius: 0.65rem;
  background: #001040;
  box-shadow: none;
}

.brand-logo-image {
  display: block;
  width: 100%;
  height: 100%;
  object-fit: contain;
}

.nav-button,
.secondary-action,
.copy-action,
.details-action {
  display: inline-flex;
  min-height: 2.5rem;
  align-items: center;
  justify-content: center;
  border-radius: 0.5rem;
  font-size: 0.875rem;
  font-weight: 800;
  transition:
    border-color 0.16s ease,
    background-color 0.16s ease,
    color 0.16s ease,
    transform 0.16s ease;
}

.nav-button,
.secondary-action,
.details-action {
  border: 1px solid rgba(148, 163, 184, 0.35);
  background: rgba(255, 255, 255, 0.82);
  padding: 0.5rem 1rem;
  color: #111827;
}

.copy-action {
  flex: 1;
  background: #001040;
  padding: 0.5rem 0.9rem;
  color: white;
}

.details-action {
  flex: 1;
}

.nav-button:hover,
.secondary-action:hover,
.copy-action:hover,
.details-action:hover {
  transform: translateY(-1px);
}

.copy-action:hover {
  background: #002080;
}

.details-action:hover,
.nav-button:hover,
.secondary-action:hover {
  border-color: rgba(71, 85, 105, 0.45);
}

.eyebrow-pill {
  display: inline-flex;
  min-height: 2.25rem;
  align-items: center;
  gap: 0.6rem;
  border: 1px solid rgba(0, 160, 255, 0.24);
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.7);
  padding: 0.45rem 0.85rem;
  color: #005db8;
  font-size: 0.875rem;
  font-weight: 800;
  box-shadow: 0 12px 28px rgba(15, 23, 42, 0.04);
}

.hero-highlight {
  color: #006fd6;
}

.stats-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 0.75rem;
}

.stat-panel {
  border: 1px solid rgba(203, 213, 225, 0.72);
  border-radius: 0.5rem;
  background: rgba(255, 255, 255, 0.78);
  padding: 1rem;
  box-shadow: 0 14px 30px rgba(15, 23, 42, 0.04);
}

.stat-panel dt {
  color: #64748b;
  font-size: 0.75rem;
  font-weight: 800;
  letter-spacing: 0.12em;
  text-transform: uppercase;
}

.stat-panel dd {
  margin-top: 0.45rem;
  color: #0f172a;
  font-size: 1.75rem;
  font-weight: 900;
  line-height: 1;
}

.controls-band {
  backdrop-filter: blur(14px);
}

.search-box {
  display: flex;
  min-height: 3rem;
  min-width: 0;
  align-items: center;
  gap: 0.8rem;
  border: 1px solid rgba(148, 163, 184, 0.36);
  border-radius: 0.5rem;
  background: rgba(255, 255, 255, 0.86);
  padding: 0 1rem;
  box-shadow: 0 10px 24px rgba(15, 23, 42, 0.04);
}

.search-icon {
  display: inline-flex;
  width: 1.5rem;
  height: 1.5rem;
  flex: 0 0 auto;
  align-items: center;
  justify-content: center;
  border-radius: 0.375rem;
  background: #e8f6ff;
  color: #006fd6;
  font-size: 0.85rem;
  font-weight: 900;
}

.filter-chip {
  display: inline-flex;
  min-height: 2.25rem;
  align-items: center;
  gap: 0.45rem;
  border: 1px solid rgba(148, 163, 184, 0.35);
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.78);
  padding: 0.42rem 0.75rem;
  color: #475569;
  font-size: 0.78rem;
  font-weight: 800;
  transition:
    border-color 0.16s ease,
    background-color 0.16s ease,
    color 0.16s ease;
}

.filter-chip span {
  color: #94a3b8;
  font-weight: 900;
}

.filter-chip:hover,
.filter-chip-active {
  border-color: rgba(0, 160, 255, 0.38);
  background: rgba(232, 246, 255, 0.86);
  color: #005db8;
}

.filter-chip-active span {
  color: #005db8;
}

.agent-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(min(100%, 18rem), 1fr));
  gap: 1rem;
}

.agent-card {
  --agent-accent: #006fd6;
  display: flex;
  min-height: 18rem;
  flex-direction: column;
  border: 1px solid rgba(203, 213, 225, 0.72);
  border-top: 3px solid var(--agent-accent);
  border-radius: 0.5rem;
  background: rgba(255, 255, 255, 0.82);
  padding: 1rem;
  box-shadow: 0 14px 30px rgba(15, 23, 42, 0.04);
}

.agent-card > .mt-5 {
  margin-top: auto;
  padding-top: 1.25rem;
}

.agent-avatar {
  display: inline-flex;
  width: 2.625rem;
  height: 2.625rem;
  flex: 0 0 auto;
  align-items: center;
  justify-content: center;
  border-radius: 0.5rem;
  background: #f8fafc;
  color: #0f172a;
  font-size: 1.3rem;
  box-shadow: inset 0 0 0 1px rgba(203, 213, 225, 0.8);
}

.drawer-avatar {
  width: 3rem;
  height: 3rem;
  font-size: 1.5rem;
}

.accent-dot {
  width: 0.7rem;
  height: 0.7rem;
  flex: 0 0 auto;
  border-radius: 999px;
  background: var(--agent-accent);
}

.empty-state {
  border: 1px solid rgba(203, 213, 225, 0.72);
  border-radius: 0.5rem;
  background: rgba(255, 255, 255, 0.82);
  padding: 2rem;
  text-align: center;
}

.drawer-backdrop {
  position: fixed;
  z-index: 80;
  inset: 0;
  display: flex;
  justify-content: flex-end;
  background: rgba(15, 23, 42, 0.42);
  backdrop-filter: blur(8px);
}

.drawer-panel {
  width: min(100vw, 48rem);
  height: 100%;
  overflow-y: auto;
  background:
    linear-gradient(180deg, rgba(248, 250, 252, 0.98) 0%, rgba(255, 255, 255, 0.99) 100%);
  box-shadow: -24px 0 45px rgba(15, 23, 42, 0.22);
}

.drawer-header {
  position: sticky;
  top: 0;
  z-index: 2;
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 1rem;
  border-bottom: 1px solid rgba(203, 213, 225, 0.78);
  background: rgba(255, 255, 255, 0.92);
  padding: 1rem;
  backdrop-filter: blur(14px);
}

.close-button {
  display: inline-flex;
  width: 2.25rem;
  height: 2.25rem;
  flex: 0 0 auto;
  align-items: center;
  justify-content: center;
  border: 1px solid rgba(148, 163, 184, 0.35);
  border-radius: 0.5rem;
  background: white;
  color: #334155;
  font-size: 1.5rem;
  line-height: 1;
}

.drawer-meta {
  border-bottom: 1px solid rgba(226, 232, 240, 0.9);
  padding: 1rem;
  color: #475569;
  font-size: 0.95rem;
  line-height: 1.7;
}

.markdown-body {
  padding: 1.25rem;
  color: #334155;
  font-size: 0.95rem;
  line-height: 1.75;
  overflow-wrap: anywhere;
}

.markdown-body :deep(h1),
.markdown-body :deep(h2),
.markdown-body :deep(h3) {
  color: #0f172a;
  font-weight: 900;
  letter-spacing: 0;
}

.markdown-body :deep(h1) {
  margin: 0.4rem 0 1rem;
  font-size: 1.85rem;
  line-height: 1.2;
}

.markdown-body :deep(h2) {
  margin: 1.5rem 0 0.6rem;
  font-size: 1.25rem;
}

.markdown-body :deep(h3) {
  margin: 1.1rem 0 0.45rem;
  font-size: 1.05rem;
}

.markdown-body :deep(p),
.markdown-body :deep(ul),
.markdown-body :deep(ol),
.markdown-body :deep(pre) {
  margin-top: 0.75rem;
}

.markdown-body :deep(ul),
.markdown-body :deep(ol) {
  padding-left: 1.25rem;
}

.markdown-body :deep(li + li) {
  margin-top: 0.35rem;
}

.markdown-body :deep(code) {
  border-radius: 0.35rem;
  background: #eef2f7;
  padding: 0.12rem 0.3rem;
  color: #0f172a;
  font-size: 0.85em;
}

.markdown-body :deep(pre) {
  overflow-x: auto;
  border-radius: 0.5rem;
  background: #001040;
  padding: 1rem;
  color: #d1d5db;
}

.markdown-body :deep(pre code) {
  background: transparent;
  padding: 0;
  color: inherit;
}

.markdown-body :deep(a) {
  color: #005db8;
  font-weight: 700;
}

.drawer-fade-enter-active,
.drawer-fade-leave-active {
  transition: opacity 0.18s ease;
}

.drawer-fade-enter-from,
.drawer-fade-leave-to {
  opacity: 0;
}

@media (max-width: 640px) {
  .stats-grid {
    grid-template-columns: 1fr;
  }

  .copy-action,
  .details-action {
    min-width: 0;
    padding-right: 0.6rem;
    padding-left: 0.6rem;
  }

  .drawer-panel {
    width: 100vw;
  }
}
</style>
