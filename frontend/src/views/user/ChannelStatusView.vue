<template>
  <AppLayout>
    <TablePageLayout>
      <template #filters>
        <div class="flex flex-col justify-between gap-4 lg:flex-row lg:items-start">
          <div class="flex flex-1 flex-wrap items-center gap-3">
            <div class="relative w-full sm:w-64">
              <Icon
                name="search"
                size="md"
                class="absolute left-3 top-1/2 -translate-y-1/2 text-gray-400 dark:text-gray-500"
              />
              <input
                v-model="searchQuery"
                type="text"
                :placeholder="t('channelStatus.searchPlaceholder')"
                class="input pl-10"
              />
            </div>

            <Select
              v-model="providerFilter"
              :options="providerFilterOptions"
              :placeholder="t('channelStatus.allProviders')"
              class="w-44"
            />
          </div>

          <div class="flex w-full flex-shrink-0 flex-wrap items-center justify-end gap-3 lg:w-auto">
            <button
              @click="reload"
              :disabled="loading"
              class="btn btn-secondary"
              :title="t('common.refresh')"
            >
              <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
            </button>
          </div>
        </div>
      </template>

      <template #table>
        <DataTable :columns="columns" :data="filteredItems" :loading="loading">
          <template #cell-name="{ row REDACTED">
            <button
              @click="openDetail(row)"
              class="font-medium text-primary-600 transition-colors hover:text-primary-700 dark:text-primary-400 dark:hover:text-primary-300"
            >
              {{ row.name REDACTEDREDACTED
            </button>
          </template>

          <template #cell-provider="{ row REDACTED">
            <span
              class="inline-flex items-center rounded-md px-2 py-0.5 text-xs font-medium"
              :class="providerBadgeClass(row.provider)"
            >
              {{ providerLabel(row.provider) REDACTEDREDACTED
            </span>
          </template>

          <template #cell-group_name="{ value REDACTED">
            <span class="text-sm text-gray-700 dark:text-gray-300">{{ value || '-' REDACTEDREDACTED</span>
          </template>

          <template #cell-primary_model="{ row REDACTED">
            <MonitorPrimaryModelCell :row="row" />
          </template>

          <template #cell-availability_7d="{ row REDACTED">
            <span class="text-sm text-gray-900 dark:text-gray-100">
              {{ formatAvailability(row) REDACTEDREDACTED
            </span>
          </template>

          <template #cell-latency="{ row REDACTED">
            <span class="text-sm text-gray-900 dark:text-gray-100">
              {{ formatLatency(row.primary_latency_ms) REDACTEDREDACTED
            </span>
          </template>

          <template #empty>
            <EmptyState
              :title="t('channelStatus.empty.title')"
              :description="t('channelStatus.empty.description')"
            />
          </template>
        </DataTable>
      </template>
    </TablePageLayout>

    <MonitorDetailDialog
      :show="showDetail"
      :monitor-id="detailTarget?.id ?? null"
      :title="detailTitle"
      @close="closeDetail"
    />
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, computed, onMounted REDACTED from 'vue'
import { useI18n REDACTED from 'vue-i18n'
import { useAppStore REDACTED from '@/stores/app'
import { extractApiErrorMessage REDACTED from '@/utils/apiError'
import {
  list as listChannelMonitorViews,
  type Provider,
  type UserMonitorView,
REDACTED from '@/api/channelMonitor'
import type { Column REDACTED from '@/components/common/types'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import DataTable from '@/components/common/DataTable.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import Select from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'
import MonitorDetailDialog from '@/components/user/MonitorDetailDialog.vue'
import MonitorPrimaryModelCell from '@/components/user/MonitorPrimaryModelCell.vue'
import { useChannelMonitorFormat REDACTED from '@/composables/useChannelMonitorFormat'
import {
  PROVIDER_OPENAI,
  PROVIDER_ANTHROPIC,
  PROVIDER_GEMINI,
REDACTED from '@/constants/channelMonitor'

const { t REDACTED = useI18n()
const appStore = useAppStore()
const {
  providerLabel,
  providerBadgeClass,
  formatLatency,
  formatAvailability,
REDACTED = useChannelMonitorFormat()

// ── State ──
const items = ref<UserMonitorView[]>([])
const loading = ref(false)
const searchQuery = ref('')
const providerFilter = ref<Provider | ''>('')

const showDetail = ref(false)
const detailTarget = ref<UserMonitorView | null>(null)

// ── Options ──
const providerFilterOptions = computed(() => [
  { value: '', label: t('channelStatus.allProviders') REDACTED,
  { value: PROVIDER_OPENAI, label: providerLabel(PROVIDER_OPENAI) REDACTED,
  { value: PROVIDER_ANTHROPIC, label: providerLabel(PROVIDER_ANTHROPIC) REDACTED,
  { value: PROVIDER_GEMINI, label: providerLabel(PROVIDER_GEMINI) REDACTED,
])

// ── Columns ──
const columns = computed<Column[]>(() => [
  { key: 'name', label: t('channelStatus.columns.name'), sortable: false REDACTED,
  { key: 'provider', label: t('channelStatus.columns.provider'), sortable: false REDACTED,
  { key: 'group_name', label: t('channelStatus.columns.groupName'), sortable: false REDACTED,
  { key: 'primary_model', label: t('channelStatus.columns.primaryModel'), sortable: false REDACTED,
  { key: 'availability_7d', label: t('channelStatus.columns.availability7d'), sortable: false REDACTED,
  { key: 'latency', label: t('channelStatus.columns.latency'), sortable: false REDACTED,
])

// ── Filtered data ──
const filteredItems = computed(() => {
  const q = searchQuery.value.trim().toLowerCase()
  return items.value.filter(it => {
    if (providerFilter.value && it.provider !== providerFilter.value) return false
    if (!q) return true
    return (
      it.name.toLowerCase().includes(q) ||
      (it.group_name || '').toLowerCase().includes(q) ||
      it.primary_model.toLowerCase().includes(q)
    )
  REDACTED)
REDACTED)

const detailTitle = computed(() => {
  return detailTarget.value?.name || t('channelStatus.detailTitle')
REDACTED)

// ── Loaders ──
async function reload() {
  loading.value = true
  try {
    const res = await listChannelMonitorViews()
    items.value = res.items || []
  REDACTED catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('channelStatus.loadError')))
  REDACTED finally {
    loading.value = false
  REDACTED
REDACTED

function openDetail(row: UserMonitorView) {
  detailTarget.value = row
  showDetail.value = true
REDACTED

function closeDetail() {
  showDetail.value = false
  detailTarget.value = null
REDACTED

// ── Lifecycle ──
onMounted(() => {
  reload()
REDACTED)
</script>
