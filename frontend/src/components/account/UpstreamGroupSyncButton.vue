<template>
  <div class="flex flex-wrap items-center gap-x-4 gap-y-2">
    <button
      type="button"
      class="inline-flex items-center gap-1 text-xs text-blue-600 hover:underline disabled:cursor-not-allowed disabled:opacity-50 dark:text-blue-400"
      :disabled="!source || loading"
      :title="!source ? t('admin.accounts.upstreamBilling.groupCredentials') : undefined"
      @click="sync"
    >
      <Icon name="refresh" size="xs" :class="{ 'animate-spin': loading }" />
      {{ t('admin.accounts.upstreamBilling.syncGroup') }}
    </button>
    <label v-if="allowAuto" class="inline-flex items-center gap-2 text-xs text-gray-700 dark:text-gray-200"
      :title="t('admin.accounts.upstreamBilling.autoSyncGroupHint')">
      {{ t('admin.accounts.upstreamBilling.autoSyncGroup') }}
      <Toggle v-model="auto" data-testid="upstream-billing-group-sync"
        :aria-label="t('admin.accounts.upstreamBilling.autoSyncGroup')" />
    </label>
  </div>
</template>

<script setup lang="ts">
import { onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import type { UpstreamGroupSource } from '@/api/admin/accounts'
import Icon from '@/components/icons/Icon.vue'
import Toggle from '@/components/common/Toggle.vue'
import { useAppStore } from '@/stores/app'

const props = defineProps<{ source: UpstreamGroupSource | null; allowAuto?: boolean }>()
const auto = defineModel<boolean>('auto', { default: false })
const name = defineModel<string>('name', { required: true })
const notes = defineModel<string>('notes', { required: true })
const { t } = useI18n()
const appStore = useAppStore()
const loading = ref(false)
let request: AbortController | undefined
watch(() => props.source, () => request?.abort())
onBeforeUnmount(() => request?.abort())

async function sync() {
  if (!props.source || loading.value) return
  const controller = new AbortController()
  request = controller
  loading.value = true
  const previousName = name.value
  const previousNotes = notes.value
  try {
    const fields = await adminAPI.accounts.previewUpstreamGroup(props.source, controller.signal)
    if (controller.signal.aborted) return
    if (!fields.name && !fields.notes) {
      appStore.showInfo(t('admin.accounts.upstreamBilling.groupUnavailable'))
      return
    }
    if (fields.name && name.value === previousName) name.value = fields.name
    if (fields.notes && notes.value === previousNotes) notes.value = fields.notes
  } catch {
    if (!controller.signal.aborted) appStore.showError(t('admin.accounts.upstreamBilling.groupFailed'))
  } finally {
    loading.value = false
    request = undefined
  }
}
</script>
