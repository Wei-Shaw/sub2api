<template>
  <AppLayout>
    <div class="space-y-6">
      <div class="flex flex-col gap-4 lg:flex-row lg:items-center lg:justify-between">
        <div>
          <h1 class="text-2xl font-semibold text-gray-900 dark:text-white">{{ t('admin.audit.title') }}</h1>
          <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('admin.audit.description') }}</p>
        </div>
        <div class="flex flex-wrap items-center gap-2">
          <button type="button" class="btn btn-secondary inline-flex items-center gap-2" @click="resetFilters">
            <Icon name="refresh" size="sm" />
            {{ t('admin.audit.reset') }}
          </button>
          <button type="button" class="btn btn-primary inline-flex items-center gap-2" :disabled="loading" @click="loadRows">
            <Icon name="refresh" size="sm" />
            {{ t('admin.audit.refresh') }}
          </button>
        </div>
      </div>

      <div class="grid grid-cols-1 gap-3 md:grid-cols-2 xl:grid-cols-4">
        <div
          v-for="item in overviewItems"
          :key="item.key"
          class="rounded-lg border border-gray-100 bg-white px-4 py-3 shadow-sm dark:border-dark-700 dark:bg-dark-800"
        >
          <div class="flex min-w-0 items-center gap-3">
            <div class="flex h-9 w-9 flex-shrink-0 items-center justify-center rounded-lg" :class="item.iconClass">
              <Icon :name="item.icon" size="sm" />
            </div>
            <div class="min-w-0 flex-1">
              <p class="truncate text-xs font-medium text-gray-500 dark:text-gray-400">{{ item.label }}</p>
              <div class="mt-1 flex min-w-0 items-baseline gap-2">
                <p class="truncate text-xl font-semibold leading-7 text-gray-900 dark:text-white">{{ item.value }}</p>
                <p v-if="item.meta" class="truncate text-xs text-gray-500 dark:text-gray-400">{{ item.meta }}</p>
              </div>
            </div>
          </div>
        </div>
      </div>

      <div class="card p-4">
        <div class="grid grid-cols-1 gap-3 md:grid-cols-2 xl:grid-cols-4">
          <div>
            <label class="mb-1 block text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('admin.audit.search') }}</label>
            <div class="relative">
              <Icon name="search" size="sm" class="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-gray-400" />
              <input
                v-model="filters.keyword"
                type="search"
                class="input w-full pl-9"
                :placeholder="t('admin.audit.searchPlaceholder')"
              />
            </div>
          </div>
          <div>
            <label class="mb-1 block text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('admin.audit.platform') }}</label>
            <select v-model="filters.platform" class="input w-full">
              <option v-for="option in platformOptions" :key="option.value" :value="option.value">{{ option.label }}</option>
            </select>
          </div>
          <div>
            <label class="mb-1 block text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('admin.audit.model') }}</label>
            <input v-model="filters.model" type="text" class="input w-full" :placeholder="t('admin.audit.modelPlaceholder')" />
          </div>
          <div>
            <label class="mb-1 block text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('admin.audit.endpoint') }}</label>
            <input v-model="filters.endpoint" type="text" class="input w-full" :placeholder="t('admin.audit.endpointPlaceholder')" />
          </div>
        </div>
      </div>

      <div class="grid grid-cols-1 gap-6 xl:grid-cols-[minmax(0,1fr)_380px]">
        <div class="card overflow-hidden">
          <div class="flex items-center justify-between gap-3 border-b border-gray-100 px-4 py-3 dark:border-dark-700">
            <div>
              <h2 class="text-base font-semibold text-gray-900 dark:text-white">{{ t('admin.audit.records') }}</h2>
              <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.audit.recordCount', { count: pagination.total }) }}</p>
            </div>
            <span class="inline-flex items-center rounded-full bg-gray-100 px-2.5 py-1 text-xs font-medium text-gray-600 dark:bg-dark-700 dark:text-gray-300">
              {{ t('admin.audit.retentionBadge') }}
            </span>
          </div>

          <div v-if="errorMessage" class="border-b border-rose-100 bg-rose-50 px-4 py-3 text-sm text-rose-700 dark:border-rose-900/40 dark:bg-rose-900/20 dark:text-rose-200">
            {{ errorMessage }}
          </div>

          <div class="overflow-x-auto">
            <table class="min-w-full divide-y divide-gray-200 dark:divide-dark-700">
              <thead class="bg-gray-50 dark:bg-dark-800/80">
                <tr>
                  <th class="px-4 py-3 text-left text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400">{{ t('admin.audit.time') }}</th>
                  <th class="px-4 py-3 text-left text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400">{{ t('admin.audit.user') }}</th>
                  <th class="px-4 py-3 text-left text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400">{{ t('admin.audit.platform') }}</th>
                  <th class="px-4 py-3 text-left text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400">{{ t('admin.audit.model') }}</th>
                  <th class="px-4 py-3 text-left text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400">{{ t('admin.audit.session') }}</th>
                  <th class="px-4 py-3 text-left text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400">{{ t('admin.audit.statusCode') }}</th>
                  <th class="px-4 py-3 text-right text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400">{{ t('admin.audit.actions') }}</th>
                </tr>
              </thead>
              <tbody class="divide-y divide-gray-100 bg-white dark:divide-dark-700 dark:bg-dark-800">
                <tr v-if="loading">
                  <td colspan="7" class="px-4 py-14 text-center text-sm text-gray-500 dark:text-gray-400">
                    {{ t('admin.audit.loading') }}
                  </td>
                </tr>
                <tr v-else-if="rows.length === 0">
                  <td colspan="7" class="px-4 py-14 text-center">
                    <div class="mx-auto flex max-w-sm flex-col items-center">
                      <div class="flex h-12 w-12 items-center justify-center rounded-lg bg-gray-100 text-gray-500 dark:bg-dark-700 dark:text-gray-300">
                        <Icon name="inbox" size="lg" />
                      </div>
                      <p class="mt-4 text-sm font-medium text-gray-900 dark:text-white">{{ t('admin.audit.emptyTitle') }}</p>
                      <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('admin.audit.emptyDescription') }}</p>
                    </div>
                  </td>
                </tr>
                <tr
                  v-for="row in rows"
                  :key="row.id"
                  class="cursor-pointer transition-colors hover:bg-gray-50 dark:hover:bg-dark-700/50"
                  :class="{ 'bg-primary-50/70 dark:bg-primary-900/10': selectedRow?.id === row.id }"
                  @click="selectedRow = row"
                >
                  <td class="whitespace-nowrap px-4 py-3 text-sm text-gray-600 dark:text-gray-300">{{ row.time }}</td>
                  <td class="px-4 py-3">
                    <div class="min-w-0">
                      <p class="truncate text-sm font-medium text-gray-900 dark:text-white">{{ row.user }}</p>
                      <p class="truncate text-xs text-gray-500 dark:text-gray-400">{{ row.apiKey }}</p>
                    </div>
                  </td>
                  <td class="whitespace-nowrap px-4 py-3 text-sm text-gray-600 dark:text-gray-300">{{ row.platform }}</td>
                  <td class="whitespace-nowrap px-4 py-3 font-mono text-xs text-gray-600 dark:text-gray-300">{{ row.model }}</td>
                  <td class="px-4 py-3">
                    <div class="min-w-0">
                      <p class="truncate font-mono text-xs text-gray-600 dark:text-gray-300">{{ row.sessionId }}</p>
                      <p class="truncate text-xs text-gray-500 dark:text-gray-400">{{ t('admin.audit.turnCount', { count: row.requestCount }) }}</p>
                    </div>
                  </td>
                  <td class="whitespace-nowrap px-4 py-3">
                    <span class="inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium" :class="statusClass(row.statusCode)">
                      {{ row.statusCode || '-' }}
                    </span>
                  </td>
                  <td class="whitespace-nowrap px-4 py-3 text-right">
                    <button type="button" class="btn btn-secondary px-2 py-1 text-xs" @click.stop="selectedRow = row">
                      {{ t('admin.audit.review') }}
                    </button>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>

          <div class="flex flex-col gap-3 border-t border-gray-100 px-4 py-3 text-sm dark:border-dark-700 sm:flex-row sm:items-center sm:justify-between">
            <div class="text-gray-500 dark:text-gray-400">
              {{ t('admin.audit.pageSummary', { page: pagination.page, pages: pagination.pages }) }}
            </div>
            <div class="flex items-center gap-2">
              <button type="button" class="btn btn-secondary px-3 py-1.5 text-xs" :disabled="loading || pagination.page <= 1" @click="goPage(pagination.page - 1)">
                {{ t('admin.audit.previousPage') }}
              </button>
              <button type="button" class="btn btn-secondary px-3 py-1.5 text-xs" :disabled="loading || pagination.page >= pagination.pages" @click="goPage(pagination.page + 1)">
                {{ t('admin.audit.nextPage') }}
              </button>
            </div>
          </div>
        </div>

        <aside class="card flex max-h-[calc(100vh-9rem)] flex-col overflow-hidden">
          <div class="flex-shrink-0 border-b border-gray-100 px-4 py-3 dark:border-dark-700">
            <h2 class="text-base font-semibold text-gray-900 dark:text-white">{{ t('admin.audit.detail') }}</h2>
            <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">{{ selectedRow?.sessionId || selectedRow?.requestId || t('admin.audit.noSelection') }}</p>
          </div>
          <div v-if="selectedRow" class="min-h-0 flex-1 space-y-4 overflow-y-auto p-4">
            <div class="grid grid-cols-2 gap-3 text-sm">
              <div>
                <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.audit.account') }}</p>
                <p class="mt-1 truncate font-medium text-gray-900 dark:text-white">{{ selectedRow.account }}</p>
              </div>
              <div>
                <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.audit.turns') }}</p>
                <p class="mt-1 truncate font-medium text-gray-900 dark:text-white">{{ selectedRow.requestCount }}</p>
              </div>
              <div>
                <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.audit.totalLatency') }}</p>
                <p class="mt-1 truncate font-medium text-gray-900 dark:text-white">{{ selectedRow.latency }}</p>
              </div>
              <div>
                <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.audit.statusCode') }}</p>
                <p class="mt-1 truncate font-medium text-gray-900 dark:text-white">{{ selectedRow.statusCode || '-' }}</p>
              </div>
              <div>
                <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.audit.ipAddress') }}</p>
                <p class="mt-1 truncate font-medium text-gray-900 dark:text-white">{{ selectedRow.ipAddress }}</p>
              </div>
            </div>
            <div class="space-y-3">
              <div v-for="turn in paginatedTurns" :key="turn.key" class="space-y-2 border-t border-gray-100 pt-3 first:border-t-0 first:pt-0 dark:border-dark-700">
                <p class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ turn.title }}</p>
                <div class="space-y-2">
                  <div>
                    <p class="mb-1 text-xs font-medium text-sky-700 dark:text-sky-300">
                      {{ t('admin.audit.requestContent') }}<span v-if="turn.requestTruncated"> · {{ t('admin.audit.truncated') }}</span>
                    </p>
                    <pre class="max-h-40 overflow-auto rounded-lg bg-sky-50 p-3 text-xs text-gray-800 dark:bg-sky-950/30 dark:text-gray-100">{{ turn.request }}</pre>
                  </div>
                  <div>
                    <p class="mb-1 text-xs font-medium text-emerald-700 dark:text-emerald-300">
                      {{ t('admin.audit.responseContent') }}<span v-if="turn.responseTruncated"> · {{ t('admin.audit.truncated') }}</span>
                    </p>
                    <pre class="max-h-40 overflow-auto rounded-lg bg-emerald-50 p-3 text-xs text-gray-800 dark:bg-emerald-950/30 dark:text-gray-100">{{ turn.response }}</pre>
                  </div>
                </div>
              </div>
            </div>
            <div v-if="turnPages > 1" class="flex flex-col gap-2 border-t border-gray-100 pt-3 text-xs dark:border-dark-700 sm:flex-row sm:items-center sm:justify-between">
              <p class="text-gray-500 dark:text-gray-400">
                {{ t('admin.audit.turnPageSummary', { start: turnRangeStart, end: turnRangeEnd, total: selectedTurns.length }) }}
              </p>
              <div class="flex items-center gap-2">
                <button type="button" class="btn btn-secondary px-3 py-1.5 text-xs" :disabled="turnPagination.page <= 1" @click="goTurnPage(turnPagination.page - 1)">
                  {{ t('admin.audit.previousPage') }}
                </button>
                <span class="text-gray-500 dark:text-gray-400">{{ t('admin.audit.pageSummary', { page: turnPagination.page, pages: turnPages }) }}</span>
                <button type="button" class="btn btn-secondary px-3 py-1.5 text-xs" :disabled="turnPagination.page >= turnPages" @click="goTurnPage(turnPagination.page + 1)">
                  {{ t('admin.audit.nextPage') }}
                </button>
              </div>
            </div>
          </div>
          <div v-else class="flex min-h-[360px] flex-1 items-center justify-center px-6 text-center">
            <div>
              <div class="mx-auto flex h-12 w-12 items-center justify-center rounded-lg bg-gray-100 text-gray-500 dark:bg-dark-700 dark:text-gray-300">
                <Icon name="clipboard" size="lg" />
              </div>
              <p class="mt-4 text-sm font-medium text-gray-900 dark:text-white">{{ t('admin.audit.noSelectionTitle') }}</p>
              <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('admin.audit.noSelectionDescription') }}</p>
            </div>
          </div>
        </aside>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import auditAPI, { type AuditLog } from '@/api/admin/audit'

