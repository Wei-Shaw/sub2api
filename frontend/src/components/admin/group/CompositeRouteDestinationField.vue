<template>
  <div class="space-y-2" data-testid="composite-destination">
    <label class="input-label">{{ t('admin.groups.compositeRoutes.destinationMode') }}</label>
    <Select
      :model-value="modelValue.mode"
      :options="modeOptions"
      :aria-label="t('admin.groups.compositeRoutes.destinationMode')"
      data-testid="destination-mode"
      @update:model-value="changeMode($event as CompositeDestinationMode)"
    />
    <p class="text-xs text-gray-500 dark:text-gray-400">
      {{ t(`admin.groups.compositeRoutes.destinationHints.${modelValue.mode}`) }}
    </p>
    <template v-if="modelValue.mode !== 'pool'">
      <div v-if="selectedId != null" class="rounded border border-gray-200 p-2 text-sm dark:border-dark-600" data-testid="destination-selected">
        <div>{{ selectedLabel }}</div>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400" data-testid="destination-status">
          {{ selectedStatus }}
        </p>
      </div>
      <input
        v-model="keyword"
        class="input"
        type="search"
        :aria-label="t('admin.groups.compositeRoutes.destinationSearch')"
        :placeholder="t('admin.groups.compositeRoutes.destinationSearch')"
        data-testid="destination-search"
      />
      <div class="max-h-40 overflow-y-auto rounded border border-gray-200 dark:border-dark-600">
        <p v-if="loading" class="px-3 py-2 text-xs text-gray-500">{{ t('common.loading') }}</p>
        <p v-else-if="error" class="px-3 py-2 text-xs text-red-600" role="alert">{{ t('admin.groups.compositeRoutes.destinationLoadFailed') }}</p>
        <p v-else-if="!results.length" class="px-3 py-2 text-xs text-gray-500">{{ t('admin.groups.compositeRoutes.destinationEmpty') }}</p>
        <button
          v-for="target in results"
          :key="target.id"
          type="button"
          class="block w-full px-3 py-2 text-left text-sm hover:bg-gray-100 disabled:opacity-50 dark:hover:bg-dark-700"
          :disabled="target.status !== 'active' || target.schedulable === false"
          :data-testid="`destination-option-${target.id}`"
          @click="selectTarget(target)"
        >
          {{ target.name }} #{{ target.id }}
          <span class="text-xs text-gray-500"> · {{ statusLabel(target) }}</span>
        </button>
      </div>
      <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.groups.compositeRoutes.destinationResultsHint') }}</p>
    </template>
    <p v-if="showValidationError" class="text-xs text-red-600 dark:text-red-400" role="alert" data-testid="destination-validation">
      {{ t('admin.groups.compositeRoutes.destinationRequired') }}
    </p>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import Select from '@/components/common/Select.vue'
import { useKeyedDebouncedSearch } from '@/composables/useKeyedDebouncedSearch'
import type { GroupPlatform } from '@/types'
import {
  changeCompositeDestinationMode,
  eligibleCompositeTargetGroup,
  validCompositeDestination,
  type CompositeDestinationMode,
  type CompositeRouteDestination
} from '@/views/admin/groupsCompositeRoutes'

interface TargetOption {
  id: number
  name: string
  status: string
  schedulable?: boolean
  reason?: string | null
}

const props = defineProps<{
  modelValue: CompositeRouteDestination
  platform: Exclude<GroupPlatform, 'composite'>
  sourceGroupId: number
}>()
const emit = defineEmits<{
  (event: 'update:modelValue', value: CompositeRouteDestination): void
}>()
const { t } = useI18n()
const modeOptions = computed(() => (['pool', 'group', 'account'] as const).map(mode => ({
  value: mode,
  label: t(`admin.groups.compositeRoutes.destinationModes.${mode}`)
})))
const keyword = ref('')
const results = ref<TargetOption[]>([])
// Cache only display metadata, never account credentials or complete API objects.
const knownTargets = ref<Record<string, TargetOption>>({})
const loading = ref(false)
const error = ref(false)
const showValidationError = ref(false)
const selectedId = computed(() => props.modelValue.mode === 'group'
  ? props.modelValue.target_group_id : props.modelValue.target_account_id)
const contextKey = computed(() => `${props.sourceGroupId}:${props.platform}:${props.modelValue.mode}`)
const selectedTarget = computed(() => knownTargets.value[`${contextKey.value}:${selectedId.value}`])
const selectedLabel = computed(() => selectedTarget.value
  ? `${selectedTarget.value.name} #${selectedId.value}` : `#${selectedId.value}`)
const statusLabel = (target: TargetOption) => {
  const status = target.status === 'active' && target.schedulable !== false
    ? t('admin.groups.compositeRoutes.destinationActive')
    : t('admin.groups.compositeRoutes.destinationUnavailable', { status: target.status })
  return target.reason ? `${status} · ${target.reason}` : status
}
const selectedStatus = computed(() => selectedTarget.value
  ? statusLabel(selectedTarget.value) : t('admin.groups.compositeRoutes.destinationUnknown'))

const searchRunner = useKeyedDebouncedSearch<TargetOption[]>({
  delay: 300,
  search: async (search, { signal }) => {
    const platform = props.platform
    if (props.modelValue.mode === 'group') {
      const response = await adminAPI.groups.list(1, 50, { platform, search }, { signal })
      return response.items
        .filter(group => eligibleCompositeTargetGroup(group, platform, props.sourceGroupId))
        .map(group => ({ id: group.id, name: group.name, status: group.status }))
    }
    const response = await adminAPI.accounts.list(1, 50, {
      platform, search, lite: '1', include_scheduler_score: '0'
    }, { signal })
    return response.items.filter(account => account.platform === platform).map(account => ({
      id: account.id, name: account.name, status: account.status,
      schedulable: account.schedulable,
      reason: account.temp_unschedulable_reason || account.error_message
    }))
  },
  onSuccess: (_key, targets) => {
    results.value = targets
    for (const target of targets) knownTargets.value[`${contextKey.value}:${target.id}`] = target
    loading.value = false
  },
  onError: () => {
    error.value = true
    loading.value = false
  }
})

const searchTargets = () => {
  searchRunner.clearAll()
  results.value = []
  error.value = false
  loading.value = props.modelValue.mode !== 'pool'
  if (loading.value) searchRunner.trigger(contextKey.value, keyword.value.trim())
}
watch(contextKey, () => {
  keyword.value = ''
  showValidationError.value = false
  searchTargets()
}, { immediate: true })
watch(keyword, searchTargets)
watch(selectedId, () => { showValidationError.value = false })

const changeMode = (mode: CompositeDestinationMode) => {
  if (mode !== props.modelValue.mode) emit('update:modelValue', changeCompositeDestinationMode(mode))
}
const selectTarget = (target: TargetOption) => {
  emit('update:modelValue', {
    mode: props.modelValue.mode,
    target_group_id: props.modelValue.mode === 'group' ? target.id : null,
    target_account_id: props.modelValue.mode === 'account' ? target.id : null
  })
  showValidationError.value = false
}
const validate = () => {
  showValidationError.value = !validCompositeDestination(props.modelValue, props.sourceGroupId)
  return !showValidationError.value
}
defineExpose({ validate })
</script>
