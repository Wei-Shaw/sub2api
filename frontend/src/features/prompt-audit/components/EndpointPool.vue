<template>
  <section aria-labelledby="prompt-pool-title" class="border-b border-gray-200 py-6 dark:border-dark-700/60">
    <div class="flex flex-wrap items-start justify-between gap-3">
      <div>
        <h2 id="prompt-pool-title" class="text-base font-semibold text-gray-950 dark:text-white">{{ t('admin.promptAudit.pool.title') }}</h2>
        <p class="mt-1 text-sm text-gray-500 dark:text-dark-300">{{ t('admin.promptAudit.pool.description') }}</p>
      </div>
      <button type="button" class="btn btn-primary btn-sm" data-test="add-endpoint" @click="openCreate">
        {{ t('admin.promptAudit.pool.add') }}
      </button>
    </div>

    <div v-if="endpoints.length === 0" class="mt-5 rounded-xl border border-dashed border-gray-300 px-5 py-10 text-center text-sm text-gray-500 dark:border-dark-600 dark:bg-dark-900/20 dark:text-dark-300">
      {{ t('admin.promptAudit.pool.empty') }}
    </div>
    <div v-else class="mt-5 overflow-hidden rounded-xl border border-gray-200 bg-white dark:border-dark-700/60 dark:bg-dark-900/20">
      <div class="hidden grid-cols-[minmax(260px,1.45fr)_minmax(210px,1fr)_minmax(190px,.8fr)_minmax(230px,1.15fr)_auto] gap-5 border-b border-l-[3px] border-b-gray-200 border-l-transparent bg-gray-50/80 px-5 py-2.5 text-[11px] font-semibold uppercase tracking-[0.08em] text-gray-500 dark:border-b-dark-700/60 dark:bg-dark-900/70 dark:text-dark-400 xl:grid">
        <span>{{ t('admin.promptAudit.pool.node') }}</span>
        <span>{{ t('admin.promptAudit.pool.model') }}</span>
        <span>{{ t('admin.promptAudit.pool.limits') }}</span>
        <span>{{ t('admin.promptAudit.pool.credential') }}</span>
        <span class="text-right">{{ t('admin.promptAudit.common.actions') }}</span>
      </div>

      <div class="divide-y divide-gray-100 dark:divide-dark-800">
        <article
          v-for="endpoint in endpoints"
          :key="endpoint.id"
          :data-test="`endpoint-${endpoint.id}`"
          class="group grid gap-4 border-l-[3px] border-l-transparent px-4 py-4 transition-[background-color,border-color] duration-200 hover:border-l-primary-500 hover:bg-gray-50/80 dark:hover:bg-dark-800/55 sm:px-5 xl:grid-cols-[minmax(260px,1.45fr)_minmax(210px,1fr)_minmax(190px,.8fr)_minmax(230px,1.15fr)_auto] xl:items-center xl:gap-5"
        >
          <div class="flex min-w-0 items-center gap-3">
            <button
              type="button"
              role="switch"
              :aria-checked="endpoint.enabled"
              :aria-label="t('admin.promptAudit.pool.toggleNode', { name: endpoint.name })"
              class="relative inline-flex h-6 w-11 shrink-0 cursor-pointer items-center rounded-full border-2 border-transparent transition-colors duration-200 focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500 focus-visible:ring-offset-2"
              :class="endpoint.enabled ? 'bg-primary-600' : 'bg-gray-200 dark:bg-dark-600'"
              @click="toggleEndpoint(endpoint.id)"
            >
              <span
                class="pointer-events-none inline-block h-5 w-5 rounded-full bg-white shadow transition-transform duration-200 ease-in-out"
                :class="endpoint.enabled ? 'translate-x-5' : 'translate-x-0'"
              />
            </button>
            <div class="min-w-0">
              <div class="flex min-w-0 items-center gap-2">
                <p class="truncate font-semibold text-gray-950 dark:text-white">{{ endpoint.name }}</p>
                <span class="h-1.5 w-1.5 shrink-0 rounded-full" :class="endpoint.enabled ? 'bg-emerald-500' : 'bg-gray-300 dark:bg-dark-500'" aria-hidden="true" />
              </div>
              <p class="mt-0.5 truncate font-mono text-[11px] text-gray-500 dark:text-dark-400" :title="endpoint.base_url">{{ endpoint.base_url }}</p>
            </div>
          </div>

          <div class="min-w-0 xl:block">
            <p class="mb-1 text-[10px] font-semibold uppercase tracking-wider text-gray-400 xl:hidden">{{ t('admin.promptAudit.pool.model') }}</p>
            <p class="truncate text-sm font-medium text-gray-700 dark:text-dark-200" :title="endpoint.model">{{ endpoint.model }}</p>
          </div>

          <div>
            <p class="mb-1 text-[10px] font-semibold uppercase tracking-wider text-gray-400 xl:hidden">{{ t('admin.promptAudit.pool.limits') }}</p>
            <div class="flex flex-wrap gap-1.5 text-xs text-gray-600 dark:text-dark-300">
              <span class="rounded-md bg-gray-100 px-2 py-1 tabular-nums dark:bg-dark-800">{{ endpoint.timeout_ms }} ms</span>
              <span class="rounded-md bg-gray-100 px-2 py-1 tabular-nums dark:bg-dark-800">{{ endpoint.input_limit }} chars</span>
            </div>
          </div>

          <div class="min-w-0">
            <p class="mb-1 text-[10px] font-semibold uppercase tracking-wider text-gray-400 xl:hidden">{{ t('admin.promptAudit.pool.credential') }}</p>
            <div class="flex items-center gap-1.5 text-xs font-medium" :class="hasCredential(endpoint) ? 'text-emerald-700 dark:text-emerald-300' : 'text-gray-500 dark:text-dark-400'">
              <span class="h-1.5 w-1.5 rounded-full" :class="hasCredential(endpoint) ? 'bg-emerald-500' : 'bg-gray-300 dark:bg-dark-500'" aria-hidden="true" />
              {{ hasCredential(endpoint) ? t('admin.promptAudit.pool.configured') : t('admin.promptAudit.pool.missing') }}
            </div>
            <p v-if="probingIds.includes(endpoint.id)" class="mt-1.5 text-xs text-primary-600 dark:text-primary-300">
              {{ t('admin.promptAudit.pool.probeProgress') }}
            </p>
            <p v-if="probeResults[endpoint.id]" class="mt-1.5 line-clamp-2 text-xs leading-5" :class="probeResults[endpoint.id].ok ? 'text-emerald-600 dark:text-emerald-300' : 'text-red-600 dark:text-red-300'">
              {{ t('admin.promptAudit.pool.probeResult', { status: probeResults[endpoint.id].status, http: probeResults[endpoint.id].http_status || '—', latency: probeResults[endpoint.id].latency_ms }) }}
              · {{ probeResults[endpoint.id].message }}
            </p>
          </div>

          <div class="flex flex-wrap items-center justify-end gap-1 border-t border-gray-100 pt-3 dark:border-dark-800 xl:flex-nowrap xl:border-0 xl:pt-0">
            <button type="button" class="btn btn-secondary btn-sm" :disabled="probingIds.includes(endpoint.id)" @click="$emit('probe', endpoint)">
              {{ probingIds.includes(endpoint.id) ? t('admin.promptAudit.pool.probing') : t('admin.promptAudit.pool.probe') }}
            </button>
            <button type="button" class="btn btn-ghost btn-sm" @click="openEdit(endpoint)">{{ t('common.edit') }}</button>
            <button type="button" class="btn btn-ghost btn-sm text-red-600 hover:bg-red-50 dark:text-red-300 dark:hover:bg-red-950/30" @click="removeEndpoint(endpoint)">{{ t('common.delete') }}</button>
          </div>
        </article>
      </div>
    </div>

    <BaseDialog :show="Boolean(editing)" :title="editingIndex < 0 ? t('admin.promptAudit.pool.add') : t('admin.promptAudit.pool.edit')" width="wide" @close="closeEditor">
      <form v-if="editing" class="grid gap-4 sm:grid-cols-2" @submit.prevent="saveEditor">
        <label class="space-y-1 text-sm text-gray-700 dark:text-dark-200">
          <span>{{ t('admin.promptAudit.pool.name') }}</span>
          <input v-model="editing.name" class="input w-full" required :aria-label="t('admin.promptAudit.pool.name')" />
        </label>
        <label class="space-y-1 text-sm text-gray-700 dark:text-dark-200">
          <span>{{ t('admin.promptAudit.pool.id') }}</span>
          <input v-model="editing.id" class="input w-full" required :disabled="editingIndex >= 0" :aria-label="t('admin.promptAudit.pool.id')" />
        </label>
        <label class="space-y-1 text-sm text-gray-700 dark:text-dark-200 sm:col-span-2">
          <span>{{ t('admin.promptAudit.pool.baseUrl') }}</span>
          <input v-model="editing.base_url" class="input w-full" required inputmode="url" :aria-label="t('admin.promptAudit.pool.baseUrl')" />
        </label>
        <label class="space-y-1 text-sm text-gray-700 dark:text-dark-200 sm:col-span-2">
          <span>{{ t('admin.promptAudit.pool.apiKey') }}</span>
          <input v-model="editing.token" class="input w-full" type="password" autocomplete="new-password" :placeholder="editing.has_token ? t('admin.promptAudit.pool.keepSecret') : ''" :aria-label="t('admin.promptAudit.pool.apiKey')" />
          <span class="block text-xs text-gray-500 dark:text-dark-400">{{ t('admin.promptAudit.pool.secretHint') }}</span>
        </label>
        <label v-if="editing.has_token" class="flex items-center gap-2 text-sm text-red-600 dark:text-red-300 sm:col-span-2">
          <input v-model="editing.clear_token" type="checkbox" :aria-label="t('admin.promptAudit.pool.clearSecret')" />
          {{ t('admin.promptAudit.pool.clearSecret') }}
        </label>
        <label class="space-y-1 text-sm text-gray-700 dark:text-dark-200 sm:col-span-2">
          <span>{{ t('admin.promptAudit.pool.model') }}</span>
          <input v-model="editing.model" class="input w-full" :aria-label="t('admin.promptAudit.pool.model')" />
        </label>
        <label class="space-y-1 text-sm text-gray-700 dark:text-dark-200">
          <span>{{ t('admin.promptAudit.pool.timeout') }}</span>
          <input v-model.number="editing.timeout_ms" class="input w-full" type="number" min="100" max="30000" required :aria-label="t('admin.promptAudit.pool.timeout')" />
        </label>
        <label class="space-y-1 text-sm text-gray-700 dark:text-dark-200">
          <span>{{ t('admin.promptAudit.pool.inputLimit') }}</span>
          <input v-model.number="editing.input_limit" class="input w-full" type="number" min="128" max="100000" required :aria-label="t('admin.promptAudit.pool.inputLimit')" />
        </label>
      </form>
      <template #footer>
        <div class="flex justify-end gap-3">
          <button type="button" class="btn btn-secondary" @click="closeEditor">{{ t('common.cancel') }}</button>
          <button type="button" class="btn btn-primary" data-test="save-endpoint" @click="saveEditor">{{ t('common.save') }}</button>
        </div>
      </template>
    </BaseDialog>
  </section>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import type { PromptAuditEndpointDraft, PromptProbeResult } from '../types'