interface AuditContentItem {
  request_id?: string
  model?: string
  content?: unknown
  truncated?: boolean
  created_at?: string
}

interface AuditTurn {
  key: string
  title: string
  request: string
  response: string
  requestTruncated: boolean
  responseTruncated: boolean
}

interface AuditRow {
  id: string
  time: string
  user: string
  apiKey: string
  account: string
  platform: string
  model: string
  endpoint: string
  sessionId: string
  requestCount: number
  statusCode: number
  requestId: string
  latency: string
  ipAddress: string
  requestTruncated: boolean
  responseTruncated: boolean
  turns: AuditTurn[]
}

const { t } = useI18n()

const filters = reactive({
  keyword: '',
  platform: 'all',
  model: '',
  endpoint: '',
})

const rows = ref<AuditRow[]>([])
const selectedRow = ref<AuditRow | null>(null)
const loading = ref(false)
const errorMessage = ref('')
const pagination = reactive({
  page: 1,
  pageSize: 20,
  total: 0,
  pages: 1,
})
const turnPagination = reactive({
  page: 1,
  pageSize: 5,
})
let activeController: AbortController | null = null

const overviewItems = computed(() => [
  {
    key: 'total',
    label: t('admin.audit.totalRecords'),
    value: pagination.total.toLocaleString(),
    meta: t('admin.audit.allModels'),
    icon: 'clipboard' as const,
    iconClass: 'bg-sky-50 text-sky-600 dark:bg-sky-900/20 dark:text-sky-300',
  },
  {
    key: 'success',
    label: t('admin.audit.successful'),
    value: rows.value.filter((row) => row.statusCode >= 200 && row.statusCode < 300).length.toLocaleString(),
    meta: t('admin.audit.currentPage'),
    icon: 'eye' as const,
    iconClass: 'bg-amber-50 text-amber-600 dark:bg-amber-900/20 dark:text-amber-300',
  },
  {
    key: 'errors',
    label: t('admin.audit.failed'),
    value: rows.value.filter((row) => row.statusCode >= 400).length.toLocaleString(),
    meta: t('admin.audit.currentPage'),
    icon: 'shield' as const,
    iconClass: 'bg-rose-50 text-rose-600 dark:bg-rose-900/20 dark:text-rose-300',
  },
  {
    key: 'coverage',
    label: t('admin.audit.coverage'),
    value: rows.value.length > 0 ? '100%' : '0%',
    meta: t('admin.audit.captured'),
    icon: 'database' as const,
    iconClass: 'bg-emerald-50 text-emerald-600 dark:bg-emerald-900/20 dark:text-emerald-300',
  },
])

