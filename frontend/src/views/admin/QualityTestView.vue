<template>
  <AppLayout>
    <div class="space-y-4">
      <section class="card space-y-4 p-4 sm:p-5">
        <div class="flex flex-wrap items-end gap-3">
          <div class="w-full sm:w-44">
            <label class="input-label">{{ t('admin.qualityTest.platform') }}</label>
            <Select v-model="platform" :options="platformOptions" :disabled="running" />
          </div>
          <div class="w-full sm:w-44">
            <label class="input-label">{{ t('admin.qualityTest.reasoningEffort') }}</label>
            <Select v-model="effort" :options="effortOptions" :disabled="running" />
          </div>
          <div class="flex flex-1 justify-end gap-2">
            <button v-if="running" class="btn btn-secondary" @click="stopAll">
              {{ t('admin.qualityTest.stop') }}
            </button>
            <button class="btn btn-primary" :disabled="!canRun" @click="runAll">
              <Icon name="play" size="md" class="mr-2" />
              {{ t('admin.qualityTest.run') }}
            </button>
          </div>
        </div>

        <div class="grid gap-3 md:grid-cols-3">
          <div v-for="(slot, index) in slots" :key="index" class="space-y-2">
            <label class="input-label">{{ t('admin.qualityTest.slot', { n: index + 1 }) }}</label>
            <Select
              v-model="slot.accountId"
              :options="accountOptions"
              :placeholder="t('admin.qualityTest.selectAccount')"
              :disabled="running"
              searchable
              clearable
              @change="loadModels(slot)"
            />
            <Select
              v-model="slot.model"
              :options="slot.models"
              :placeholder="t('admin.qualityTest.selectModel')"
              :disabled="running || !slot.accountId"
              searchable
              creatable
            />
          </div>
        </div>

        <div>
          <label class="input-label">{{ t('admin.qualityTest.prompt') }}</label>
          <textarea v-model="prompt" rows="4" class="input font-mono text-meta" :disabled="running" maxlength="16000" />
        </div>
      </section>

      <div class="grid gap-3 md:grid-cols-3">
        <section v-for="(slot, index) in activeSlots" :key="index" class="card flex min-h-[420px] flex-col">
          <div class="card-header flex items-center justify-between gap-2">
            <div class="min-w-0">
              <div class="truncate font-semibold text-fg">{{ accountName(slot.accountId) }}</div>
              <div class="truncate text-meta text-fg-subtle">{{ slot.model }}</div>
            </div>
            <span :class="['badge', statusBadge(slot.status)]">{{ t(`admin.qualityTest.status.${slot.status}`) }}</span>
          </div>
          <div class="flex flex-wrap gap-x-3 border-b border-border px-4 py-1.5 text-meta tabular-nums text-fg-muted">
            <span>TTFT {{ slot.ttftMs ?? '-' }} ms</span>
            <span>{{ t('admin.qualityTest.duration') }} {{ slot.durationMs ?? '-' }} ms</span>
            <span>{{ t('admin.qualityTest.chars', { n: slot.output.length }) }}</span>
            <button class="ml-auto text-accent" @click="slot.showSource = !slot.showSource">
              {{ slot.showSource ? t('admin.qualityTest.preview') : t('admin.qualityTest.source') }}
            </button>
          </div>
          <div v-if="slot.error" class="border-b border-danger/40 bg-danger-weak px-4 py-2 text-meta text-danger-strong">
            {{ slot.error }}
          </div>
          <pre v-if="slot.showSource || slot.status === 'running'" class="flex-1 overflow-auto whitespace-pre-wrap break-all p-3 text-meta">{{ slot.output }}</pre>
          <!-- 无 allow-same-origin：预览内容无法访问管理端 cookie/存储 -->
          <iframe v-else class="w-full flex-1 bg-white" sandbox="allow-scripts" :title="t('admin.qualityTest.preview')" :srcdoc="extractHtml(slot.output)" />
        </section>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import Select from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'
import { adminAPI } from '@/api/admin'
import { buildApiUrl } from '@/api/client'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import { ADMIN_UI_REQUEST_HEADER } from '@/api/adminUIRequest'
import { extractHtml } from './qualityTestHtml'

type SlotStatus = 'idle' | 'running' | 'completed' | 'error' | 'stopped'

interface TestSlot {
  accountId: number | null
  model: string
  models: { value: string; label: string }[]
  status: SlotStatus
  output: string
  error: string
  ttftMs: number | null
  durationMs: number | null
  showSource: boolean
  controller: AbortController | null
}

const { t } = useI18n()
const appStore = useAppStore()

const DEFAULT_PROMPT =
  'Create a single self-contained HTML document (inline SVG/CSS/JS, no external resources) showing an animated pelican riding a bicycle. Return only the HTML.'

// ponytail: 结果不持久化；需要跨会话/多管理员共享历史时再加表
const platform = ref<'openai' | 'anthropic'>('openai')
const effort = ref('')
const prompt = ref(DEFAULT_PROMPT)
const accounts = ref<{ id: number; name: string }[]>([])
const slots = reactive<TestSlot[]>([0, 1, 2].map(() => newSlot()))

