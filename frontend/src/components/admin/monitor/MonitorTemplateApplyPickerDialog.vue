<template>
  <BaseDialog
    :show="show"
    :title="t('admin.channelMonitor.template.applyPickerTitle', { name: templateName REDACTED)"
    @close="$emit('close')"
  >
    <p class="mb-3 text-sm text-gray-600 dark:text-gray-400">
      {{ t('admin.channelMonitor.template.applyPickerHint') REDACTEDREDACTED
    </p>

    <div v-if="loading" class="py-6 text-center text-sm text-gray-400">
      {{ t('common.loading') REDACTEDREDACTED
    </div>

    <div v-else-if="monitors.length === 0" class="py-6 text-center text-sm text-gray-400">
      {{ t('admin.channelMonitor.template.applyPickerEmpty') REDACTEDREDACTED
    </div>

    <div v-else>
      <!-- 全选/全不选 -->
      <div class="mb-2 flex items-center gap-3 text-xs">
        <button
          type="button"
          class="text-primary-600 hover:underline dark:text-primary-400"
          @click="selectAll"
        >
          {{ t('common.selectAll') REDACTEDREDACTED
        </button>
        <button
          type="button"
          class="text-gray-500 hover:underline dark:text-gray-400"
          @click="selectNone"
        >
          {{ t('admin.channelMonitor.template.selectNone') REDACTEDREDACTED
        </button>
        <span class="ml-auto text-gray-500 dark:text-gray-400">
          {{ t('admin.channelMonitor.template.selectedCount', {
            n: selectedIds.length,
            total: monitors.length,
          REDACTED) REDACTEDREDACTED
        </span>
      </div>

      <ul class="max-h-80 divide-y divide-gray-100 overflow-y-auto rounded-lg border border-gray-200 dark:divide-dark-700 dark:border-dark-700">
        <li
          v-for="m in monitors"
          :key="m.id"
          class="flex cursor-pointer items-center gap-3 px-3 py-2 hover:bg-gray-50 dark:hover:bg-dark-800"
          @click="toggle(m.id)"
        >
          <input
            type="checkbox"
            :checked="selectedSet.has(m.id)"
            class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500"
            @click.stop="toggle(m.id)"
          />
          <span class="font-medium text-gray-900 dark:text-white">{{ m.name REDACTEDREDACTED</span>
          <span class="text-xs text-gray-400">{{ m.provider REDACTEDREDACTED</span>
          <span v-if="m.provider === 'openai'" class="text-xs text-gray-400">{{ m.api_mode REDACTEDREDACTED</span>
          <span
            v-if="!m.enabled"
            class="ml-auto rounded bg-gray-100 px-1.5 py-0.5 text-xs text-gray-500 dark:bg-dark-700 dark:text-gray-400"
          >
            {{ t('admin.channelMonitor.onlyDisabled').replace(/^仅|^Only /, '') REDACTEDREDACTED
          </span>
        </li>
      </ul>
    </div>

    <template #footer>
      <div class="flex justify-end gap-2">
        <button class="btn btn-secondary" @click="$emit('close')">
          {{ t('common.cancel') REDACTEDREDACTED
        </button>
        <button
          class="btn btn-primary"
          :disabled="submitting || selectedIds.length === 0"
          @click="handleApply"
        >
          {{ submitting
            ? t('common.submitting')
            : t('admin.channelMonitor.template.applyPickerConfirm', { n: selectedIds.length REDACTED) REDACTEDREDACTED
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, ref, watch REDACTED from 'vue'
import { useI18n REDACTED from 'vue-i18n'
import { useAppStore REDACTED from '@/stores/app'
import { extractApiErrorMessage REDACTED from '@/utils/apiError'
import { adminAPI REDACTED from '@/api/admin'
import type { AssociatedMonitorBrief REDACTED from '@/api/admin/channelMonitorTemplate'
import BaseDialog from '@/components/common/BaseDialog.vue'

const props = defineProps<{
  show: boolean
  templateId: number | null
  templateName: string
REDACTED>()

const emit = defineEmits<{
  (e: 'close'): void
  (e: 'applied', affected: number): void
REDACTED>()

const { t REDACTED = useI18n()
const appStore = useAppStore()

const loading = ref(false)
const submitting = ref(false)
const monitors = ref<AssociatedMonitorBrief[]>([])
const selectedIds = ref<number[]>([])

const selectedSet = computed(() => new Set(selectedIds.value))

watch(
  () => [props.show, props.templateId] as const,
  ([show, id]) => {
    if (!show || id == null) return
    void fetchMonitors(id)
  REDACTED,
  { immediate: true REDACTED,
)

async function fetchMonitors(id: number) {
  loading.value = true
  monitors.value = []
  selectedIds.value = []
  try {
    const { items REDACTED = await adminAPI.channelMonitorTemplate.listAssociatedMonitors(id)
    monitors.value = items
    // 默认全选
    selectedIds.value = items.map((m) => m.id)
  REDACTED catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('common.error')))
  REDACTED finally {
    loading.value = false
  REDACTED
REDACTED

function toggle(id: number) {
  const idx = selectedIds.value.indexOf(id)
  if (idx >= 0) selectedIds.value.splice(idx, 1)
  else selectedIds.value.push(id)
REDACTED

function selectAll() {
  selectedIds.value = monitors.value.map((m) => m.id)
REDACTED

function selectNone() {
  selectedIds.value = []
REDACTED

async function handleApply() {
  if (props.templateId == null || selectedIds.value.length === 0 || submitting.value) return
  submitting.value = true
  try {
    const { affected REDACTED = await adminAPI.channelMonitorTemplate.apply(
      props.templateId,
      [...selectedIds.value],
    )
    appStore.showSuccess(t('admin.channelMonitor.template.applySuccess', { n: affected REDACTED))
    emit('applied', affected)
    emit('close')
  REDACTED catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('common.error')))
  REDACTED finally {
    submitting.value = false
  REDACTED
REDACTED
</script>
