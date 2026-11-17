<template>
  <div class="flex w-[15ch] max-w-[15ch] flex-col items-start gap-1" :title="details" data-testid="session-name-cell">
    <span class="block w-full truncate text-sm">{{ row.session_name || '—' }}</span>
    <span v-if="kindLabel" class="rounded bg-blue-50 px-1.5 py-0.5 text-xs text-blue-700 dark:bg-blue-900/30 dark:text-blue-300">{{ kindLabel }}</span>
    <span v-if="!exact" class="rounded bg-amber-50 px-1.5 py-0.5 text-xs text-amber-700 dark:bg-amber-900/30 dark:text-amber-300">{{ labels.threadMissing }}</span>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { usePersonalUsageLabels, type PersonalUsageLog } from './index'
const props = defineProps<{ row: PersonalUsageLog }>()
const labels = usePersonalUsageLabels()
const exact = computed(() => props.row.session_identity?.match === 'thread')
const kindLabel = computed(() => {
  const identity = props.row.session_identity
  if (identity?.subagent === 'guardian') return labels.value.guardian
  if (identity?.subagent) return `${labels.value.subagent}${identity.subagent === 'subagent' ? '' : ` · ${identity.subagent}`}`
  if (identity?.kind === 'memory') return labels.value.memory
  if (identity?.kind && !['turn', 'prewarm', 'compaction'].includes(identity.kind)) return identity.kind
  return ''
})
const details = computed(() => [
  props.row.session_name || labels.value.noTitle,
  exact.value ? labels.value.exactThread : labels.value.identityMissing,
  props.row.session_identity?.thread_id && `Thread ID: ${props.row.session_identity.thread_id}`,
  props.row.session_identity?.parent_thread_id && `Parent Thread ID: ${props.row.session_identity.parent_thread_id}`,
  props.row.session_id && `Session ID: ${props.row.session_id}`,
].filter(Boolean).join('\n'))
</script>
