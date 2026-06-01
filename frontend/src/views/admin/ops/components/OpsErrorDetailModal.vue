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
            {{ prefersAccountSummary(detail) ? t('admin.ops.errorDetail.account') : t('admin.ops.errorDetail.user') }}
          </div>
          <div class="mt-1 space-y-1 text-sm font-medium text-gray-900 dark:text-white">
            <div>
              <template v-if="prefersAccountSummary(detail)">
                {{ formatAccountIdentity(detail) || '—' }}
              </template>
              <template v-else>
                {{ formatUserIdentity(detail) || '—' }}
              </template>
            </div>
            <div
              v-if="showSecondaryAccountSummary(detail)"
              class="text-xs font-medium text-gray-500 dark:text-gray-400"
            >
              {{ t('admin.ops.errorDetail.account') }}: {{ formatAccountIdentity(detail) }}
            </div>
            <div
              v-else-if="showSecondaryUserSummary(detail)"
              class="text-xs font-medium text-gray-500 dark:text-gray-400"
            >
              {{ t('admin.ops.errorDetail.user') }}: {{ formatUserIdentity(detail) }}
            </div>
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
          <div class="text-xs font-bold uppercase tracking-wider text-gray-400">{{ t('admin.ops.errorDetail.requestType') }}</div>
          <div class="mt-1 text-sm font-medium text-gray-900 dark:text-white">
            {{ formatRequestTypeLabel(detail.request_type) }}
          </div>
        </div>

        <div class="rounded-xl bg-gray-50 p-4 dark:bg-dark-900">
          <div class="text-xs font-bold uppercase tracking-wider text-gray-400">{{ t('admin.ops.errorDetail.message') }}</div>
          <div class="mt-1 truncate text-sm font-medium text-gray-900 dark:text-white" :title="detail.message">
            {{ detail.message || '—' }}
          </div>
        </div>
      </div>

      <!-- Response content (client request -> error_body; upstream -> upstream_error_detail/message) -->
      <div class="rounded-xl bg-gray-50 p-6 dark:bg-dark-900">
        <h3 class="text-sm font-black uppercase tracking-wider text-gray-900 dark:text-white">{{ t('admin.ops.errorDetail.responseBody') }}</h3>
        <pre class="mt-4 max-h-[520px] overflow-auto rounded-xl border border-gray-200 bg-white p-4 text-xs text-gray-800 dark:border-dark-700 dark:bg-dark-800 dark:text-gray-100"><code>{{ prettyJSON(primaryResponseBody || '') }}</code></pre>
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

            <div class="mt-3 grid grid-cols-1 gap-2 text-xs text-gray-600 dark:text-gray-300 sm:grid-cols-3">
              <div>
                <span class="text-gray-400">{{ t('admin.ops.errorDetail.upstreamEvent.account') }}:</span>
                <span class="ml-1 font-medium">{{ formatAccountIdentity(ev) || '—' }}</span>
              </div>
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
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import { useAppStore } from '@/stores'
import { opsAPI, type OpsErrorDetail } from '@/api/admin/ops'
import { formatDateTime } from '@/utils/format'
import { resolvePrimaryResponseBody, resolveUpstreamPayload } from '../utils/errorDetailResponse'

interface Props {
  show: boolean
  errorId: number | null
  errorType?: 'request' | 'upstream'
}

interface Emits {
  (e: 'update:show', value: boolean): void
}

const props = defineProps<Props>()
const emit = defineEmits<Emits>()

const { t } = useI18n()
const appStore = useAppStore()

const loading = ref(false)
const detail = ref<OpsErrorDetail | null>(null)

const showUpstreamList = computed(() => props.errorType === 'request')

const requestId = computed(() => detail.value?.request_id || detail.value?.client_request_id || '')

const primaryResponseBody = computed(() => {
  return resolvePrimaryResponseBody(detail.value, props.errorType)
})




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

function formatAccountIdentity(d: OpsErrorDetail | null): string {
  if (!d) return ''
  const accountName = String(d.account_name || '').trim()
  if (accountName) return accountName
  if (d.account_id != null) return String(d.account_id)
  return ''
}

function formatUserIdentity(d: OpsErrorDetail | null): string {
  if (!d) return ''
  const userEmail = String(d.user_email || '').trim()
  if (userEmail) return userEmail
  if (d.user_id != null) return String(d.user_id)
  return ''
}

function prefersAccountSummary(d: OpsErrorDetail | null): boolean {
  return !!formatAccountIdentity(d) && (isUpstreamError(d) || !formatUserIdentity(d))
}

function showSecondaryAccountSummary(d: OpsErrorDetail | null): boolean {
  return !prefersAccountSummary(d) && !!formatAccountIdentity(d)
}

function showSecondaryUserSummary(d: OpsErrorDetail | null): boolean {
  return prefersAccountSummary(d) && !isUpstreamError(d) && !!formatUserIdentity(d)
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

const statusClass = computed(() => {
  const code = detail.value?.status_code ?? 0
  if (code >= 500) return 'bg-red-50 text-red-700 ring-red-600/20 dark:bg-red-900/30 dark:text-red-400 dark:ring-red-500/30'
  if (code === 429) return 'bg-purple-50 text-purple-700 ring-purple-600/20 dark:bg-purple-900/30 dark:text-purple-400 dark:ring-purple-500/30'
  if (code >= 400) return 'bg-amber-50 text-amber-700 ring-amber-600/20 dark:bg-amber-900/30 dark:text-amber-400 dark:ring-amber-500/30'
  return 'bg-gray-50 text-gray-700 ring-gray-600/20 dark:bg-gray-900/30 dark:text-gray-400 dark:ring-gray-500/30'
})

</script>
