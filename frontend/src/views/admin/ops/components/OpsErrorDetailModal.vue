<template>
  <BaseDialog :show="show" :title="title" width="full" :close-on-click-outside="true" @close="close">
    <div v-if="loading" class="flex items-center justify-center py-16">
      <div class="flex flex-col items-center gap-3">
        <div class="h-8 w-8 animate-spin rounded-full border-b-2 border-primary-600"></div>
        <div class="text-sm font-medium text-gray-500 dark:text-gray-400">{{ t('admin.ops.errorDetail.loading') }}</div>
      </div>
    </div>

    <div v-else-if="!detail" class="py-10 text-center text-sm text-gray-500 dark:text-gray-400">
      {{ emptyText }}
    </div>

    <div v-else class="space-y-6 p-6">
      <!-- Summary -->
      <div class="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
        <div class="rounded-xl bg-gray-50 p-4 dark:bg-dark-900">
          <div class="text-xs font-bold uppercase tracking-wider text-gray-400">{{ t('admin.ops.errorDetail.requestId') }}</div>
          <div class="mt-1 break-all font-mono text-sm font-medium text-gray-900 dark:text-white">
            {{ requestId || '—' }}
          </div>
        </div>

        <div class="rounded-xl bg-gray-50 p-4 dark:bg-dark-900">
          <div class="text-xs font-bold uppercase tracking-wider text-gray-400">{{ t('admin.ops.errorDetail.time') }}</div>
          <div class="mt-1 text-sm font-medium text-gray-900 dark:text-white">
            {{ formatDateTime(detail.created_at) }}
          </div>
        </div>

        <div class="rounded-xl bg-gray-50 p-4 dark:bg-dark-900">
          <div class="text-xs font-bold uppercase tracking-wider text-gray-400">
            {{ isUpstreamError(detail) ? t('admin.ops.errorDetail.account') : t('admin.ops.errorDetail.user') }}
          </div>
          <div class="mt-1 text-sm font-medium text-gray-900 dark:text-white">
            <template v-if="isUpstreamError(detail)">
              {{ detail.account_name || (detail.account_id != null ? String(detail.account_id) : '—') }}
            </template>
            <template v-else>
              {{ detail.user_email || (detail.user_id != null ? String(detail.user_id) : '—') }}
            </template>
          </div>
        </div>

        <div class="rounded-xl bg-gray-50 p-4 dark:bg-dark-900">
          <div class="text-xs font-bold uppercase tracking-wider text-gray-400">{{ t('admin.ops.errorDetail.platform') }}</div>
          <div class="mt-1 text-sm font-medium text-gray-900 dark:text-white">
            {{ detail.platform || '—' }}
          </div>
        </div>

        <div class="rounded-xl bg-gray-50 p-4 dark:bg-dark-900">
          <div class="text-xs font-bold uppercase tracking-wider text-gray-400">{{ t('admin.ops.errorDetail.group') }}</div>
          <div class="mt-1 text-sm font-medium text-gray-900 dark:text-white">
            {{ detail.group_name || (detail.group_id != null ? String(detail.group_id) : '—') }}
          </div>
        </div>

        <div class="rounded-xl bg-gray-50 p-4 dark:bg-dark-900">
          <div class="text-xs font-bold uppercase tracking-wider text-gray-400">{{ t('admin.ops.errorDetail.model') }}</div>
          <div class="mt-1 text-sm font-medium text-gray-900 dark:text-white">
            <template v-if="hasModelMapping(detail)">
              <span class="font-mono">{{ detail.requested_model }}</span>
              <span class="mx-1 text-gray-400">→</span>
              <span class="font-mono text-primary-600 dark:text-primary-400">{{ detail.upstream_model }}</span>
            </template>
            <template v-else>
              {{ displayModel(detail) || '—' }}
            </template>
          </div>
        </div>

        <div class="rounded-xl bg-gray-50 p-4 dark:bg-dark-900">
          <div class="text-xs font-bold uppercase tracking-wider text-gray-400">{{ t('admin.ops.errorDetail.inboundEndpoint') }}</div>
          <div class="mt-1 break-all font-mono text-sm font-medium text-gray-900 dark:text-white">
            {{ detail.inbound_endpoint || '—' }}
          </div>
        </div>

        <div class="rounded-xl bg-gray-50 p-4 dark:bg-dark-900">
          <div class="text-xs font-bold uppercase tracking-wider text-gray-400">{{ t('admin.ops.errorDetail.upstreamEndpoint') }}</div>
          <div class="mt-1 break-all font-mono text-sm font-medium text-gray-900 dark:text-white">
            {{ detail.upstream_endpoint || '—' }}
          </div>
        </div>

        <div class="rounded-xl bg-gray-50 p-4 dark:bg-dark-900">
          <div class="text-xs font-bold uppercase tracking-wider text-gray-400">{{ t('admin.ops.errorDetail.status') }}</div>
          <div class="mt-1">
            <span :class="['inline-flex items-center rounded-lg px-2 py-1 text-xs font-black ring-1 ring-inset shadow-sm', statusClass]">
              {{ detail.status_code }}
            </span>
          </div>
        </div>

        <div class="rounded-xl bg-gray-50 p-4 dark:bg-dark-900">
          <div class="text-xs font-bold uppercase tracking-wider text-gray-400">{{ t('admin.ops.errorDetail.upstreamStatus') }}</div>
          <div class="mt-1">
            <span :class="['inline-flex items-center rounded-lg px-2 py-1 text-xs font-black ring-1 ring-inset shadow-sm', upstreamStatusClass]">
              {{ detail.upstream_status_code ?? '—' }}
            </span>
          </div>
        </div>

        <!-- 上游商户：回答「这次失败是哪家上游商户造成的」，并能直接拉黑整个商户。
             只有当请求真的到达了上游并被拒绝时才有值：网络层失败（连接被重置/超时）
             上游根本没收到，取不到商户；上游对失败日志也只保留很短一段，超期补不回来。
             这两种情况都显示为「未知」，是正常的，不代表这里坏了。 -->
        <div class="rounded-xl bg-gray-50 p-4 dark:bg-dark-900">
          <div class="text-xs font-bold uppercase tracking-wider text-gray-400">{{ t('admin.ops.errorDetail.upstreamSupplier') }}</div>
          <div class="mt-1 flex flex-wrap items-center gap-2">
            <span v-if="detail.upstream_supplier_id != null" class="text-sm font-medium text-gray-900 dark:text-white">
              {{ detail.upstream_supplier_name || '—' }}
              <span class="ml-1 font-mono text-xs text-gray-400">#{{ detail.upstream_supplier_id }}</span>
            </span>
            <span v-else class="text-sm text-gray-400 dark:text-gray-500">
              {{ t('admin.ops.errorDetail.upstreamSupplierUnknown') }}
            </span>
            <button
              v-if="detail.upstream_supplier_id != null"
              type="button"
              data-testid="block-supplier"
              class="inline-flex items-center rounded px-2 py-0.5 text-xs font-medium transition-colors disabled:cursor-not-allowed disabled:opacity-40 bg-red-100 text-red-800 hover:bg-red-200 dark:bg-red-900 dark:text-red-200 dark:hover:bg-red-800"
              :disabled="blocking"
              @click="blockSupplier"
            >
              {{ t('admin.ops.errorDetail.blockSupplier') }}
            </button>
          </div>
        </div>

        <div class="rounded-xl bg-gray-50 p-4 dark:bg-dark-900">
          <div class="text-xs font-bold uppercase tracking-wider text-gray-400">{{ t('admin.ops.errorDetail.requestType') }}</div>
          <div class="mt-1 text-sm font-medium text-gray-900 dark:text-white">
            {{ formatRequestTypeLabel(detail.request_type) }}
          </div>
        </div>

        <div class="rounded-xl bg-gray-50 p-4 dark:bg-dark-900">
          <div class="text-xs font-bold uppercase tracking-wider text-gray-400">{{ t('admin.ops.errorDetail.message') }}</div>
          <div class="mt-1 break-words text-sm font-medium text-gray-900 dark:text-white" :title="rootCauseMessage">
            {{ rootCauseMessage || '—' }}
          </div>
        </div>

        <div v-if="detail.api_key_prefix" class="rounded-xl bg-gray-50 p-4 dark:bg-dark-900">
          <div class="text-xs font-bold uppercase tracking-wider text-gray-400">{{ t('admin.ops.errorDetail.apiKeyPrefix') }}</div>
          <div class="mt-1 font-mono text-sm font-medium text-gray-900 dark:text-white">
            {{ detail.api_key_prefix }}
          </div>
        </div>

      </div>

      <div v-if="rootCauseMessage" class="rounded-xl bg-amber-50 p-6 dark:bg-amber-900/10">
        <h3 class="text-sm font-black uppercase tracking-wider text-amber-900 dark:text-amber-200">{{ t('admin.ops.errorDetail.rootCause') }}</h3>
        <div class="mt-3 break-words text-sm font-medium text-amber-900 dark:text-amber-100">{{ rootCauseMessage }}</div>
      </div>

      <div class="rounded-xl bg-gray-50 p-6 dark:bg-dark-900">
        <h3 class="text-sm font-black uppercase tracking-wider text-gray-900 dark:text-white">{{ t('admin.ops.errorDetail.diagnosticPayloads') }}</h3>
        <div v-if="!diagnosticPayloadSections.length" class="mt-4 text-sm text-gray-500 dark:text-gray-400">{{ t('common.noData') }}</div>
        <div v-else class="mt-4 space-y-4">
          <div v-for="section in diagnosticPayloadSections" :key="section.key">
            <div class="mb-2 text-xs font-bold uppercase tracking-wider text-gray-500 dark:text-gray-400">{{ diagnosticPayloadLabel(section.key) }}</div>
            <pre class="max-h-[520px] overflow-auto rounded-xl border border-gray-200 bg-white p-4 text-xs text-gray-800 dark:border-dark-700 dark:bg-dark-800 dark:text-gray-100"><code>{{ prettyJSON(section.value) }}</code></pre>
          </div>
        </div>
      </div>

      <!-- Upstream errors list (only for request errors) -->
      <div v-if="showUpstreamList" class="rounded-xl bg-gray-50 p-6 dark:bg-dark-900">
        <div class="flex flex-wrap items-center justify-between gap-2">
          <h3 class="text-sm font-black uppercase tracking-wider text-gray-900 dark:text-white">{{ t('admin.ops.errorDetails.upstreamErrors') }}</h3>
          <div class="text-xs text-gray-500 dark:text-gray-400" v-if="correlatedUpstreamLoading">{{ t('common.loading') }}</div>
        </div>

        <div v-if="!correlatedUpstreamLoading && !correlatedUpstreamErrors.length" class="mt-3 text-sm text-gray-500 dark:text-gray-400">
          {{ t('common.noData') }}
        </div>

        <div v-else class="mt-4 space-y-3">
          <div
            v-for="(ev, idx) in correlatedUpstreamErrors"
            :key="ev.id"
            class="rounded-xl border border-gray-200 bg-white p-4 dark:border-dark-700 dark:bg-dark-800"
          >
            <div class="flex flex-wrap items-center justify-between gap-2">
              <div class="text-xs font-black text-gray-900 dark:text-white">
                #{{ idx + 1 }}
                <span v-if="ev.type" class="ml-2 rounded-md bg-gray-100 px-2 py-0.5 font-mono text-[10px] font-bold text-gray-700 dark:bg-dark-700 dark:text-gray-200">{{ ev.type }}</span>
              </div>
              <div class="flex items-center gap-2">
                <div class="font-mono text-xs text-gray-500 dark:text-gray-400">
                  {{ ev.status_code ?? '—' }}
                </div>
                <button
                  type="button"
                  class="inline-flex items-center gap-1.5 rounded-md px-1.5 py-1 text-[10px] font-bold text-primary-700 hover:bg-primary-50 disabled:cursor-not-allowed disabled:opacity-60 dark:text-primary-200 dark:hover:bg-dark-700"
                  :disabled="!getUpstreamResponsePreview(ev)"
                  :title="getUpstreamResponsePreview(ev) ? '' : t('common.noData')"
                  @click="toggleUpstreamDetail(ev.id)"
                >
                  <Icon
                    :name="expandedUpstreamDetailIds.has(ev.id) ? 'chevronDown' : 'chevronRight'"
                    size="xs"
                    :stroke-width="2"
                  />
                  <span>
                    {{
                      expandedUpstreamDetailIds.has(ev.id)
                        ? t('admin.ops.errorDetail.responsePreview.collapse')
                        : t('admin.ops.errorDetail.responsePreview.expand')
                    }}
                  </span>
                </button>
              </div>
            </div>

            <div class="mt-3 grid grid-cols-1 gap-2 text-xs text-gray-600 dark:text-gray-300 sm:grid-cols-2">
              <div>
                <span class="text-gray-400">{{ t('admin.ops.errorDetail.upstreamEvent.status') }}:</span>
                <span class="ml-1 font-mono">{{ ev.status_code ?? '—' }}</span>
              </div>
              <div>
                <span class="text-gray-400">{{ t('admin.ops.errorDetail.upstreamEvent.requestId') }}:</span>
                <span class="ml-1 font-mono">{{ ev.request_id || ev.client_request_id || '—' }}</span>
              </div>
            </div>

            <div v-if="ev.message" class="mt-3 break-words text-sm font-medium text-gray-900 dark:text-white">{{ ev.message }}</div>

            <pre
              v-if="expandedUpstreamDetailIds.has(ev.id)"
              class="mt-3 max-h-[240px] overflow-auto rounded-xl border border-gray-200 bg-gray-50 p-3 text-xs text-gray-800 dark:border-dark-700 dark:bg-dark-900 dark:text-gray-100"
            ><code>{{ prettyJSON(getUpstreamResponsePreview(ev)) }}</code></pre>
          </div>
        </div>
      </div>
    </div>
    <template v-if="backToList" #footer>
      <button
        type="button"
        class="btn btn-secondary"
        data-testid="error-detail-back-to-list"
        @click="goBack"
      >
        {{ t('admin.ops.errorDetail.backToList') }}
      </button>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import { useAppStore } from '@/stores'
