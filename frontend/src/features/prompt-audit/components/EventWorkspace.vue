<template>
  <section aria-labelledby="prompt-events-title" class="py-6">
    <div class="flex flex-wrap items-start justify-between gap-3">
      <div>
        <h2 id="prompt-events-title" class="text-base font-semibold text-gray-950 dark:text-white">{{ t('admin.promptAudit.events.title') REDACTEDREDACTED</h2>
        <p class="mt-1 text-sm text-gray-500 dark:text-dark-300">{{ t('admin.promptAudit.events.description') REDACTEDREDACTED</p>
      </div>
      <div class="flex flex-wrap gap-2">
        <button type="button" class="btn btn-secondary btn-sm" :disabled="selectedIds.length === 0" @click="$emit('batch-delete')">
          {{ t('admin.promptAudit.events.deleteSelected', { count: selectedIds.length REDACTED) REDACTEDREDACTED
        </button>
        <button type="button" class="btn btn-danger btn-sm" :disabled="!hasExplicitDeleteRange(localFilters)" data-test="filter-delete" @click="$emit('preview-delete')">
          {{ t('admin.promptAudit.events.deleteByFilter') REDACTEDREDACTED
        </button>
      </div>
    </div>

    <form class="mt-5 grid gap-3 sm:grid-cols-2 lg:grid-cols-4 xl:grid-cols-5" @submit.prevent="applyFilters">
      <label class="text-xs text-gray-600 dark:text-dark-300">
        <span>{{ t('admin.promptAudit.events.decision') REDACTEDREDACTED</span>
        <select v-model="localFilters.decision" class="input mt-1 w-full" :aria-label="t('admin.promptAudit.events.decision')" @change="filtersChanged">
          <option value="">{{ t('common.all') REDACTEDREDACTED</option><option value="pass">pass</option><option value="flag">flag</option><option value="critical">critical</option>
        </select>
      </label>
      <label class="text-xs text-gray-600 dark:text-dark-300">
        <span>{{ t('admin.promptAudit.events.risk') REDACTEDREDACTED</span>
        <select v-model="localFilters.risk_level" class="input mt-1 w-full" :aria-label="t('admin.promptAudit.events.risk')" @change="filtersChanged">
          <option value="">{{ t('common.all') REDACTEDREDACTED</option><option value="low">low</option><option value="medium">medium</option><option value="high">high</option><option value="critical">critical</option>
        </select>
      </label>
      <FilterInput v-model="localFilters.endpoint" :label="t('admin.promptAudit.events.endpoint')" @change="filtersChanged" />
      <FilterInput v-model="localFilters.group_id" :label="t('admin.promptAudit.events.groupId')" type="number" @change="filtersChanged" />
      <FilterInput v-model="localFilters.user_id" :label="t('admin.promptAudit.events.userId')" type="number" @change="filtersChanged" />
      <FilterInput v-model="localFilters.api_key_id" :label="t('admin.promptAudit.events.apiKeyId')" type="number" @change="filtersChanged" />
      <FilterInput v-model="localFilters.request_id" label="Request ID" @change="filtersChanged" />
      <FilterInput v-model="localFilters.prompt_hash" label="Prompt SHA-256" @change="filtersChanged" />
      <FilterInput v-model="localFilters.keyword" :label="t('admin.promptAudit.events.keyword')" @change="filtersChanged" />
      <label class="text-xs text-gray-600 dark:text-dark-300">
        <span>{{ t('admin.promptAudit.events.startAt') REDACTEDREDACTED</span>
        <input v-model="localFilters.start_at" type="datetime-local" class="input mt-1 w-full" :aria-label="t('admin.promptAudit.events.startAt')" @change="filtersChanged" />
      </label>
      <label class="text-xs text-gray-600 dark:text-dark-300">
        <span>{{ t('admin.promptAudit.events.endAt') REDACTEDREDACTED</span>
        <input v-model="localFilters.end_at" type="datetime-local" class="input mt-1 w-full" :aria-label="t('admin.promptAudit.events.endAt')" @change="filtersChanged" />
      </label>
      <div class="flex items-end gap-2 sm:col-span-2">
        <button type="submit" class="btn btn-primary btn-sm">{{ t('common.search') REDACTEDREDACTED</button>
        <button type="button" class="btn btn-ghost btn-sm" @click="resetFilters">{{ t('common.reset') REDACTEDREDACTED</button>
      </div>
    </form>
    <p class="mt-2 text-xs text-gray-500 dark:text-dark-400">{{ t('admin.promptAudit.events.deleteRangeHint') REDACTEDREDACTED</p>

    <div v-if="error" role="alert" class="mt-4 rounded-lg bg-red-50 px-4 py-3 text-sm text-red-700 dark:bg-red-950/30 dark:text-red-300">{{ error REDACTEDREDACTED</div>
    <div class="mt-5 overflow-x-auto rounded-lg border border-gray-200 dark:border-dark-700">
      <table class="min-w-[1120px] w-full text-left text-sm">
        <thead class="bg-gray-50 text-xs uppercase tracking-wide text-gray-500 dark:bg-dark-900 dark:text-dark-400">
          <tr>
            <th class="w-10 px-3 py-3"><input type="checkbox" :checked="allSelected" :aria-label="t('admin.promptAudit.events.selectAll')" @change="toggleAll" /></th>
            <th class="px-3 py-3 font-medium">{{ t('admin.promptAudit.events.time') REDACTEDREDACTED</th>
            <th class="px-3 py-3 font-medium">{{ t('admin.promptAudit.events.identity') REDACTEDREDACTED</th>
            <th class="px-3 py-3 font-medium">{{ t('admin.promptAudit.events.group') REDACTEDREDACTED</th>
            <th class="px-3 py-3 font-medium">{{ t('admin.promptAudit.events.route') REDACTEDREDACTED</th>
            <th class="px-3 py-3 font-medium">{{ t('admin.promptAudit.events.result') REDACTEDREDACTED</th>
            <th class="px-3 py-3 font-medium">{{ t('admin.promptAudit.events.preview') REDACTEDREDACTED</th>
            <th class="px-3 py-3 text-right font-medium">{{ t('admin.promptAudit.common.actions') REDACTEDREDACTED</th>
          </tr>
        </thead>
        <tbody class="divide-y divide-gray-100 bg-white dark:divide-dark-800 dark:bg-dark-850">
          <tr v-if="loading"><td colspan="8" class="px-4 py-12 text-center text-gray-500" aria-busy="true">{{ t('common.loading') REDACTEDREDACTED</td></tr>
          <tr v-else-if="events.length === 0"><td colspan="8" class="px-4 py-12 text-center text-gray-500">{{ t('admin.promptAudit.events.empty') REDACTEDREDACTED</td></tr>
          <tr v-for="event in events" v-else :key="event.id" :data-test="`event-${event.idREDACTED`" class="align-top hover:bg-gray-50/70 dark:hover:bg-dark-800/70">
            <td class="px-3 py-3"><input type="checkbox" :checked="selectedIds.includes(event.id)" :aria-label="t('admin.promptAudit.events.selectEvent', { id: event.id REDACTED)" @change="toggleOne(event.id)" /></td>
            <td class="whitespace-nowrap px-3 py-3 text-xs text-gray-600 dark:text-dark-300">{{ formatDate(event.created_at) REDACTEDREDACTED</td>
            <td class="px-3 py-3">
              <CopyLine :label="t('admin.promptAudit.events.user')" :value="event.snapshot.username" />
              <CopyLine :label="t('admin.promptAudit.events.email')" :value="event.snapshot.user_email" />
              <CopyLine :label="t('admin.promptAudit.events.apiKey')" :value="event.snapshot.api_key_name" />
            </td>
            <td class="px-3 py-3 text-gray-700 dark:text-dark-200">{{ event.snapshot.group_name || '—' REDACTEDREDACTED</td>
            <td class="px-3 py-3">
              <p class="font-medium text-gray-900 dark:text-white">{{ event.snapshot.endpoint REDACTEDREDACTED</p>
              <p class="mt-1 text-xs text-gray-500">{{ event.snapshot.model REDACTEDREDACTED · {{ event.snapshot.protocol REDACTEDREDACTED · {{ event.snapshot.stage || 'http' REDACTEDREDACTED</p>
            </td>
            <td class="px-3 py-3">
              <span class="rounded-full px-2 py-0.5 text-xs font-medium" :class="decisionClass(event.decision)">{{ event.decision REDACTEDREDACTED · {{ event.risk_level REDACTEDREDACTED</span>
              <p class="mt-2 max-w-48 truncate text-xs text-gray-500">{{ event.categories.join(', ') || '—' REDACTEDREDACTED</p>
            </td>
            <td class="max-w-xs px-3 py-3"><p class="line-clamp-2 break-words text-gray-600 dark:text-dark-300">{{ event.snapshot.redacted_preview || '—' REDACTEDREDACTED</p></td>
            <td class="whitespace-nowrap px-3 py-3 text-right">
              <button type="button" class="btn btn-ghost btn-sm" @click="$emit('view', event.id)">{{ t('common.view') REDACTEDREDACTED</button>
              <button type="button" class="btn btn-ghost btn-sm text-red-600" @click="$emit('delete', event.id)">{{ t('common.delete') REDACTEDREDACTED</button>
            </td>
          </tr>
        </tbody>
      </table>
      <Pagination :total="total" :page="page" :page-size="pageSize" @update:page="$emit('page', $event)" @update:page-size="$emit('page-size', $event)" />
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, defineComponent, h, reactive, watch REDACTED from 'vue'
import { useI18n REDACTED from 'vue-i18n'
import Pagination from '@/components/common/Pagination.vue'
import type { PromptAuditEvent, PromptEventFilters REDACTED from '../types'
import { cloneData, emptyEventFilters, hasExplicitDeleteRange REDACTED from '../viewModel'

const props = defineProps<{
  events: PromptAuditEvent[]; total: number; page: number; pageSize: number
  filters: PromptEventFilters; selectedIds: number[]; loading: boolean; error: string
REDACTED>()
const emit = defineEmits<{
  (event: 'filters-change', value: PromptEventFilters): void
  (event: 'search', value: PromptEventFilters): void
  (event: 'selection', value: number[]): void
  (event: 'page', value: number): void
  (event: 'page-size', value: number): void
  (event: 'view', id: number): void
  (event: 'delete', id: number): void
  (event: 'batch-delete'): void
  (event: 'preview-delete'): void
REDACTED>()
const { t, locale REDACTED = useI18n()
const localFilters = reactive<PromptEventFilters>(cloneData(props.filters))
watch(() => props.filters, (value) => Object.assign(localFilters, cloneData(value)), { deep: true REDACTED)
const allSelected = computed(() => props.events.length > 0 && props.events.every((event) => props.selectedIds.includes(event.id)))

const FilterInput = defineComponent({
  props: { modelValue: { type: String, required: true REDACTED, label: { type: String, required: true REDACTED, type: { type: String, default: 'text' REDACTED REDACTED,
  emits: ['update:modelValue', 'change'],
  setup(componentProps, { emit: componentEmit REDACTED) {
    return () => h('label', { class: 'text-xs text-gray-600 dark:text-dark-300' REDACTED, [
      h('span', componentProps.label),
      h('input', {
        value: componentProps.modelValue, type: componentProps.type, class: 'input mt-1 w-full', 'aria-label': componentProps.label,
        onInput: (event: Event) => componentEmit('update:modelValue', (event.target as HTMLInputElement).value),
        onChange: () => componentEmit('change'),
      REDACTED),
    ])
  REDACTED,
REDACTED)

const CopyLine = defineComponent({
  props: { label: { type: String, required: true REDACTED, value: { type: String, default: '' REDACTED REDACTED,
  setup(componentProps) {
    return () => h('div', { class: 'flex max-w-56 items-center gap-1 text-xs' REDACTED, [
      h('span', { class: 'w-16 flex-none text-gray-500' REDACTED, componentProps.label),
      h('span', { class: 'min-w-0 flex-1 truncate text-gray-800 dark:text-dark-100' REDACTED, componentProps.value || '—'),
      componentProps.value ? h('button', {
        type: 'button', class: 'text-primary-600 hover:underline', 'aria-label': `${t('common.copy')REDACTED ${componentProps.labelREDACTED`,
        onClick: () => navigator.clipboard?.writeText(componentProps.value),
      REDACTED, t('common.copy')) : null,
    ])
  REDACTED,
REDACTED)

function filtersChanged() {
  emit('filters-change', cloneData(localFilters))
REDACTED
function applyFilters() {
  const value = cloneData(localFilters)
  emit('filters-change', value)
  emit('search', value)
REDACTED
function resetFilters() {
  Object.assign(localFilters, emptyEventFilters())
  applyFilters()
REDACTED
function toggleOne(id: number) {
  const selected = new Set(props.selectedIds)
  if (selected.has(id)) selected.delete(id)
  else selected.add(id)
  emit('selection', [...selected])
REDACTED
function toggleAll() {
  emit('selection', allSelected.value ? [] : props.events.map((event) => event.id))
REDACTED
function formatDate(value: string): string {
  return new Intl.DateTimeFormat(locale.value, { dateStyle: 'short', timeStyle: 'medium' REDACTED).format(new Date(value))
REDACTED
function decisionClass(decision: string): string {
  if (decision === 'critical') return 'bg-red-100 text-red-700 dark:bg-red-950/50 dark:text-red-300'
  if (decision === 'flag') return 'bg-amber-100 text-amber-700 dark:bg-amber-950/50 dark:text-amber-300'
  return 'bg-emerald-100 text-emerald-700 dark:bg-emerald-950/50 dark:text-emerald-300'
REDACTED
</script>
