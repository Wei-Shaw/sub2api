<template>
  <BaseDialog
    :show="show"
    :title="t('admin.channelMonitor.template.applyPickerTitle', { name: templateName })"
    @close="$emit('close')"
  >
    <p class="mb-3 text-sm text-fg-muted">
      {{ t('admin.channelMonitor.template.applyPickerHint') }}
    </p>

    <div v-if="loading" class="py-6 text-center text-sm text-fg-subtle">
      {{ t('common.loading') }}
    </div>

    <div v-else-if="monitors.length === 0" class="py-6 text-center text-sm text-fg-subtle">
      {{ t('admin.channelMonitor.template.applyPickerEmpty') }}
    </div>

    <div v-else>
      <!-- 全选/全不选 -->
      <div class="mb-2 flex items-center gap-3 text-xs">
        <button
          type="button"
          class="text-accent hover:underline"
          @click="selectAll"
        >
          {{ t('common.selectAll') }}
        </button>
        <button
          type="button"
          class="text-fg-muted hover:underline"
          @click="selectNone"
        >
          {{ t('admin.channelMonitor.template.selectNone') }}
        </button>
        <span class="ml-auto text-fg-muted">
          {{ t('admin.channelMonitor.template.selectedCount', {
            n: selectedIds.length,
            total: monitors.length,
          }) }}
        </span>
      </div>

      <ul class="max-h-80 divide-y divide-border overflow-y-auto border border-border">
        <li
          v-for="m in monitors"
          :key="m.id"
          class="flex cursor-pointer items-center gap-3 px-3 py-2 hover:bg-accent-weak/50"
          @click="toggle(m.id)"
        >
          <input
            type="checkbox"
            :checked="selectedSet.has(m.id)"
            class="h-4 w-4 rounded-sm border-border-strong text-accent focus:ring-accent"
            @click.stop="toggle(m.id)"
          />
          <span class="font-medium text-fg">{{ m.name }}</span>
          <span class="text-xs text-fg-subtle">{{ m.provider }}</span>
          <span v-if="m.provider === 'openai'" class="text-xs text-fg-subtle">{{ m.api_mode }}</span>
          <span
            v-if="!m.enabled"
            class="badge badge-gray ml-auto"
          >
            {{ t('common.disabled') }}
          </span>
        </li>
      </ul>
    </div>

    <template #footer>
      <div class="flex justify-end gap-2">
        <button class="btn btn-secondary" @click="$emit('close')">
          {{ t('common.cancel') }}
        </button>
        <button
          class="btn btn-primary"
          :disabled="submitting || selectedIds.length === 0"
          @click="handleApply"
        >
          {{ submitting
            ? t('common.submitting')
            : t('admin.channelMonitor.template.applyPickerConfirm', { n: selectedIds.length }) }}
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import { adminAPI } from '@/api/admin'
import type { AssociatedMonitorBrief } from '@/api/admin/channelMonitorTemplate'
import BaseDialog from '@/components/common/BaseDialog.vue'

const props = defineProps<{
  show: boolean
  templateId: number | null
  templateName: string
}>()

const emit = defineEmits<{
  (e: 'close'): void
  (e: 'applied', affected: number): void
}>()

const { t } = useI18n()
const appStore = useAppStore()

const loading = ref(false)
const submitting = ref(false)
const monitors = ref<AssociatedMonitorBrief[]>([])
const selectedIds = ref<number[]>([])

const selectedSet = computed(() => new Set(selectedIds.value))
let requestVersion = 0

onUnmounted(() => { requestVersion++ })

watch(
  () => [props.show, props.templateId] as const,
  ([show, id]) => {
    const version = ++requestVersion
    if (!show || id == null) return
    void fetchMonitors(id, version)
  },
  { immediate: true },
)

async function fetchMonitors(id: number, version: number) {
  loading.value = true
  monitors.value = []
  selectedIds.value = []
  try {
    const { items } = await adminAPI.channelMonitorTemplate.listAssociatedMonitors(id)
    if (version !== requestVersion) return
    monitors.value = items
    // 默认全选
    selectedIds.value = items.map((m) => m.id)
  } catch (err: unknown) {
    if (version !== requestVersion) return
    appStore.showError(extractApiErrorMessage(err, t('common.error')))
  } finally {
    if (version === requestVersion) loading.value = false
  }
}

function toggle(id: number) {
  const idx = selectedIds.value.indexOf(id)
  if (idx >= 0) selectedIds.value.splice(idx, 1)
  else selectedIds.value.push(id)
}

function selectAll() {
  selectedIds.value = monitors.value.map((m) => m.id)
}

function selectNone() {
  selectedIds.value = []
}

async function handleApply() {
  if (props.templateId == null || selectedIds.value.length === 0 || submitting.value) return
  submitting.value = true
  try {
    const { affected } = await adminAPI.channelMonitorTemplate.apply(
      props.templateId,
      [...selectedIds.value],
    )
    appStore.showSuccess(t('admin.channelMonitor.template.applySuccess', { n: affected }))
    emit('applied', affected)
    emit('close')
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('common.error')))
  } finally {
    submitting.value = false
  }
}
</script>