import { opsAPI, type OpsErrorDetail } from '@/api/admin/ops'
import { blockUpstream } from '@/api/admin/usage'
import { formatDateTime } from '@/utils/format'
import { resolveUpstreamPayload } from '../utils/errorDetailResponse'

interface Props {
  show: boolean
  errorId: number | null
  errorType?: 'request' | 'upstream'
  backToList?: boolean
}

interface Emits {
  (e: 'update:show', value: boolean): void
  (e: 'back'): void
}

const props = defineProps<Props>()
const emit = defineEmits<Emits>()

const { t } = useI18n()
const appStore = useAppStore()

const loading = ref(false)
const detail = ref<OpsErrorDetail | null>(null)

// 拉黑整个上游商户。用 window.confirm 二次确认并写明不可逆影响面：
// 该商户名下所有渠道都会退出路由，且其名下固定绑定不会自动改绑。
const blocking = ref(false)
const blockSupplier = async () => {
  const id = detail.value?.upstream_supplier_id
  if (id == null || blocking.value) return
  const name = (detail.value?.upstream_supplier_name || '').trim() || `#${id}`
  if (!window.confirm(t('admin.ops.errorDetail.blockSupplierConfirm', { name, id }))) return
  blocking.value = true
  try {
    await blockUpstream({ scope: 'supplier', supplier_id: id })
    appStore.showSuccess(t('admin.ops.errorDetail.blockSupplierOk'))
  } catch (err: any) {
    // 把后端带回的上游原话原样透出：令牌失效/无权限的原因只在那里。
    appStore.showError(err?.message || String(err))
  } finally {
    blocking.value = false
  }
}