const platformOptions = computed(() => [
  { value: 'all', label: t('admin.audit.allPlatforms') },
  { value: 'openai', label: 'OpenAI' },
  { value: 'anthropic', label: 'Claude / Anthropic' },
  { value: 'gemini', label: 'Gemini' },
  { value: 'antigravity', label: 'Antigravity' },
])

const selectedTurns = computed(() => selectedRow.value?.turns || [])
const turnPages = computed(() => Math.max(1, Math.ceil(selectedTurns.value.length / turnPagination.pageSize)))
const paginatedTurns = computed(() => {
  const start = (turnPagination.page - 1) * turnPagination.pageSize
  return selectedTurns.value.slice(start, start + turnPagination.pageSize)
})
const turnRangeStart = computed(() => {
  if (selectedTurns.value.length === 0) return 0
  return (turnPagination.page - 1) * turnPagination.pageSize + 1
})
const turnRangeEnd = computed(() => Math.min(turnPagination.page * turnPagination.pageSize, selectedTurns.value.length))

function resetFilters() {
  filters.keyword = ''
  filters.platform = 'all'
  filters.model = ''
  filters.endpoint = ''
  pagination.page = 1
  loadRows()
}

function statusClass(statusCode: number) {
  if (statusCode >= 500) return 'bg-rose-50 text-rose-700 dark:bg-rose-900/20 dark:text-rose-300'
  if (statusCode >= 400) return 'bg-amber-50 text-amber-700 dark:bg-amber-900/20 dark:text-amber-300'
  if (statusCode >= 200 && statusCode < 300) return 'bg-emerald-50 text-emerald-700 dark:bg-emerald-900/20 dark:text-emerald-300'
  return 'bg-gray-100 text-gray-700 dark:bg-dark-700 dark:text-gray-300'
}

