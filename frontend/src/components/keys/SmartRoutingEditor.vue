<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import Select from '@/components/common/Select.vue'
import GroupBadge from '@/components/common/GroupBadge.vue'
import type { Group } from '@/types'

const props = defineProps<{
  enabled: boolean
  modelValue: number[]
  groups: Group[]
  rates: Record<number, number>
  fixedGroupId: number | null
}>()
const emit = defineEmits<{
  'update:enabled': [value: boolean]
  'update:modelValue': [value: number[]]
}>()
const { t } = useI18n()
const announcement = ref('')
const platforms = new Set(['openai', 'grok', 'kimi', 'zhipu', 'deepseek', 'minimax'])
const eligible = computed(() => props.groups.filter(g => platforms.has(g.platform) && g.status === 'active'))
const options = computed(() => eligible.value.filter(g => !props.modelValue.includes(g.id)).map(g => ({ value: g.id, label: g.name })))
const routes = computed(() => props.modelValue.map(id => ({ id, group: eligible.value.find(g => g.id === id) })))

function setMode(enabled: boolean) {
  if (enabled && !props.modelValue.length && eligible.value.some(g => g.id === props.fixedGroupId)) {
    emit('update:modelValue', [props.fixedGroupId!])
  }
  emit('update:enabled', enabled)
}
function add(value: string | number | boolean | null) {
  if (typeof value !== 'number' || props.modelValue.length >= 10 || !options.value.some(o => o.value === value)) return
  emit('update:modelValue', [...props.modelValue, value])
  announcement.value = t('keys.smartRouting.added')
}
function move(index: number, delta: number) {
  const next = [...props.modelValue]
  const target = index + delta
  if (target < 0 || target >= next.length) return
  ;[next[index], next[target]] = [next[target]!, next[index]!]
  emit('update:modelValue', next)
  announcement.value = t('keys.smartRouting.moved', { position: target + 1 })
}
function remove(id: number) {
  emit('update:modelValue', props.modelValue.filter(value => value !== id))
  announcement.value = t('keys.smartRouting.removed')
}
</script>

