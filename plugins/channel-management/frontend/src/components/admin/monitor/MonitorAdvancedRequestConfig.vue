<template>
  <div class="space-y-4">
    <!-- Headers key-value rows -->
    <div>
      <label class="input-label">{{ t('admin.channelMonitor.advanced.headers') }}</label>
      <div class="space-y-1.5">
        <div
          v-for="(row, i) in headerRows"
          :key="i"
          class="flex items-center gap-2"
        >
          <input
            v-model="row.name"
            type="text"
            spellcheck="false"
            :placeholder="t('admin.channelMonitor.advanced.headerNamePlaceholder')"
            class="input w-52 flex-none font-mono text-xs"
            @blur="commitHeaders"
          />
          <input
            v-model="row.value"
            type="text"
            spellcheck="false"
            :placeholder="t('admin.channelMonitor.advanced.headerValuePlaceholder')"
            class="input flex-1 font-mono text-xs"
            @blur="commitHeaders"
          />
          <button
            type="button"
            class="hover-tint-danger flex-none rounded p-1"
            :title="t('common.delete')"
            @click="removeRow(i)"
          >
            <svg class="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
            </svg>
          </button>
        </div>
        <button
          type="button"
          class="inline-flex items-center gap-1 rounded border border-dashed border-gray-300 px-2 py-1 text-xs text-gray-500 hover:border-primary-400 hover:text-primary-600 dark:border-dark-600 dark:text-gray-400 dark:hover:border-primary-500 dark:hover:text-primary-400"
          @click="addRow"
        >
          <svg class="h-3.5 w-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 4v16m8-8H4" />
          </svg>
          {{ t('admin.channelMonitor.advanced.headerAddRow') }}
        </button>
      </div>
      <p v-if="headersError" class="text-semantic-danger mt-1 text-xs">{{ headersError }}</p>
      <p v-else class="mt-1 text-xs text-gray-400">
        {{ t('admin.channelMonitor.advanced.headersHint') }}
      </p>
    </div>

    <!-- Body mode radio -->
    <div>
      <label class="input-label">{{ t('admin.channelMonitor.advanced.bodyMode') }}</label>
      <div class="grid grid-cols-3 gap-3">
        <button
          v-for="opt in bodyModeOptions"
          :key="opt.value"
          type="button"
          class="rounded-lg border-2 px-3 py-2 text-sm font-medium transition-colors"
          :class="bodyModeButtonClass(opt.value)"
          @click="updateBodyMode(opt.value)"
        >
          {{ opt.label }}
        </button>
      </div>
      <p class="mt-1 text-xs text-gray-400">
        {{ bodyModeHint }}
      </p>
    </div>

    <!-- Body JSON (仅当 mode != off) -->
    <div v-if="bodyOverrideMode !== 'off'">
      <div class="mb-1 flex items-center justify-between">
        <label class="input-label !mb-0">{{ t('admin.channelMonitor.advanced.bodyJson') }}</label>
      </div>
      <textarea
        v-model="bodyText"
        rows="10"
        :placeholder="bodyPlaceholder"
        class="input font-mono text-xs"
        style="white-space: pre; overflow-wrap: normal; overflow-x: auto;"
        spellcheck="false"
        @blur="commitBody"
      />
      <p v-if="bodyError" class="text-semantic-danger mt-1 text-xs">{{ bodyError }}</p>
      <p v-else class="mt-1 text-xs text-gray-400">
        {{ t('admin.channelMonitor.advanced.bodyJsonHint') }}
      </p>
    </div>
  </div>
</template>

<script setup lang="ts">
/**
 * Worker B' — Advanced request config (headers KV rows + body override mode).
 *
 * Ported from host @ v0.1.121
 * (frontend/src/components/admin/monitor/MonitorAdvancedRequestConfig.vue,
 * commit e1193212b introduced KV-row headers).
 *
 * 端口差异 vs host v0.1.121:
 *   - i18n key 完全沿用 `admin.channelMonitor.advanced.*`，但在 plugin i18n
 *     文件里独立声明（不依赖 host 定义）
 *   - BodyOverrideMode 类型从 plugin 的 `../../../api/admin/channelMonitor` 取
 *   - 删除 host 里 "Format JSON" 按钮（ADR §5 决定 a7415d4d2 暂不移植）
 *   - "commit 同值不回写" 约束保留，避免 commit 后行被重排
 */
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import type { BodyOverrideMode } from '../../../api/admin/channelMonitor'

const props = defineProps<{
  extraHeaders: Record<string, string>
  bodyOverrideMode: BodyOverrideMode
  bodyOverride: Record<string, unknown> | null
}>()

const emit = defineEmits<{
  (e: 'update:extraHeaders', value: Record<string, string>): void
  (e: 'update:bodyOverrideMode', value: BodyOverrideMode): void
  (e: 'update:bodyOverride', value: Record<string, unknown> | null): void
}>()

const { t } = useI18n()

// ---- Headers key-value rows ----
interface HeaderRow {
  name: string
  value: string
}

const headerRows = ref<HeaderRow[]>(toRows(props.extraHeaders))
const headersError = ref('')