import { cloneData, createDefaultEndpoint } from '../viewModel'

const props = defineProps<{
  endpoints: PromptAuditEndpointDraft[]
  probeResults: Record<string, PromptProbeResult>
  probingIds: string[]
}>()
const emit = defineEmits<{
  (event: 'update:endpoints', value: PromptAuditEndpointDraft[]): void
  (event: 'probe', endpoint: PromptAuditEndpointDraft): void
}>()
const { t } = useI18n()
const editing = ref<PromptAuditEndpointDraft | null>(null)
const editingIndex = ref(-1)

function openCreate() {
  editingIndex.value = -1
  editing.value = createDefaultEndpoint(props.endpoints.length + 1)
}
function openEdit(endpoint: PromptAuditEndpointDraft) {
  editingIndex.value = props.endpoints.findIndex((item) => item.id === endpoint.id)
  editing.value = cloneData(endpoint)
}
function closeEditor() {
  editing.value = null
  editingIndex.value = -1
}
function saveEditor() {
  if (!editing.value?.id.trim() || !editing.value.name.trim() || !editing.value.base_url.trim()) return
  const next = props.endpoints.map((item) => cloneData(item))
  const value = cloneData(editing.value)
  if (value.token.trim()) value.clear_token = false
  if (editingIndex.value < 0) next.push(value)
  else next.splice(editingIndex.value, 1, value)
  emit('update:endpoints', next)
  closeEditor()
}
function toggleEndpoint(id: string) {
  emit('update:endpoints', props.endpoints.map((item) => item.id === id ? { ...item, enabled: !item.enabled } : cloneData(item)))
}
function removeEndpoint(endpoint: PromptAuditEndpointDraft) {
  if (!window.confirm(t('admin.promptAudit.pool.deleteConfirm', { name: endpoint.name }))) return
  emit('update:endpoints', props.endpoints.filter((item) => item.id !== endpoint.id).map((item) => cloneData(item)))
}
function hasCredential(endpoint: PromptAuditEndpointDraft): boolean {
  return Boolean(endpoint.token.trim() || (endpoint.has_token && !endpoint.clear_token))
}
</script>