const showUpstreamList = computed(() => props.errorType === 'request')

const requestId = computed(() => detail.value?.request_id || detail.value?.client_request_id || '')

type DiagnosticPayloadKey = 'client' | 'upstream_message' | 'upstream_detail' | 'upstream_events'

const rootCauseMessage = computed(() => {
  const current = detail.value
  if (!current) return ''
  for (const candidate of [current.upstream_error_message, current.upstream_error_detail, current.message, current.error_body]) {
    const value = meaningfulPayload(candidate)
    if (value) return value
  }
  return ''
})

const diagnosticPayloadSections = computed(() => {
  const current = detail.value
  if (!current) return []
  const candidates: Array<{ key: DiagnosticPayloadKey; value: string }> = [
    { key: 'client', value: meaningfulPayload(current.error_body) },
    { key: 'upstream_message', value: meaningfulPayload(current.upstream_error_message) },
    { key: 'upstream_detail', value: meaningfulPayload(current.upstream_error_detail) },
    { key: 'upstream_events', value: meaningfulPayload(current.upstream_errors) }
  ]
  return candidates.filter((section, index, all) => {
    return section.value && all.findIndex(candidate => candidate.value === section.value) === index
  })
})

function meaningfulPayload(candidate: unknown): string {
  const value = String(candidate || '').trim()
  if (!value || value === '[]' || value === '{}' || value.toLowerCase() === 'null') return ''
  return value
}