function formatTime(value: string) {
  if (!value) return '-'
  return new Intl.DateTimeFormat(undefined, {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  }).format(new Date(value))
}

function prettifyJSON(value: string) {
  if (!value) return ''
  try {
    return JSON.stringify(JSON.parse(value), null, 2)
  } catch {
    return value
  }
}

function contentToText(value: unknown) {
  if (value === undefined || value === null) return ''
  if (typeof value === 'string') return value
  return JSON.stringify(value, null, 2)
}

function extractTextFromContent(value: unknown): string[] {
  if (value === undefined || value === null) return []
  if (typeof value === 'string') return [value]
  if (Array.isArray(value)) {
    return value.flatMap((item) => extractTextFromContent(item))
  }
  if (typeof value === 'object') {
    const item = value as Record<string, unknown>
    const type = typeof item.type === 'string' ? item.type : ''
    if (typeof item.text === 'string' && (!type || type.includes('text') || type === 'input')) return [item.text]
    if (typeof item.content === 'string') return [item.content]
    if (Array.isArray(item.content)) return extractTextFromContent(item.content)
    if (Array.isArray(item.parts)) return extractTextFromContent(item.parts)
  }
  return []
}

function extractSessionTagText(value: string): string {
  const match = value.match(/<session>\s*([\s\S]*?)\s*<\/session>/)
  return match?.[1]?.trim() || ''
}