watch(
  () => props.extraHeaders,
  (v) => {
    // 外部重置时（切换平台 / 应用模板）同步行。
    // 同值不回写，避免每次 commit 都把行重排。
    if (!isSameHeaderMap(toMap(headerRows.value), v)) {
      headerRows.value = toRows(v)
    }
    headersError.value = ''
  },
)

function toRows(h: Record<string, string>): HeaderRow[] {
  const entries = Object.entries(h || {})
  if (entries.length === 0) return [{ name: '', value: '' }]
  return entries.map(([name, value]) => ({ name, value }))
}

function toMap(rows: HeaderRow[]): Record<string, string> {
  const out: Record<string, string> = {}
  for (const row of rows) {
    const name = row.name.trim()
    if (name === '') continue
    out[name] = row.value
  }
  return out
}

function isSameHeaderMap(a: Record<string, string>, b: Record<string, string>): boolean {
  const ak = Object.keys(a)
  const bk = Object.keys(b || {})
  if (ak.length !== bk.length) return false
  for (const k of ak) {
    if (a[k] !== (b || {})[k]) return false
  }
  return true
}

function commitHeaders() {
  // 空白 name + 空白 value 的行允许保留作为"占位新行"，不报错；
  // name 非空但 value 为空（或反之）都视为用户正在编辑，同样不报错。
  // 只在 name 里含冒号 / 空白这种明显不合法时兜一下。
  for (const row of headerRows.value) {
    const name = row.name.trim()
    if (name === '') continue
    if (name.includes(':') || /\s/.test(name)) {
      headersError.value = t('admin.channelMonitor.advanced.headerNameInvalid', { name })
      return
    }
  }
  headersError.value = ''
  emit('update:extraHeaders', toMap(headerRows.value))
}

function addRow() {
  headerRows.value.push({ name: '', value: '' })
}

function removeRow(index: number) {
  headerRows.value.splice(index, 1)
  if (headerRows.value.length === 0) {
    headerRows.value.push({ name: '', value: '' })
  }
  commitHeaders()
}

// ---- Body mode + JSON ----
const bodyText = ref(serializeBody(props.bodyOverride))
const bodyError = ref('')

watch(
  () => props.bodyOverride,
  (v) => {
    bodyText.value = serializeBody(v)
    bodyError.value = ''
  },
)

function commitBody() {
  if (props.bodyOverrideMode === 'off') {
    return
  }
  const trimmed = bodyText.value.trim()
  if (trimmed === '') {
    emit('update:bodyOverride', null)
    bodyError.value = ''
    return
  }
  try {
    const parsed = JSON.parse(trimmed)
    if (parsed === null || typeof parsed !== 'object' || Array.isArray(parsed)) {
      bodyError.value = t('admin.channelMonitor.advanced.bodyJsonObjectError')
      return
    }
    emit('update:bodyOverride', parsed as Record<string, unknown>)
    bodyError.value = ''
  } catch (e: unknown) {
    bodyError.value =
      t('admin.channelMonitor.advanced.bodyJsonError') +
      ': ' +
      (e instanceof Error ? e.message : String(e))
  }
}

function serializeBody(body: Record<string, unknown> | null): string {
  if (!body || Object.keys(body).length === 0) return ''
  return JSON.stringify(body, null, 2)
}

function updateBodyMode(mode: BodyOverrideMode) {
  emit('update:bodyOverrideMode', mode)
  // 切换到 off 时清掉 body（提示用户）
  if (mode === 'off') {
    emit('update:bodyOverride', null)
  }
}

const bodyModeOptions = computed<{ value: BodyOverrideMode; label: string }[]>(() => [
  { value: 'off', label: t('admin.channelMonitor.advanced.bodyModeOff') },
  { value: 'merge', label: t('admin.channelMonitor.advanced.bodyModeMerge') },
  { value: 'replace', label: t('admin.channelMonitor.advanced.bodyModeReplace') },
])

function bodyModeButtonClass(mode: BodyOverrideMode): string {
  const active = props.bodyOverrideMode === mode
  if (active) {
    return 'border-primary-500 bg-primary-50 text-primary-700 dark:bg-primary-500/15 dark:text-primary-300 dark:border-primary-400'
  }
  return 'border-gray-200 bg-white text-gray-600 hover:border-primary-300 dark:border-dark-700 dark:bg-dark-800 dark:text-gray-400'
}

const bodyModeHint = computed(() => {
  switch (props.bodyOverrideMode) {
    case 'merge':
      return t('admin.channelMonitor.advanced.bodyModeHintMerge')
    case 'replace':
      return t('admin.channelMonitor.advanced.bodyModeHintReplace')
    default:
      return t('admin.channelMonitor.advanced.bodyModeHintOff')
  }
})

const bodyPlaceholder = computed(() => {
  if (props.bodyOverrideMode === 'merge') {
    return '{\n  "system": "You are Claude Code..."\n}'
  }
  return '{\n  "model": "claude-x",\n  "messages": [{"role":"user","content":"hi"}],\n  "max_tokens": 10\n}'
})
</script>