function diagnosticPayloadLabel(key: DiagnosticPayloadKey): string {
  return t(`admin.ops.errorDetail.payloads.${key}`)
}

const title = computed(() => {
  if (!props.errorId) return t('admin.ops.errorDetail.title')
  return t('admin.ops.errorDetail.titleWithId', { id: String(props.errorId) })
})

const emptyText = computed(() => t('admin.ops.errorDetail.noErrorSelected'))

function isUpstreamError(d: OpsErrorDetail | null): boolean {
  if (!d) return false
  const phase = String(d.phase || '').toLowerCase()
  const owner = String(d.error_owner || '').toLowerCase()
  return phase === 'upstream' && owner === 'provider'
}

function formatRequestTypeLabel(type: number | null | undefined): string {
  switch (type) {
    case 1: return t('admin.ops.errorDetail.requestTypeSync')
    case 2: return t('admin.ops.errorDetail.requestTypeStream')
    case 3: return t('admin.ops.errorDetail.requestTypeWs')
    default: return t('admin.ops.errorDetail.requestTypeUnknown')
  }
}

function hasModelMapping(d: OpsErrorDetail | null): boolean {
  if (!d) return false
  const requested = String(d.requested_model || '').trim()
  const upstream = String(d.upstream_model || '').trim()
  return !!requested && !!upstream && requested !== upstream
}

