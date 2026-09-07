<script setup lang="ts">
import { computed, ref, watch, onMounted, onUnmounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useClipboard } from '@/composables/useClipboard'
import { adminUsageAPI } from '@/api/admin/usage'
import { useAppStore } from '@/stores/app'
import JsonTreeViewer from './JsonTreeViewer.vue'
import { formatHeaders } from '../utils/jsonFormat'
import { extractDialogTurns, extractImagesFromMarkdown } from '../utils/dialogExtract'
import type {
  DiagnosisPrimaryTab,
  MoreResponsePart,
  MoreSide,
  UpstreamSubTab,
  UsageDiagnosisDetail
} from '../types'

const props = defineProps<{
  show: boolean
  usageId?: number | null
}>()

const emit = defineEmits<{
  (e: 'update:show', v: boolean): void
  (e: 'close'): void
}>()

const { t } = useI18n()
const appStore = useAppStore()
const { copyToClipboard } = useClipboard()

const loading = ref(false)
const detail = ref<UsageDiagnosisDetail | null>(null)
const primaryTab = ref<DiagnosisPrimaryTab>('overview')
const upstreamTab = ref<UpstreamSubTab>('overview')
const moreSide = ref<MoreSide>('request')
const morePart = ref<MoreResponsePart>('thinking')
const requestTurnIndex = ref(0)
const lightboxSrc = ref<string | null>(null)

const modalWidth = ref(960)
const modalHeight = ref(720)
const resizing = ref(false)

const turns = computed(() => {
  if (!detail.value) return []
  const base = extractDialogTurns(detail.value.req_body, detail.value.res_body, detail.value.dialog)
  return base.map((turn) => {
    const mdImgs = extractImagesFromMarkdown(turn.text || '')
    return { ...turn, images: [...turn.images, ...mdImgs] }
  })
})

const requestTurns = computed(() => turns.value.filter((x) => x.role === 'system' || x.role === 'user'))
const thinkingTurns = computed(() => turns.value.filter((x) => x.role === 'thinking'))
const replyTurns = computed(() => turns.value.filter((x) => x.role === 'assistant'))

const subtitle = computed(() => {
  if (!detail.value) return ''
  const id = (detail.value.request_id || String(detail.value.id || '')).slice(0, 12)
  const ip = detail.value.client_ip || '-'
  const path = detail.value.path || '-'
  const time = formatTime(detail.value.created_at)
  return `${id} · ${ip} · ${path} · ${time}`
})

const statusCode = computed(() => detail.value?.status_code || detail.value?.upstream_status || 0)
const statusOk = computed(() => statusCode.value > 0 && statusCode.value < 400)