<template>
  <div class="space-y-4" data-testid="smart-routing-editor">
    <div class="grid grid-cols-2 gap-2 rounded-xl bg-gray-100 p-1 dark:bg-dark-900" role="group" :aria-label="t('keys.smartRouting.mode')">
      <button v-for="smart in [false, true]" :key="String(smart)" type="button" :aria-pressed="enabled === smart"
        class="rounded-lg px-3 py-2.5 text-sm font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-500"
        :class="enabled === smart ? 'bg-white text-primary-700 shadow-sm dark:bg-dark-700 dark:text-primary-300' : 'text-gray-500 hover:text-gray-800 dark:text-gray-400 dark:hover:text-gray-200'"
        @click="setMode(smart)">
        {{ t(smart ? 'keys.smartRouting.title' : 'keys.smartRouting.fixed') }}
      </button>
    </div>

    <div v-if="enabled" class="space-y-3 rounded-xl border border-primary-200 bg-primary-50/40 p-4 dark:border-primary-800/60 dark:bg-primary-950/20">
      <div class="flex items-center justify-between gap-3">
        <h3 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('keys.smartRouting.order') }}</h3>
        <span class="rounded-full bg-white px-2 py-0.5 text-xs tabular-nums text-gray-500 dark:bg-dark-800 dark:text-gray-400">{{ modelValue.length }} / 10</span>
      </div>
      <p class="text-xs leading-5 text-gray-600 dark:text-gray-400">{{ t('keys.smartRouting.description') }}</p>

      <ol v-if="routes.length" class="space-y-2" :aria-label="t('keys.smartRouting.order')">
        <li v-for="(route, index) in routes" :key="route.id" class="flex items-center gap-2 rounded-lg border bg-white p-2.5 dark:bg-dark-800"
          :class="route.group ? 'border-gray-200 dark:border-dark-600' : 'border-amber-400 dark:border-amber-700'">
          <span class="flex h-6 w-6 shrink-0 items-center justify-center rounded-full text-xs font-semibold tabular-nums"
            :class="index === 0 ? 'bg-primary-100 text-primary-700 dark:bg-primary-900 dark:text-primary-200' : 'bg-gray-100 text-gray-500 dark:bg-dark-700 dark:text-gray-400'">{{ index + 1 }}</span>
          <div class="min-w-0 flex-1">
            <GroupBadge v-if="route.group" class="max-w-full" :name="route.group.name" :platform="route.group.platform"
              :subscription-type="route.group.subscription_type" :rate-multiplier="route.group.rate_multiplier"
              :user-rate-multiplier="rates[route.id]" :peak-rate-enabled="route.group.peak_rate_enabled"
              :peak-start="route.group.peak_start" :peak-end="route.group.peak_end" :peak-rate-multiplier="route.group.peak_rate_multiplier" />
            <span v-else class="text-xs text-amber-700 dark:text-amber-300">{{ t('keys.smartRouting.unavailable', { id: route.id }) }}</span>
            <p v-if="index === 0" class="mt-1 pl-0.5 text-[11px] text-primary-600 dark:text-primary-400">{{ t('keys.smartRouting.first') }}</p>
          </div>
          <div class="flex shrink-0 items-center gap-0.5">
            <button type="button" class="route-action" :disabled="index === 0" :aria-label="t('keys.smartRouting.moveUp', { name: route.group?.name ?? route.id })" :title="t('keys.smartRouting.up')" @click="move(index, -1)">
              <svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true"><path d="m6 12 6-6 6 6M12 6v13" /></svg>
            </button>
            <button type="button" class="route-action" :disabled="index === routes.length - 1" :aria-label="t('keys.smartRouting.moveDown', { name: route.group?.name ?? route.id })" :title="t('keys.smartRouting.down')" @click="move(index, 1)">
              <svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true"><path d="m6 12 6 6 6-6M12 18V5" /></svg>
            </button>
            <button type="button" class="route-action hover:!text-red-600" :aria-label="t('keys.smartRouting.removeGroup', { name: route.group?.name ?? route.id })" :title="t('keys.smartRouting.remove')" @click="remove(route.id)">
              <svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true"><path d="m6 6 12 12M6 18 18 6" /></svg>
            </button>
          </div>
        </li>
      </ol>
      <div v-else class="rounded-lg border border-dashed border-primary-200 bg-white/70 px-4 py-5 text-center dark:border-primary-800 dark:bg-dark-800/50">
        <p class="text-sm font-medium text-gray-700 dark:text-gray-200">{{ t('keys.smartRouting.empty') }}</p>
        <p class="mt-1 text-xs text-gray-500">{{ t('keys.smartRouting.emptyHint') }}</p>
      </div>
      <Select :model-value="null" :options="options" :searchable="true" :disabled="modelValue.length >= 10 || !options.length"
        :aria-label="t('keys.smartRouting.add')" :placeholder="t(modelValue.length >= 10 ? 'keys.smartRouting.limit' : options.length ? 'keys.smartRouting.add' : 'keys.smartRouting.noGroups')"
        :search-placeholder="t('keys.searchGroup')" @update:model-value="add" />
      <div class="border-t border-primary-100 pt-3 dark:border-primary-900">
        <p class="text-xs font-medium text-gray-700 dark:text-gray-300">{{ t('keys.smartRouting.billing') }}</p>
        <p class="mt-1 text-xs leading-5 text-gray-500 dark:text-gray-400">{{ t('keys.smartRouting.scope') }}</p>
      </div>
    </div>
    <span class="sr-only" role="status" aria-live="polite">{{ announcement }}</span>
  </div>
</template>

<style scoped>
.route-action {
  @apply inline-flex h-9 w-8 items-center justify-center rounded-md text-gray-400 transition-colors hover:bg-gray-100 hover:text-gray-700 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-500 disabled:cursor-not-allowed disabled:opacity-25 dark:hover:bg-dark-700 dark:hover:text-gray-200;
}
</style>