function isAuditUserText(value: string) {
  const text = value.trim()
  if (!text) return false
  if (text.startsWith('<system-reminder>')) return false
  if (text.startsWith('<command-message>')) return false
  if (text.startsWith('<local-command-stdout>')) return false
  if (text.startsWith('<tool_use_id>')) return false
  if (text.startsWith('x-anthropic-billing-header:')) return false
  if (text.startsWith("You are Claude Code, Anthropic's official CLI for Claude.")) return false
  if (text.startsWith('You are an interactive agent that helps users with software engineering tasks.')) return false
  if (text.includes('The following skills are available for use with the Skill tool')) return false
  return true
}

function cleanAuditUserText(value: string) {
  return value
    .replace(/<system-reminder>[\s\S]*?<\/system-reminder>/g, '')
    .replace(/<command-message>[\s\S]*?<\/command-message>/g, '')
    .replace(/<local-command-stdout>[\s\S]*?<\/local-command-stdout>/g, '')
    .trim()
}

function lastUsefulUserText(values: string[]) {
  const cleaned = values.map(cleanAuditUserText).filter(isAuditUserText)
  return cleaned[cleaned.length - 1] || ''
}

function extractEscapedJSONTextFields(value: string) {
  const texts: string[] = []
  const searchable = value
    .split(',"system":[')[0]
    .split(',\\"system\\":[')[0]
  const patterns = [
    /\\"text\\":\\"((?:\\\\.|[^"\\])*)\\"/g,
    /"text"\s*:\s*"((?:\\.|[^"\\])*)"/g,
  ]
  for (const pattern of patterns) {
    let match: RegExpExecArray | null
    while ((match = pattern.exec(searchable)) !== null) {
      try {
        texts.push(JSON.parse(`"${match[1]}"`))
      } catch {
        texts.push(match[1].replace(/\\n/g, '\n').replace(/\\"/g, '"'))
      }
    }
  }
  return texts
}

function extractEscapedJSONUserTextFields(value: string) {
  const texts: string[] = []
  const searchable = value
    .split(',"system":[')[0]
    .split(',\\"system\\":[')[0]
  const patterns = [
    /\{\\"role\\":\\"user\\"[\s\S]*?\\"content\\":\[(?:[\s\S]*?\\"text\\":\\"((?:\\\\.|[^"\\])*)\\")+[\s\S]*?\]/g,
    /\{"role"\s*:\s*"user"[\s\S]*?"content"\s*:\s*\[(?:[\s\S]*?"text"\s*:\s*"((?:\\.|[^"\\])*)")+[\s\S]*?\]/g,
  ]
  for (const pattern of patterns) {
    let match: RegExpExecArray | null
    while ((match = pattern.exec(searchable)) !== null) {
      if (!match[1]) continue
      try {
        texts.push(JSON.parse(`"${match[1]}"`))
      } catch {
        texts.push(match[1].replace(/\\n/g, '\n').replace(/\\"/g, '"'))
      }
    }
  }
  return texts
}

function extractEscapedJSONContentFields(value: string) {
  const texts: string[] = []
  const searchable = value
    .split(',"system":[')[0]
    .split(',\\"system\\":[')[0]
  const patterns = [
    /\\"content\\":\\"((?:\\\\.|[^"\\])*)\\"/g,
    /"content"\s*:\s*"((?:\\.|[^"\\])*)"/g,
  ]
  for (const pattern of patterns) {
    let match: RegExpExecArray | null
    while ((match = pattern.exec(searchable)) !== null) {
      const before = searchable.slice(Math.max(0, match.index - 80), match.index)
      if (before.includes('\\"role\\":\\"assistant') || before.includes('"role":"assistant')) continue
      try {
        texts.push(JSON.parse(`"${match[1]}"`))
      } catch {
        texts.push(match[1].replace(/\\n/g, '\n').replace(/\\"/g, '"'))
      }
    }
  }
  return texts
}

function extractUserInputText(value: unknown): string {
  const raw = contentToText(value)
  if (!raw.trim()) return ''
  try {
    const parsed = JSON.parse(raw) as Record<string, unknown>
    const messages = Array.isArray(parsed.messages) ? parsed.messages : Array.isArray(parsed.input) ? parsed.input : []
    const messageTexts = messages.flatMap((message) => {
      if (!message || typeof message !== 'object') return []
      const item = message as Record<string, unknown>
      if (item.role !== 'user') return []
      return extractTextFromContent(item.content)
    })
    const lastMessageText = lastUsefulUserText(messageTexts)
    if (lastMessageText) return lastMessageText

    const contents = Array.isArray(parsed.contents) ? parsed.contents : []
    const contentTexts = contents.flatMap((content) => {
      if (!content || typeof content !== 'object') return []
      const item = content as Record<string, unknown>
      if (item.role !== 'user') return []
      return extractTextFromContent(item.parts)
    })
    const lastContentText = lastUsefulUserText(contentTexts)
    if (lastContentText) return lastContentText

    const promptTexts = extractTextFromContent(parsed.prompt)
    const lastPromptText = lastUsefulUserText(promptTexts)
    if (lastPromptText) return lastPromptText

    const sessionText = extractSessionTagText(raw)
    if (sessionText) return sessionText
  } catch {
    const lastTextField = lastUsefulUserText([
      ...extractEscapedJSONUserTextFields(raw),
      ...extractEscapedJSONTextFields(raw),
      ...extractEscapedJSONContentFields(raw),
    ])
    if (lastTextField) return lastTextField
    const sessionText = extractSessionTagText(raw)
    if (sessionText) return sessionText
  }
  const lastTextField = lastUsefulUserText([
    ...extractEscapedJSONUserTextFields(raw),
    ...extractEscapedJSONTextFields(raw),
    ...extractEscapedJSONContentFields(raw),
  ])
  if (lastTextField) return lastTextField
  return cleanAuditUserText(raw)
}

function parseAuditContentItems(value: string): AuditContentItem[] | null {
  if (!value) return null
  try {
    const parsed = JSON.parse(value)
    if (Array.isArray(parsed) && parsed.every((item) => item && typeof item === 'object' && 'content' in item)) {
      return parsed as AuditContentItem[]
    }
  } catch {
    return null
  }
  return null
}

function buildAuditTurns(item: AuditLog): AuditTurn[] {
  const requests = parseAuditContentItems(item.request_body)
  const responses = parseAuditContentItems(item.response_body)
  if (!requests || !responses) {
    return [{
      key: item.request_id || String(item.id),
      title: `#1 ${formatTime(item.created_at)} ${item.model || '-'}`,
      request: extractUserInputText(item.request_body),
      response: prettifyJSON(item.response_body),
      requestTruncated: item.request_truncated,
      responseTruncated: item.response_truncated,
    }]
  }
  const count = Math.max(requests.length, responses.length)
  return Array.from({ length: count }, (_, index) => {
    const request = requests[index]
    const response = responses[index]
    const createdAt = request?.created_at || response?.created_at || item.created_at
    const model = request?.model || response?.model || item.model || '-'
    const requestID = request?.request_id || response?.request_id || item.request_id || String(index)
    return {
      key: `${requestID}-${index}`,
      title: `#${index + 1} ${formatTime(createdAt || '')} ${model}`,
      request: extractUserInputText(request?.content),
      response: contentToText(response?.content),
      requestTruncated: Boolean(request?.truncated),
      responseTruncated: Boolean(response?.truncated),
    }
  })
}

function mapAuditLog(item: AuditLog): AuditRow {
  return {
    id: String(item.id),
    time: formatTime(item.updated_at || item.created_at),
    user: item.user_email || '-',
    apiKey: item.api_key_name || (item.api_key_id ? `#${item.api_key_id}` : '-'),
    account: item.group_name || (item.group_id ? `#${item.group_id}` : '-'),
    platform: item.platform || '-',
    model: item.model || '-',
    endpoint: item.endpoint || '-',
    sessionId: item.session_id || item.request_id || '-',
    requestCount: item.request_count || 1,
    statusCode: item.status_code,
    requestId: item.request_id || '-',
    latency: `${item.duration_ms} ms`,
    ipAddress: item.ip_address || '-',
    requestTruncated: item.request_truncated,
    responseTruncated: item.response_truncated,
    turns: buildAuditTurns(item),
  }
}

async function loadRows() {
  activeController?.abort()
  const controller = new AbortController()
  activeController = controller
  loading.value = true
  errorMessage.value = ''
  try {
    const result = await auditAPI.list({
      page: pagination.page,
      page_size: pagination.pageSize,
      search: filters.keyword.trim() || undefined,
      platform: filters.platform === 'all' ? undefined : filters.platform,
      model: filters.model.trim() || undefined,
      endpoint: filters.endpoint.trim() || undefined,
    }, { signal: controller.signal })
    rows.value = result.items.map(mapAuditLog)
    pagination.total = result.total
    pagination.page = result.page
    pagination.pageSize = result.page_size
    pagination.pages = result.pages
    selectedRow.value = rows.value.find((row) => row.id === selectedRow.value?.id) || rows.value[0] || null
  } catch (error: any) {
    if (error?.code === 'ERR_CANCELED') return
    errorMessage.value = error?.response?.data?.message || error?.message || t('admin.audit.loadFailed')
  } finally {
    if (activeController === controller) {
      activeController = null
      loading.value = false
    }
  }
}

function goPage(page: number) {
  pagination.page = Math.min(Math.max(1, page), pagination.pages)
  loadRows()
}

function goTurnPage(page: number) {
  turnPagination.page = Math.min(Math.max(1, page), turnPages.value)
}

let filterTimer: ReturnType<typeof setTimeout> | undefined
watch(filters, () => {
  if (filterTimer) clearTimeout(filterTimer)
  filterTimer = setTimeout(() => {
    pagination.page = 1
    loadRows()
  }, 300)
}, { deep: true })

watch(() => selectedRow.value?.id, () => {
  turnPagination.page = 1
})

watch(turnPages, (pages) => {
  if (turnPagination.page > pages) {
    turnPagination.page = pages
  }
})

onMounted(loadRows)

onBeforeUnmount(() => {
  activeController?.abort()
  if (filterTimer) clearTimeout(filterTimer)
})
</script>