const platformOptions = [
  { value: 'openai', label: 'OpenAI' },
  { value: 'anthropic', label: 'Anthropic' }
]
// 与后端 qualityTest*Efforts 保持一致
const effortOptions = computed(() =>
  (platform.value === 'openai' ? ['', 'none', 'minimal', 'low', 'medium', 'high', 'xhigh'] : ['', 'low', 'medium', 'high', 'max']).map(
    (value) => ({ value, label: value || t('admin.qualityTest.effortDefault') })
  )
)
const accountOptions = computed(() => accounts.value.map((a) => ({ value: a.id, label: `${a.name} (#${a.id})` })))
const activeSlots = computed(() => slots.filter((s) => s.status !== 'idle'))
const running = computed(() => slots.some((s) => s.status === 'running'))
const canRun = computed(() => !running.value && prompt.value.trim() !== '' && slots.some((s) => s.accountId && s.model))

function newSlot(): TestSlot {
  return { accountId: null, model: '', models: [], status: 'idle', output: '', error: '', ttftMs: null, durationMs: null, showSource: false, controller: null }
}

async function loadAccounts() {
  const current = platform.value
  try {
    // 1000 = backend max page_size; keep paging so accounts past the first page stay selectable
    const all: { id: number; name: string }[] = []
    for (let page = 1; ; page++) {
      const res = await adminAPI.accounts.list(page, 1000, { platform: current, lite: '1' })
      all.push(...res.items.map((a) => ({ id: a.id, name: a.name })))
      if (res.items.length === 0 || all.length >= res.total) break
    }
    if (platform.value === current) accounts.value = all
  } catch (err) {
    if (platform.value === current) appStore.showError(extractApiErrorMessage(err, t('admin.qualityTest.loadAccountsFailed')))
  }
}

async function loadModels(slot: TestSlot) {
  slot.model = ''
  slot.models = []
  if (!slot.accountId) return
  try {
    const models = await adminAPI.accounts.getAvailableModels(slot.accountId)
    slot.models = models.map((m) => ({ value: m.id, label: m.display_name || m.id }))
  } catch (err) {
    appStore.showError(extractApiErrorMessage(err, t('admin.qualityTest.loadModelsFailed')))
  }
}

function accountName(id: number | null) {
  return accounts.value.find((a) => a.id === id)?.name ?? `#${id}`
}

function statusBadge(status: SlotStatus) {
  return { idle: 'badge-gray', running: 'badge-warning', completed: 'badge-success', error: 'badge-danger', stopped: 'badge-gray' }[status]
}

async function runSlot(slot: TestSlot) {
  Object.assign(slot, { status: 'running', output: '', error: '', ttftMs: null, durationMs: null, showSource: false })
  slot.controller = new AbortController()
  const startedAt = performance.now()
  try {
    const response = await fetch(buildApiUrl(`/admin/accounts/${slot.accountId}/test`), {
      method: 'POST',
      headers: {
        Authorization: `Bearer ${localStorage.getItem('auth_token')}`,
        'Content-Type': 'application/json',
        [ADMIN_UI_REQUEST_HEADER]: '1'
      },
      body: JSON.stringify({ model_id: slot.model, prompt: prompt.value, mode: 'quality', reasoning_effort: effort.value }),
      signal: slot.controller.signal
    })
    if (!response.ok || !response.body) throw new Error(`HTTP ${response.status}`)
    const reader = response.body.getReader()
    const decoder = new TextDecoder()
    let buffer = ''
    for (;;) {
      const { done, value } = await reader.read()
      if (done) break
      buffer += decoder.decode(value, { stream: true })
      const lines = buffer.split('\n')
      buffer = lines.pop() || ''
      for (const line of lines) {
        if (!line.startsWith('data: ')) continue
        let event: { type: string; text?: string; error?: string; success?: boolean }
        try {
          event = JSON.parse(line.slice(6))
        } catch {
          continue
        }
        if (event.type === 'content' && event.text) {
          slot.ttftMs ??= Math.round(performance.now() - startedAt)
          slot.output += event.text
        } else if (event.type === 'error') {
          slot.error = event.error || t('common.unknownError')
        } else if (event.type === 'test_complete' && event.success) {
          slot.status = 'completed'
        }
      }
    }
    if (slot.status === 'running') slot.status = 'error'
  } catch (err) {
    if (err instanceof DOMException && err.name === 'AbortError') {
      slot.status = 'stopped'
    } else {
      slot.status = 'error'
      slot.error = err instanceof Error ? err.message : t('common.unknownError')
    }
  } finally {
    slot.durationMs = Math.round(performance.now() - startedAt)
    slot.controller = null
  }
}

function runAll() {
  for (const slot of slots) {
    if (slot.accountId && slot.model) void runSlot(slot)
    else slot.status = 'idle'
  }
}

function stopAll() {
  slots.forEach((s) => s.controller?.abort())
}

watch(platform, () => {
  slots.splice(0, slots.length, ...[0, 1, 2].map(() => newSlot()))
  effort.value = ''
  void loadAccounts()
})

onMounted(loadAccounts)
onBeforeUnmount(stopAll)
</script>