function formatTime(v?: string) {
  if (!v) return '-'
  const d = new Date(v)
  if (Number.isNaN(d.getTime())) return v
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`
}

function formatDuration(ms?: number | null) {
  if (ms == null) return '-'
  return `${ms.toLocaleString()} ms`
}

function formatTtft(ms?: number | null) {
  if (ms == null) return '-'
  return `${(ms / 1000).toFixed(2)}s`
}

function close() {
  emit('update:show', false)
  emit('close')
}

async function load() {
  if (!props.show || !props.usageId) return
  loading.value = true
  detail.value = null
  primaryTab.value = 'overview'
  upstreamTab.value = 'overview'
  try {
    detail.value = await adminUsageAPI.getDiagnosis(props.usageId)
    // default More side
    if (!requestTurns.value.length && (replyTurns.value.length || thinkingTurns.value.length)) {
      moreSide.value = 'response'
    } else {
      moreSide.value = 'request'
    }
    morePart.value = thinkingTurns.value.length ? 'thinking' : 'reply'
    requestTurnIndex.value = 0
  } catch (e: any) {
    appStore.showError(e?.message || t('usage.diagnosis.loadFailed'))
    close()
  } finally {
    loading.value = false
  }
}

watch(
  () => [props.show, props.usageId] as const,
  ([show]) => {
    if (show) load()
  }
)

function onKey(e: KeyboardEvent) {
  if (!props.show) return
  if (e.key === 'Escape') {
    if (lightboxSrc.value) lightboxSrc.value = null
    else close()
  }
}

onMounted(() => window.addEventListener('keydown', onKey))
onUnmounted(() => window.removeEventListener('keydown', onKey))

function roleName(role: string) {
  if (role === 'system') return t('usage.diagnosis.roleSystem')
  if (role === 'user') return t('usage.diagnosis.roleUser')
  if (role === 'assistant') return t('usage.diagnosis.roleAssistant')
  if (role === 'thinking') return t('usage.diagnosis.roleThinking')
  return role
}

async function copyText(text: string) {
  await copyToClipboard(text, t('usage.diagnosis.copied'))
}

function downloadMedia(src: string, name: string) {
  const a = document.createElement('a')
  a.href = src
  a.download = name || 'download'
  if (!src.startsWith('data:')) a.target = '_blank'
  a.click()
}

function downloadAudit() {
  if (!detail.value) return
  const files: Array<{ name: string; content: string }> = []
  for (const turn of turns.value) {
    for (const img of turn.images) {
      if (img.dataUrl) files.push({ name: img.name, content: img.dataUrl })
    }
    for (const f of turn.files) {
      if (f.dataUrl) files.push({ name: f.name, content: f.dataUrl })
    }
  }
  const payload = {
    ...detail.value,
    extracted_turns: turns.value.map((t) => ({
      role: t.role,
      text: t.text,
      images: t.images.map((i) => ({ name: i.name, mime: i.mime, has_data: !!i.dataUrl, url: i.url })),
      files: t.files.map((i) => ({ name: i.name, mime: i.mime, has_data: !!i.dataUrl, url: i.url }))
    })),
    embedded_files: files
  }
  const blob = new Blob([JSON.stringify(payload, null, 2)], { type: 'application/json' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = `audit-${detail.value.id || detail.value.request_id}.json`
  a.click()
  URL.revokeObjectURL(url)
}

function startResize(e: MouseEvent) {
  resizing.value = true
  const startX = e.clientX
  const startY = e.clientY
  const startW = modalWidth.value
  const startH = modalHeight.value
  const onMove = (ev: MouseEvent) => {
    modalWidth.value = Math.max(640, startW + (ev.clientX - startX))
    modalHeight.value = Math.max(480, startH + (ev.clientY - startY))
  }
  const onUp = () => {
    resizing.value = false
    window.removeEventListener('mousemove', onMove)
    window.removeEventListener('mouseup', onUp)
  }
  window.addEventListener('mousemove', onMove)
  window.addEventListener('mouseup', onUp)
}

const overviewCards = computed(() => {
  const d = detail.value
  if (!d) return []
  return [
    { label: t('usage.diagnosis.requestModel'), value: d.requested_model || '-' },
    { label: t('usage.diagnosis.upstreamModel'), value: d.upstream_model || '-' },
    { label: t('usage.diagnosis.clientKey'), value: d.api_key_name || '-' },
    { label: t('usage.diagnosis.clientIp'), value: d.client_ip || '-' },
    { label: t('usage.diagnosis.duration'), value: formatDuration(d.duration_ms) },
    { label: t('usage.diagnosis.ttft'), value: formatTtft(d.first_token_ms) },
    { label: t('usage.diagnosis.inputTokens'), value: String(d.input_tokens ?? 0) },
    { label: t('usage.diagnosis.outputTokens'), value: String(d.output_tokens ?? 0) },
    { label: t('usage.diagnosis.cachedTokens'), value: String(d.cache_read_tokens ?? 0) },
    { label: t('usage.diagnosis.amount'), value: (d.actual_cost ?? d.total_cost ?? 0).toString() },
    { label: t('usage.diagnosis.streaming'), value: d.stream ? t('common.yes') : t('common.no') },
    { label: t('usage.diagnosis.group'), value: d.group_name || '-' }
  ]
})

const moreEmpty = computed(() => {
  if (moreSide.value === 'request') return requestTurns.value.length === 0
  return !thinkingTurns.value.length && !replyTurns.value.length
})
</script>

<template>
  <Teleport to="body">
    <div v-if="show" class="fixed inset-0 z-[80] flex items-center justify-center bg-black/60 p-4" @click.self="close">
      <div
        class="relative flex flex-col overflow-hidden rounded-2xl border border-gray-200 bg-white shadow-2xl dark:border-dark-700 dark:bg-dark-900"
        :style="{ width: modalWidth + 'px', height: modalHeight + 'px', maxWidth: '95vw', maxHeight: '92vh' }"
        data-testid="request-diagnosis-modal"
      >
        <!-- Header -->
        <div class="flex items-start justify-between gap-3 border-b border-gray-200 px-5 py-4 dark:border-dark-700">
          <div class="min-w-0">
            <div class="flex items-center gap-2">
              <h3 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('usage.diagnosis.title') }}</h3>
              <span
                v-if="statusCode"
                class="rounded-full px-2 py-0.5 text-xs font-bold"
                :class="statusOk ? 'bg-emerald-500/20 text-emerald-400' : 'bg-rose-500/20 text-rose-400'"
              >
                {{ statusCode }}
              </span>
            </div>
            <div class="mt-1 truncate font-mono text-xs text-gray-500 dark:text-gray-400">{{ subtitle }}</div>
          </div>
          <button type="button" class="btn btn-secondary btn-sm" @click="close">{{ t('usage.diagnosis.close') }}</button>
        </div>

        <!-- Tabs -->
        <div class="flex gap-1 border-b border-gray-200 px-4 dark:border-dark-700">
          <button
            v-for="tab in [
              { key: 'overview', label: t('usage.diagnosis.tabOverview') },
              { key: 'request', label: t('usage.diagnosis.tabRequest') },
              { key: 'upstream', label: t('usage.diagnosis.tabUpstream') }
            ]"
            :key="tab.key"
            type="button"
            class="-mb-px border-b-2 px-3 py-2.5 text-sm font-medium"
            :class="primaryTab === tab.key ? 'border-primary-500 text-primary-500' : 'border-transparent text-gray-500 hover:text-gray-800 dark:hover:text-gray-200'"
            @click="primaryTab = tab.key as DiagnosisPrimaryTab"
          >
            {{ tab.label }}
            <span
              v-if="tab.key === 'upstream'"
              class="ml-1 inline-flex h-5 min-w-5 items-center justify-center rounded-full bg-gray-200 px-1 text-[10px] text-gray-700 dark:bg-dark-700 dark:text-gray-200"
            >1</span>
          </button>
        </div>

        <!-- Body -->
        <div class="min-h-0 flex-1 overflow-auto p-4">
          <div v-if="loading" class="flex h-full items-center justify-center text-sm text-gray-500">{{ t('common.loading') }}</div>
          <template v-else-if="detail">
            <!-- Overview -->
            <div v-if="primaryTab === 'overview'" class="space-y-4">
              <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
                <div v-for="card in overviewCards" :key="card.label" class="rounded-xl bg-gray-50 p-3 dark:bg-dark-800">
                  <div class="text-xs text-gray-500 dark:text-gray-400">{{ card.label }}</div>
                  <div class="mt-1 break-all text-sm font-semibold text-gray-900 dark:text-white">{{ card.value }}</div>
                </div>
              </div>
              <div class="space-y-3">
                <div v-for="(turn, idx) in turns" :key="idx" class="rounded-xl border border-gray-200 p-3 dark:border-dark-700">
                  <div class="mb-2 text-xs font-semibold uppercase tracking-wide text-gray-500">{{ roleName(turn.role) }}</div>
                  <div class="whitespace-pre-wrap text-sm text-gray-800 dark:text-gray-100">{{ turn.text || '—' }}</div>
                  <div v-if="turn.images.length || turn.files.length" class="mt-3 flex flex-wrap gap-2">
                    <button
                      v-for="img in turn.images"
                      :key="img.id"
                      type="button"
                      class="rounded-lg border border-gray-200 p-2 text-left dark:border-dark-600"
                      @click="lightboxSrc = img.dataUrl || img.url || null"
                    >
                      <img v-if="img.dataUrl || img.url" :src="img.dataUrl || img.url" class="h-20 w-28 rounded object-cover" alt="" />
                      <div class="mt-1 text-[10px] text-gray-500">{{ img.name }} · {{ img.mime || 'image' }}</div>
                    </button>
                    <div v-for="f in turn.files" :key="f.id" class="rounded-lg border border-gray-200 p-2 dark:border-dark-600">
                      <div class="text-xs font-medium">{{ f.name }}</div>
                      <div class="text-[10px] text-gray-500">{{ f.mime || 'file' }}</div>
                      <button
                        v-if="f.dataUrl || f.url"
                        type="button"
                        class="mt-1 text-[10px] text-primary-500 underline"
                        @click="downloadMedia(f.dataUrl || f.url!, f.name)"
                      >{{ t('usage.diagnosis.download') }}</button>
                    </div>
                  </div>
                </div>
              </div>
            </div>

            <!-- Request info -->
            <div v-else-if="primaryTab === 'request'" class="space-y-4">
              <div class="flex items-center justify-between rounded-xl bg-gray-50 px-3 py-2 font-mono text-sm dark:bg-dark-800">
                <span>{{ detail.method || 'POST' }} {{ detail.path || '/' }}</span>
                <button type="button" class="text-xs text-primary-500" @click="copyText(`${detail.method || 'POST'} ${detail.path || '/'}`)">{{ t('usage.diagnosis.copy') }}</button>
              </div>
              <div>
                <div class="mb-2 text-xs font-semibold uppercase text-gray-500">{{ t('usage.diagnosis.headers') }}</div>
                <pre class="max-h-48 overflow-auto rounded-xl border border-gray-200 bg-gray-50 p-3 font-mono text-xs dark:border-dark-700 dark:bg-dark-800">{{ formatHeaders(detail.req_headers) || t('usage.diagnosis.emptyHeaders') }}</pre>
              </div>
              <JsonTreeViewer :title="t('usage.diagnosis.requestBodyJson')" :raw="detail.req_body" :empty-text="t('usage.diagnosis.emptyBody')" />
            </div>

            <!-- Upstream -->
            <div v-else class="flex min-h-0 gap-3">
              <div class="w-36 shrink-0">
                <div class="mb-2 text-xs text-gray-500">{{ t('usage.diagnosis.attempts') }}</div>
                <div class="rounded-xl border border-primary-500/50 bg-primary-500/10 p-3">
                  <div class="flex items-center justify-between text-sm font-semibold">
                    <span>{{ t('usage.diagnosis.attempt', { n: 1 }) }}</span>
                    <span class="rounded-full bg-emerald-500/20 px-1.5 text-[10px] text-emerald-400">{{ statusCode || 200 }}</span>
                  </div>
                </div>
              </div>
              <div class="min-w-0 flex-1 space-y-3">
                <div class="flex flex-wrap items-center gap-2">
                  <div class="text-sm font-semibold">{{ t('usage.diagnosis.upstream') }}</div>
                  <div class="ml-auto flex flex-wrap gap-1">
                    <button
                      v-for="tab in [
                        { key: 'overview', label: t('usage.diagnosis.tabOverview') },
                        { key: 'request', label: t('usage.diagnosis.tabRequestShort') },
                        { key: 'response', label: statusOk ? t('usage.diagnosis.tabResponse') : t('usage.diagnosis.tabErrorResponse') },
                        { key: 'error_chain', label: 'Error Chain' },
                        { key: 'more', label: 'More' }
                      ]"
                      :key="tab.key"
                      type="button"
                      class="rounded-lg border px-2.5 py-1 text-xs"
                      :class="upstreamTab === tab.key ? 'border-primary-500 text-primary-500' : 'border-gray-200 text-gray-600 dark:border-dark-600 dark:text-gray-300'"
                      @click="upstreamTab = tab.key as UpstreamSubTab"
                    >{{ tab.label }}</button>
                  </div>
                </div>

                <div v-if="upstreamTab === 'overview'" class="grid grid-cols-1 gap-3 sm:grid-cols-2">
                  <div class="rounded-xl bg-gray-50 p-3 dark:bg-dark-800">
                    <div class="text-xs text-gray-500">{{ t('usage.diagnosis.startTime') }}</div>
                    <div class="mt-1 text-sm font-semibold">{{ formatTime(detail.created_at) }}</div>
                  </div>
                  <div class="rounded-xl bg-gray-50 p-3 dark:bg-dark-800">
                    <div class="text-xs text-gray-500">{{ t('usage.diagnosis.duration') }}</div>
                    <div class="mt-1 text-sm font-semibold">{{ formatDuration(detail.duration_ms) }}</div>
                  </div>
                  <div class="rounded-xl bg-gray-50 p-3 dark:bg-dark-800">
                    <div class="text-xs text-gray-500">{{ t('usage.diagnosis.requestMethod') }}</div>
                    <div class="mt-1 text-sm font-semibold">{{ detail.method || 'POST' }}</div>
                  </div>
                  <div class="rounded-xl bg-gray-50 p-3 dark:bg-dark-800">
                    <div class="text-xs text-gray-500">{{ t('usage.diagnosis.upstreamStatus') }}</div>
                    <div class="mt-1 text-sm font-semibold">HTTP {{ detail.upstream_status || detail.status_code || '-' }}</div>
                  </div>
                  <div class="rounded-xl bg-gray-50 p-3 dark:bg-dark-800">
                    <div class="text-xs text-gray-500">{{ t('usage.diagnosis.requestPath') }}</div>
                    <div class="mt-1 break-all text-sm font-semibold">{{ detail.path || '-' }}</div>
                  </div>
                  <div class="rounded-xl bg-gray-50 p-3 sm:col-span-2 dark:bg-dark-800">
                    <div class="text-xs text-gray-500">{{ t('usage.diagnosis.upstreamUrl') }}</div>
                    <div class="mt-1 break-all text-sm font-semibold">{{ detail.upstream_url || '-' }}</div>
                  </div>
                </div>

                <div v-else-if="upstreamTab === 'request'">
                  <JsonTreeViewer
                    :title="t('usage.diagnosis.upstreamRequestBodyJson')"
                    :raw="detail.upstream_req_body || detail.req_body"
                    :empty-text="t('usage.diagnosis.emptyUpstreamRequest')"
                  />
                </div>

                <div v-else-if="upstreamTab === 'response'" class="space-y-3">
                  <div>
                    <div class="mb-2 text-xs font-semibold uppercase text-gray-500">{{ t('usage.diagnosis.responseHeaders') }}</div>
                    <pre class="max-h-40 overflow-auto rounded-xl border border-gray-200 bg-gray-50 p-3 font-mono text-xs dark:border-dark-700 dark:bg-dark-800">{{ formatHeaders(detail.res_headers) || t('usage.diagnosis.emptyHeaders') }}</pre>
                  </div>
                  <JsonTreeViewer :title="t('usage.diagnosis.responseBodyJson')" :raw="detail.res_body" :empty-text="t('usage.diagnosis.emptyBody')" />
                </div>

                <div v-else-if="upstreamTab === 'error_chain'" class="py-10 text-center text-sm text-gray-500">
                  <template v-if="detail.error_chain">
                    <pre class="mx-auto max-w-3xl overflow-auto rounded-xl border border-gray-200 bg-gray-50 p-3 text-left font-mono text-xs dark:border-dark-700 dark:bg-dark-800">{{ typeof detail.error_chain === 'string' ? detail.error_chain : JSON.stringify(detail.error_chain, null, 2) }}</pre>
                  </template>
                  <template v-else>{{ t('usage.diagnosis.noErrorChain') }}</template>
                </div>

                <div v-else class="space-y-3">
                  <div class="flex flex-wrap items-center gap-2">
                    <button type="button" class="rounded-lg border px-2.5 py-1 text-xs" :class="moreSide === 'request' ? 'border-primary-500 text-primary-500' : 'border-gray-200 dark:border-dark-600'" @click="moreSide = 'request'">{{ t('usage.diagnosis.tabRequestShort') }}</button>
                    <button type="button" class="rounded-lg border px-2.5 py-1 text-xs" :class="moreSide === 'response' ? 'border-primary-500 text-primary-500' : 'border-gray-200 dark:border-dark-600'" @click="moreSide = 'response'">{{ t('usage.diagnosis.tabResponse') }}</button>
                    <button type="button" class="ml-auto rounded-lg border border-gray-200 px-2.5 py-1 text-xs dark:border-dark-600" @click="downloadAudit">{{ t('usage.diagnosis.download') }}</button>
                  </div>
                  <div v-if="moreSide === 'response'" class="flex gap-2">
                    <button type="button" class="rounded-lg border px-2.5 py-1 text-xs" :class="morePart === 'thinking' ? 'border-primary-500 text-primary-500' : 'border-gray-200 dark:border-dark-600'" @click="morePart = 'thinking'">{{ t('usage.diagnosis.roleThinking') }}</button>
                    <button type="button" class="rounded-lg border px-2.5 py-1 text-xs" :class="morePart === 'reply' ? 'border-primary-500 text-primary-500' : 'border-gray-200 dark:border-dark-600'" @click="morePart = 'reply'">{{ t('usage.diagnosis.roleAssistant') }}</button>
                  </div>

                  <div v-if="moreEmpty" class="py-10 text-center text-sm text-gray-500">{{ t('usage.diagnosis.noDisplayContent') }}</div>

                  <template v-else-if="moreSide === 'request'">
                    <div class="flex items-center gap-2">
                      <button type="button" class="btn btn-secondary btn-sm" :disabled="requestTurnIndex <= 0" @click="requestTurnIndex--">{{ t('usage.diagnosis.prev') }}</button>
                      <select v-model.number="requestTurnIndex" class="min-w-0 flex-1 rounded-lg border border-gray-200 bg-transparent px-2 py-1 text-xs dark:border-dark-600">
                        <option v-for="(turn, i) in requestTurns" :key="i" :value="i">
                          {{ i + 1 }}/{{ requestTurns.length }} · {{ roleName(turn.role) }} · {{ (turn.text || '').slice(0, 48) }}
                        </option>
                      </select>
                      <button type="button" class="btn btn-secondary btn-sm" :disabled="requestTurnIndex >= requestTurns.length - 1" @click="requestTurnIndex++">{{ t('usage.diagnosis.next') }}</button>
                    </div>
                    <div v-if="requestTurns[requestTurnIndex]" class="rounded-xl border border-gray-200 p-3 dark:border-dark-700">
                      <div class="mb-2 flex items-center justify-between">
                        <div class="text-sm font-semibold">{{ requestTurnIndex + 1 }}/{{ requestTurns.length }} · {{ roleName(requestTurns[requestTurnIndex].role) }}</div>
                        <button type="button" class="text-xs text-primary-500" @click="copyText(requestTurns[requestTurnIndex].text)">{{ t('usage.diagnosis.copy') }}</button>
                      </div>
                      <div class="whitespace-pre-wrap text-sm">{{ requestTurns[requestTurnIndex].text }}</div>
                    </div>
                  </template>

                  <template v-else>
                    <div v-if="morePart === 'thinking'" class="rounded-xl border border-gray-200 p-3 dark:border-dark-700">
                      <div class="mb-2 flex items-center justify-between">
                        <div class="text-sm font-semibold">{{ t('usage.diagnosis.roleThinking') }} <span class="ml-2 rounded bg-gray-200 px-1 text-[10px] dark:bg-dark-700">reason</span></div>
                        <button type="button" class="text-xs text-primary-500" @click="copyText(thinkingTurns.map(x => x.text).join('\n\n'))">{{ t('usage.diagnosis.copy') }}</button>
                      </div>
                      <div class="max-h-80 overflow-auto whitespace-pre-wrap text-sm">{{ thinkingTurns.map(x => x.text).join('\n\n') || '—' }}</div>
                    </div>
                    <div v-else class="rounded-xl border border-gray-200 p-3 dark:border-dark-700">
                      <div class="mb-2 flex items-center justify-between">
                        <div class="text-sm font-semibold">{{ t('usage.diagnosis.roleAssistant') }} <span class="ml-2 rounded bg-gray-200 px-1 text-[10px] dark:bg-dark-700">content</span></div>
                        <button type="button" class="text-xs text-primary-500" @click="copyText(replyTurns.map(x => x.text).join('\n\n'))">{{ t('usage.diagnosis.copy') }}</button>
                      </div>
                      <div class="whitespace-pre-wrap text-sm">{{ replyTurns.map(x => x.text).join('\n\n') || '—' }}</div>
                      <div class="mt-3 flex flex-wrap gap-2">
                        <button
                          v-for="img in replyTurns.flatMap(r => r.images)"
                          :key="img.id"
                          type="button"
                          class="rounded-lg border p-2 dark:border-dark-600"
                          @click="lightboxSrc = img.dataUrl || img.url || null"
                        >
                          <img v-if="img.dataUrl || img.url" :src="img.dataUrl || img.url" class="h-16 w-24 rounded object-cover" alt="" />
                        </button>
                      </div>
                    </div>
                  </template>
                </div>
              </div>
            </div>
          </template>
        </div>

        <!-- resize handle -->
        <div
          class="absolute bottom-1 right-1 h-4 w-4 cursor-se-resize opacity-60"
          title="resize"
          @mousedown.prevent="startResize"
        >
          <svg viewBox="0 0 16 16" class="h-4 w-4 text-gray-400"><path fill="currentColor" d="M10 16l6-6v2l-4 4h-2zm4 0l2-2v2h-2zM6 16l10-10v2L8 16H6z"/></svg>
        </div>
      </div>

      <!-- lightbox -->
      <div v-if="lightboxSrc" class="fixed inset-0 z-[90] flex items-center justify-center bg-black/80 p-6" @click.self="lightboxSrc = null">
        <button type="button" class="absolute right-6 top-6 text-white" @click="lightboxSrc = null">×</button>
        <img :src="lightboxSrc" class="max-h-full max-w-full rounded-lg" alt="" @click="lightboxSrc = null" />
        <button type="button" class="absolute bottom-6 right-6 rounded bg-white/90 px-3 py-1 text-sm" @click="downloadMedia(lightboxSrc, 'image')">{{ t('usage.diagnosis.download') }}</button>
      </div>
    </div>
  </Teleport>
</template>
