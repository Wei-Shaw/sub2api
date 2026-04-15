<script setup lang="ts">
import { computed } from 'vue'
import OpsRequestDetailsModal from './OpsRequestDetailsModal.vue'

interface Props {
  modelValue: boolean
  timeRange: string
  platform?: string
  groupId?: number | null
  routingSelectedGroup?: string
  title: string
}

const props = defineProps<Props>()
const emit = defineEmits<{
  (e: 'update:modelValue', value: boolean): void
  (e: 'openErrorDetail', errorId: number): void
  (e: 'openRequestErrors', requestId: string): void
}>()

const preset = computed(() => ({
  title: props.title,
  kind: 'success' as const,
  sort: 'created_at_desc' as const,
  retried_only: true,
  routing_selected_group: props.routingSelectedGroup,
}))
</script>

<template>
  <OpsRequestDetailsModal
    :model-value="modelValue"
    :time-range="timeRange"
    :preset="preset"
    :platform="platform"
    :group-id="groupId"
    @update:model-value="emit('update:modelValue', $event)"
    @open-error-detail="emit('openErrorDetail', $event)"
    @open-request-errors="emit('openRequestErrors', $event)"
  />
</template>