function displayModel(d: OpsErrorDetail | null): string {
  if (!d) return ''
  const upstream = String(d.upstream_model || '').trim()
  if (upstream) return upstream
  const requested = String(d.requested_model || '').trim()
  if (requested) return requested
  return String(d.model || '').trim()
}

const correlatedUpstream = ref<OpsErrorDetail[]>([])
const correlatedUpstreamLoading = ref(false)

const correlatedUpstreamErrors = computed<OpsErrorDetail[]>(() => correlatedUpstream.value)

const expandedUpstreamDetailIds = ref(new Set<number>())

function getUpstreamResponsePreview(ev: OpsErrorDetail): string {
  const upstreamPayload = resolveUpstreamPayload(ev)
  if (upstreamPayload) return upstreamPayload
  return String(ev.error_body || '').trim()
}

function toggleUpstreamDetail(id: number) {
  const next = new Set(expandedUpstreamDetailIds.value)
  if (next.has(id)) next.delete(id)
  else next.add(id)
  expandedUpstreamDetailIds.value = next
}

async function fetchCorrelatedUpstreamErrors(requestErrorId: number) {
  correlatedUpstreamLoading.value = true
  try {
    const res = await opsAPI.listRequestErrorUpstreamErrors(
      requestErrorId,
      { page: 1, page_size: 100, view: 'all' },
      { include_detail: true }
    )
    correlatedUpstream.value = res.items || []
  } catch (err) {
    console.error('[OpsErrorDetailModal] Failed to load correlated upstream errors', err)
    correlatedUpstream.value = []
  } finally {
    correlatedUpstreamLoading.value = false
  }
}

function close() {
  emit('update:show', false)
}

function goBack() {
  emit('update:show', false)
  emit('back')
}

function prettyJSON(raw?: string): string {
  if (!raw) return 'N/A'
  try {
    return JSON.stringify(JSON.parse(raw), null, 2)
  } catch {
    return raw
  }
}

async function fetchDetail(id: number) {
  loading.value = true
  try {
    const kind = props.errorType || (detail.value?.phase === 'upstream' ? 'upstream' : 'request')
    const d = kind === 'upstream' ? await opsAPI.getUpstreamErrorDetail(id) : await opsAPI.getRequestErrorDetail(id)
    detail.value = d
  } catch (err: any) {
    detail.value = null
    appStore.showError(err?.message || t('admin.ops.failedToLoadErrorDetail'))
  } finally {
    loading.value = false
  }
}

watch(
  () => [props.show, props.errorId] as const,
  ([show, id]) => {
    if (!show) {
      detail.value = null
      return
    }
    if (typeof id === 'number' && id > 0) {
      expandedUpstreamDetailIds.value = new Set()
      fetchDetail(id)
      if (props.errorType === 'request') {
        fetchCorrelatedUpstreamErrors(id)
      } else {
        correlatedUpstream.value = []
      }
    }
  },
  { immediate: true }
)

function statusBadgeClass(code: number): string {
  if (code >= 500) return 'bg-red-50 text-red-700 ring-red-600/20 dark:bg-red-900/30 dark:text-red-400 dark:ring-red-500/30'
  if (code === 429) return 'bg-purple-50 text-purple-700 ring-purple-600/20 dark:bg-purple-900/30 dark:text-purple-400 dark:ring-purple-500/30'
  if (code >= 400) return 'bg-amber-50 text-amber-700 ring-amber-600/20 dark:bg-amber-900/30 dark:text-amber-400 dark:ring-amber-500/30'
  return 'bg-gray-50 text-gray-700 ring-gray-600/20 dark:bg-gray-900/30 dark:text-gray-400 dark:ring-gray-500/30'
}

const statusClass = computed(() => statusBadgeClass(detail.value?.status_code ?? 0))

const upstreamStatusClass = computed(() => statusBadgeClass(detail.value?.upstream_status_code ?? 0))

</script>
