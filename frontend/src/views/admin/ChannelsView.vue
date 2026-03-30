<template>
  <AppLayout>
    <TablePageLayout>
      <template #filters>
        <div class="flex flex-col justify-between gap-4 lg:flex-row lg:items-start">
          <!-- Left: Search + Filters -->
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
                :placeholder="t('admin.channels.searchChannels', 'Search channels...')"
                class="input pl-10"
                @input="handleSearch"
              />
            </div>

            <Select
              v-model="filters.status"
              :options="statusFilterOptions"
              :placeholder="t('admin.channels.allStatus', 'All Status')"
              class="w-40"
              @change="loadChannels"
            />
          </div>

          <!-- Right: Actions -->
          <div class="flex w-full flex-shrink-0 flex-wrap items-center justify-end gap-3 lg:w-auto">
            <button
              @click="loadChannels"
              :disabled="loading"
              class="btn btn-secondary"
              :title="t('common.refresh', 'Refresh')"
            >
              <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
            </button>
            <button @click="openCreateDialog" class="btn btn-primary">
              <Icon name="plus" size="md" class="mr-2" />
              {{ t('admin.channels.createChannel', 'Create Channel') }}
            </button>
          </div>
        </div>
      </template>

      <template #table>
        <DataTable :columns="columns" :data="channels" :loading="loading">
          <template #cell-name="{ value }">
            <span class="font-medium text-gray-900 dark:text-white">{{ value }}</span>
          </template>

          <template #cell-description="{ value }">
            <span class="text-sm text-gray-600 dark:text-gray-400">{{ value || '-' }}</span>
          </template>

          <template #cell-status="{ value }">
            <span
              :class="[
                'inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium',
                value === 'active'
                  ? 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-400'
                  : 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400'
              ]"
            >
              {{ value === 'active' ? t('admin.channels.statusActive', 'Active') : t('admin.channels.statusDisabled', 'Disabled') }}
            </span>
          </template>

          <template #cell-group_count="{ row }">
            <span
              class="inline-flex items-center rounded bg-gray-100 px-2 py-0.5 text-xs font-medium text-gray-800 dark:bg-dark-600 dark:text-gray-300"
            >
              {{ (row.group_ids || []).length }}
              {{ t('admin.channels.groupsUnit', 'groups') }}
            </span>
          </template>

          <template #cell-pricing_count="{ row }">
            <span
              class="inline-flex items-center rounded bg-gray-100 px-2 py-0.5 text-xs font-medium text-gray-800 dark:bg-dark-600 dark:text-gray-300"
            >
              {{ (row.model_pricing || []).length }}
              {{ t('admin.channels.pricingUnit', 'pricing rules') }}
            </span>
          </template>

          <template #cell-created_at="{ value }">
            <span class="text-sm text-gray-600 dark:text-gray-400">
              {{ formatDate(value) }}
            </span>
          </template>

          <template #cell-actions="{ row }">
            <div class="flex items-center gap-1">
              <button
                @click="openEditDialog(row)"
                class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-gray-100 hover:text-primary-600 dark:hover:bg-dark-700 dark:hover:text-primary-400"
              >
                <Icon name="edit" size="sm" />
                <span class="text-xs">{{ t('common.edit', 'Edit') }}</span>
              </button>
              <button
                @click="handleDelete(row)"
                class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-900/20 dark:hover:text-red-400"
              >
                <Icon name="trash" size="sm" />
                <span class="text-xs">{{ t('common.delete', 'Delete') }}</span>
              </button>
            </div>
          </template>

          <template #empty>
            <EmptyState
              :title="t('admin.channels.noChannelsYet', 'No Channels Yet')"
              :description="t('admin.channels.createFirstChannel', 'Create your first channel to manage model pricing')"
              :action-text="t('admin.channels.createChannel', 'Create Channel')"
              @action="openCreateDialog"
            />
          </template>
        </DataTable>
      </template>

      <template #pagination>
        <Pagination
          v-if="pagination.total > 0"
          :page="pagination.page"
          :total="pagination.total"
          :page-size="pagination.page_size"
          @update:page="handlePageChange"
          @update:pageSize="handlePageSizeChange"
        />
      </template>
    </TablePageLayout>

    <!-- Create/Edit Dialog -->
    <BaseDialog
      :show="showDialog"
      :title="editingChannel ? t('admin.channels.editChannel', 'Edit Channel') : t('admin.channels.createChannel', 'Create Channel')"
      width="extra-wide"
      @close="closeDialog"
    >
      <form id="channel-form" @submit.prevent="handleSubmit" class="space-y-5">
        <!-- Name -->
        <div>
          <label class="input-label">{{ t('admin.channels.form.name', 'Name') }} <span class="text-red-500">*</span></label>
          <input
            v-model="form.name"
            type="text"
            required
            class="input"
            :placeholder="t('admin.channels.form.namePlaceholder', 'Enter channel name')"
          />
        </div>

        <!-- Description -->
        <div>
          <label class="input-label">{{ t('admin.channels.form.description', 'Description') }}</label>
          <textarea
            v-model="form.description"
            rows="2"
            class="input"
            :placeholder="t('admin.channels.form.descriptionPlaceholder', 'Optional description')"
          ></textarea>
        </div>

        <!-- Status (edit only) -->
        <div v-if="editingChannel">
          <label class="input-label">{{ t('admin.channels.form.status', 'Status') }}</label>
          <Select v-model="form.status" :options="statusEditOptions" />
        </div>

        <!-- Model Restriction -->
        <div>
          <label class="flex items-center gap-2 cursor-pointer">
            <input
              type="checkbox"
              v-model="form.restrict_models"
              class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500"
            />
            <span class="input-label mb-0">{{ t('admin.channels.form.restrictModels', 'Restrict Models') }}</span>
          </label>
          <p class="mt-1 ml-6 text-xs text-gray-400">
            {{ t('admin.channels.form.restrictModelsHint', 'When enabled, only models in the pricing list are allowed. Others will be rejected.') }}
          </p>
        </div>

        <!-- Group Association -->
        <div>
          <label class="input-label">
            {{ t('admin.channels.form.groups', 'Associated Groups') }}
            <span v-if="selectedGroupCount > 0" class="ml-1 text-xs font-normal text-gray-400">
              ({{ t('admin.channels.form.selectedCount', { count: selectedGroupCount }, `已选 ${selectedGroupCount} 个`) }})
            </span>
          </label>
          <div class="relative mb-2">
            <Icon
              name="search"
              size="md"
              class="absolute left-3 top-1/2 -translate-y-1/2 text-gray-400 dark:text-gray-500"
            />
            <input
              v-model="groupSearchQuery"
              type="text"
              :placeholder="t('admin.channels.form.searchGroups', 'Search groups...')"
              class="input pl-10"
            />
          </div>
          <div
            class="max-h-64 overflow-auto rounded-lg border border-gray-200 bg-white p-2 dark:border-dark-600 dark:bg-dark-800"
          >
            <div v-if="groupsLoading" class="py-4 text-center text-sm text-gray-500">
              {{ t('common.loading', 'Loading...') }}
            </div>
            <div v-else-if="allGroups.length === 0" class="py-4 text-center text-sm text-gray-500">
              {{ t('admin.channels.form.noGroupsAvailable', 'No groups available') }}
            </div>
            <div v-else-if="groupsByPlatform.length === 0" class="py-4 text-center text-sm text-gray-500">
              {{ t('admin.channels.form.noGroupsMatch', 'No groups match your search') }}
            </div>
            <template v-else>
              <div
                v-for="section in groupsByPlatform"
                :key="section.platform"
                class="mb-2 last:mb-0"
              >
                <!-- Platform header -->
                <div class="flex items-center gap-1.5 px-2 py-1">
                  <PlatformIcon :platform="section.platform" size="sm" :class="getPlatformTextColor(section.platform)" />
                  <span :class="['text-xs font-semibold', getPlatformTextColor(section.platform)]">
                    {{ t('admin.groups.platforms.' + section.platform, section.platform) }}
                  </span>
                </div>
                <!-- Groups under this platform -->
                <label
                  v-for="group in section.groups"
                  :key="group.id"
                  class="flex cursor-pointer items-center gap-2 rounded px-2 py-1.5 pl-7 hover:bg-gray-50 dark:hover:bg-dark-700"
                  :class="{ 'opacity-50': isGroupInOtherChannel(group.id) }"
                >
                  <input
                    type="checkbox"
                    :checked="form.group_ids.includes(group.id)"
                    :disabled="isGroupInOtherChannel(group.id)"
                    class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500"
                    @change="toggleGroup(group.id)"
                  />
                  <span class="text-sm text-gray-700 dark:text-gray-300">{{ group.name }}</span>
                  <span
                    :class="['ml-auto rounded-full px-1.5 py-0.5 text-[10px] font-medium', getRateBadgeClass(group.platform)]"
                  >
                    {{ group.rate_multiplier }}x
                  </span>
                  <span class="text-xs text-gray-400">
                    {{ group.account_count || 0 }}
                  </span>
                  <span
                    v-if="isGroupInOtherChannel(group.id)"
                    class="text-xs text-gray-400"
                  >
                    {{ getGroupInOtherChannelLabel(group.id) }}
                  </span>
                </label>
              </div>
            </template>
          </div>
        </div>

        <!-- Model Pricing -->
        <div>
          <div class="mb-2 flex items-center justify-between">
            <label class="input-label mb-0">{{ t('admin.channels.form.modelPricing', 'Model Pricing') }} <span class="text-red-500">*</span></label>
            <button type="button" @click="addPricingEntry" class="btn btn-secondary btn-sm">
              <Icon name="plus" size="sm" class="mr-1" />
              {{ t('common.add', 'Add') }}
            </button>
          </div>

          <div
            v-if="form.model_pricing.length === 0"
            class="rounded-lg border border-dashed border-gray-300 p-4 text-center text-sm text-gray-500 dark:border-dark-500 dark:text-gray-400"
          >
            {{ t('admin.channels.form.noPricingRules', 'No pricing rules yet. Click "Add" to create one.') }}
          </div>

          <div v-else class="space-y-3">
            <PricingEntryCard
              v-for="(entry, idx) in form.model_pricing"
              :key="idx"
              :entry="entry"
              @update="updatePricingEntry(idx, $event)"
              @remove="removePricingEntry(idx)"
            />
          </div>
        </div>

        <!-- Billing Model Source -->
        <div>
          <label class="input-label">{{ t('admin.channels.form.billingModelSource', 'Billing Model') }}</label>
          <Select v-model="form.billing_model_source" :options="billingModelSourceOptions" />
          <p class="mt-1 text-xs text-gray-400">
            {{ t('admin.channels.form.billingModelSourceHint', 'Controls which model name is used for pricing lookup') }}
          </p>
        </div>

        <!-- Model Mapping -->
        <div>
          <div class="mb-2 flex items-center justify-between">
            <label class="input-label mb-0">{{ t('admin.channels.form.modelMapping', 'Model Mapping') }}</label>
            <button type="button" @click="addMappingEntry" class="btn btn-secondary btn-sm">
              <Icon name="plus" size="sm" class="mr-1" />
              {{ t('common.add', 'Add') }}
            </button>
          </div>
          <p class="mb-2 text-xs text-gray-400">
            {{ t('admin.channels.form.modelMappingHint', 'Map request model names to actual model names. Runs before account-level mapping.') }}
          </p>
          <div
            v-if="Object.keys(form.model_mapping).length === 0"
            class="rounded-lg border border-dashed border-gray-300 p-4 text-center text-sm text-gray-500 dark:border-dark-500 dark:text-gray-400"
          >
            {{ t('admin.channels.form.noMappingRules', 'No mapping rules. Click "Add" to create one.') }}
          </div>
          <div v-else class="space-y-2">
            <div
              v-for="(_, srcModel) in form.model_mapping"
              :key="srcModel"
              class="flex items-center gap-2"
            >
              <input
                :value="srcModel"
                type="text"
                class="input flex-1 text-sm"
                :placeholder="t('admin.channels.form.mappingSource', 'Source model')"
                @change="renameMappingKey(srcModel, ($event.target as HTMLInputElement).value)"
              />
              <span class="text-gray-400">→</span>
              <input
                :value="form.model_mapping[srcModel]"
                type="text"
                class="input flex-1 text-sm"
                :placeholder="t('admin.channels.form.mappingTarget', 'Target model')"
                @input="form.model_mapping[srcModel] = ($event.target as HTMLInputElement).value"
              />
              <button
                type="button"
                @click="removeMappingEntry(srcModel)"
                class="rounded p-1 text-gray-400 hover:text-red-500"
              >
                <Icon name="trash" size="sm" />
              </button>
            </div>
          </div>
        </div>
      </form>

      <template #footer>
        <div class="flex justify-end gap-3">
          <button @click="closeDialog" type="button" class="btn btn-secondary">
            {{ t('common.cancel', 'Cancel') }}
          </button>
          <button
            type="submit"
            form="channel-form"
            :disabled="submitting"
            class="btn btn-primary"
          >
            {{ submitting
              ? t('common.submitting', 'Submitting...')
              : editingChannel
                ? t('common.update', 'Update')
                : t('common.create', 'Create')
            }}
          </button>
        </div>
      </template>
    </BaseDialog>

    <!-- Delete Confirmation -->
    <ConfirmDialog
      :show="showDeleteDialog"
      :title="t('admin.channels.deleteChannel', 'Delete Channel')"
      :message="deleteConfirmMessage"
      :confirm-text="t('common.delete', 'Delete')"
      :cancel-text="t('common.cancel', 'Cancel')"
      :danger="true"
      @confirm="confirmDelete"
      @cancel="showDeleteDialog = false"
    />
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted, onUnmounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { adminAPI } from '@/api/admin'
import type { Channel, ChannelModelPricing, CreateChannelRequest, UpdateChannelRequest } from '@/api/admin/channels'
import type { PricingFormEntry } from '@/components/admin/channel/types'
import { mTokToPerToken, perTokenToMTok, apiIntervalsToForm, formIntervalsToAPI } from '@/components/admin/channel/types'
import type { AdminGroup, GroupPlatform } from '@/types'
import type { Column } from '@/components/common/types'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import DataTable from '@/components/common/DataTable.vue'
import Pagination from '@/components/common/Pagination.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import Select from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'
import PlatformIcon from '@/components/common/PlatformIcon.vue'
import PricingEntryCard from '@/components/admin/channel/PricingEntryCard.vue'
import { getPersistedPageSize } from '@/composables/usePersistedPageSize'

const { t } = useI18n()
const appStore = useAppStore()

// ── Table columns ──
const columns = computed<Column[]>(() => [
  { key: 'name', label: t('admin.channels.columns.name', 'Name'), sortable: true },
  { key: 'description', label: t('admin.channels.columns.description', 'Description'), sortable: false },
  { key: 'status', label: t('admin.channels.columns.status', 'Status'), sortable: true },
  { key: 'group_count', label: t('admin.channels.columns.groups', 'Groups'), sortable: false },
  { key: 'pricing_count', label: t('admin.channels.columns.pricing', 'Pricing'), sortable: false },
  { key: 'created_at', label: t('admin.channels.columns.createdAt', 'Created'), sortable: true },
  { key: 'actions', label: t('admin.channels.columns.actions', 'Actions'), sortable: false }
])

const statusFilterOptions = computed(() => [
  { value: '', label: t('admin.channels.allStatus', 'All Status') },
  { value: 'active', label: t('admin.channels.statusActive', 'Active') },
  { value: 'disabled', label: t('admin.channels.statusDisabled', 'Disabled') }
])

const statusEditOptions = computed(() => [
  { value: 'active', label: t('admin.channels.statusActive', 'Active') },
  { value: 'disabled', label: t('admin.channels.statusDisabled', 'Disabled') }
])

const billingModelSourceOptions = computed(() => [
  { value: 'requested', label: t('admin.channels.form.billingModelSourceRequested', 'Bill by requested model') },
  { value: 'upstream', label: t('admin.channels.form.billingModelSourceUpstream', 'Bill by final upstream model') }
])

// ── State ──
const channels = ref<Channel[]>([])
const loading = ref(false)
const searchQuery = ref('')
const filters = reactive({ status: '' })
const pagination = reactive({
  page: 1,
  page_size: getPersistedPageSize(),
  total: 0
})

// Dialog state
const showDialog = ref(false)
const editingChannel = ref<Channel | null>(null)
const submitting = ref(false)
const showDeleteDialog = ref(false)
const deletingChannel = ref<Channel | null>(null)

// Groups
const allGroups = ref<AdminGroup[]>([])
const groupsLoading = ref(false)
const groupSearchQuery = ref('')

// Form data
const form = reactive({
  name: '',
  description: '',
  status: 'active',
  restrict_models: false,
  group_ids: [] as number[],
  model_pricing: [] as PricingFormEntry[],
  model_mapping: {} as Record<string, string>,
  billing_model_source: 'requested' as string
})

let abortController: AbortController | null = null

// ── Helpers ──
function formatDate(value: string): string {
  if (!value) return '-'
  return new Date(value).toLocaleDateString()
}

// ── Group helpers ──
// Platform color helpers
const platformOrder: GroupPlatform[] = ['anthropic', 'openai', 'gemini', 'antigravity', 'sora']

function getPlatformTextColor(platform: string): string {
  switch (platform) {
    case 'anthropic': return 'text-orange-600 dark:text-orange-400'
    case 'openai': return 'text-emerald-600 dark:text-emerald-400'
    case 'gemini': return 'text-blue-600 dark:text-blue-400'
    case 'antigravity': return 'text-purple-600 dark:text-purple-400'
    case 'sora': return 'text-rose-600 dark:text-rose-400'
    default: return 'text-gray-600 dark:text-gray-400'
  }
}

function getRateBadgeClass(platform: string): string {
  switch (platform) {
    case 'anthropic': return 'bg-orange-100 text-orange-700 dark:bg-orange-900/30 dark:text-orange-400'
    case 'openai': return 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-400'
    case 'gemini': return 'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400'
    case 'antigravity': return 'bg-purple-100 text-purple-700 dark:bg-purple-900/30 dark:text-purple-400'
    case 'sora': return 'bg-rose-100 text-rose-700 dark:bg-rose-900/30 dark:text-rose-400'
    default: return 'bg-gray-100 text-gray-700 dark:bg-gray-900/30 dark:text-gray-400'
  }
}

const groupsByPlatform = computed(() => {
  const query = groupSearchQuery.value.trim().toLowerCase()
  const groups = query
    ? allGroups.value.filter(g => g.name.toLowerCase().includes(query))
    : allGroups.value

  const grouped = new Map<GroupPlatform, typeof groups>()
  for (const g of groups) {
    const platform = g.platform
    if (!platform) continue
    if (!grouped.has(platform)) grouped.set(platform, [])
    grouped.get(platform)!.push(g)
  }

  // Sort by platformOrder
  const result: Array<{ platform: GroupPlatform; groups: typeof groups }> = []
  for (const p of platformOrder) {
    const list = grouped.get(p)
    if (list && list.length > 0) {
      result.push({ platform: p, groups: list })
    }
  }
  // Add any remaining platforms not in platformOrder
  for (const [p, list] of grouped) {
    if (!platformOrder.includes(p) && list.length > 0) {
      result.push({ platform: p, groups: list })
    }
  }
  return result
})

const selectedGroupCount = computed(() => form.group_ids.length)

const groupToChannelMap = computed(() => {
  const map = new Map<number, Channel>()
  for (const ch of channels.value) {
    if (editingChannel.value && ch.id === editingChannel.value.id) continue
    for (const gid of ch.group_ids || []) {
      map.set(gid, ch)
    }
  }
  return map
})

function isGroupInOtherChannel(groupId: number): boolean {
  return groupToChannelMap.value.has(groupId)
}

function getGroupChannelName(groupId: number): string {
  return groupToChannelMap.value.get(groupId)?.name || ''
}

function getGroupInOtherChannelLabel(groupId: number): string {
  const name = getGroupChannelName(groupId)
  return t('admin.channels.form.inOtherChannel', { name }, `In "${name}"`)
}

const deleteConfirmMessage = computed(() => {
  const name = deletingChannel.value?.name || ''
  return t(
    'admin.channels.deleteConfirm',
    { name },
    `Are you sure you want to delete channel "${name}"? This action cannot be undone.`
  )
})

function toggleGroup(groupId: number) {
  const idx = form.group_ids.indexOf(groupId)
  if (idx >= 0) {
    form.group_ids.splice(idx, 1)
  } else {
    form.group_ids.push(groupId)
  }
}

// ── Pricing helpers ──
function addPricingEntry() {
  form.model_pricing.push({
    models: [],
    billing_mode: 'token',
    input_price: null,
    output_price: null,
    cache_write_price: null,
    cache_read_price: null,
    image_output_price: null,
    per_request_price: null,
    intervals: []
  })
}

function updatePricingEntry(idx: number, updated: PricingFormEntry) {
  form.model_pricing[idx] = updated
}

function removePricingEntry(idx: number) {
  form.model_pricing.splice(idx, 1)
}

function formPricingToAPI(): ChannelModelPricing[] {
  return form.model_pricing
    .filter(e => e.models.length > 0)
    .map(e => ({
      models: e.models,
      billing_mode: e.billing_mode,
      input_price: mTokToPerToken(e.input_price),
      output_price: mTokToPerToken(e.output_price),
      cache_write_price: mTokToPerToken(e.cache_write_price),
      cache_read_price: mTokToPerToken(e.cache_read_price),
      image_output_price: mTokToPerToken(e.image_output_price),
      per_request_price: e.per_request_price != null && e.per_request_price !== '' ? Number(e.per_request_price) : null,
      intervals: formIntervalsToAPI(e.intervals || [])
    }))
}

function apiPricingToForm(pricing: ChannelModelPricing[]): PricingFormEntry[] {
  return pricing.map(p => ({
    models: p.models || [],
    billing_mode: p.billing_mode,
    input_price: perTokenToMTok(p.input_price),
    output_price: perTokenToMTok(p.output_price),
    cache_write_price: perTokenToMTok(p.cache_write_price),
    cache_read_price: perTokenToMTok(p.cache_read_price),
    image_output_price: perTokenToMTok(p.image_output_price),
    per_request_price: p.per_request_price,
    intervals: apiIntervalsToForm(p.intervals || [])
  }))
}

// ── Model Mapping helpers ──
function addMappingEntry() {
  // Find a unique key
  let key = ''
  let i = 1
  while (key === '' || key in form.model_mapping) {
    key = `model-${i}`
    i++
  }
  form.model_mapping[key] = ''
}

function removeMappingEntry(key: string) {
  delete form.model_mapping[key]
}

function renameMappingKey(oldKey: string, newKey: string) {
  newKey = newKey.trim()
  if (!newKey || newKey === oldKey) return
  if (newKey in form.model_mapping) return // prevent duplicate keys
  const value = form.model_mapping[oldKey]
  delete form.model_mapping[oldKey]
  form.model_mapping[newKey] = value
}

// ── Load data ──
async function loadChannels() {
  if (abortController) abortController.abort()
  const ctrl = new AbortController()
  abortController = ctrl
  loading.value = true

  try {
    const response = await adminAPI.channels.list(pagination.page, pagination.page_size, {
      status: filters.status || undefined,
      search: searchQuery.value || undefined
    }, { signal: ctrl.signal })

    if (ctrl.signal.aborted || abortController !== ctrl) return
    channels.value = response.items || []
    pagination.total = response.total
  } catch (error: any) {
    if (error?.name === 'AbortError' || error?.code === 'ERR_CANCELED') return
    appStore.showError(t('admin.channels.loadError', 'Failed to load channels'))
    console.error('Error loading channels:', error)
  } finally {
    if (abortController === ctrl) {
      loading.value = false
      abortController = null
    }
  }
}

async function loadGroups() {
  groupsLoading.value = true
  try {
    allGroups.value = await adminAPI.groups.getAll()
  } catch (error) {
    console.error('Error loading groups:', error)
  } finally {
    groupsLoading.value = false
  }
}

let searchTimeout: ReturnType<typeof setTimeout>
function handleSearch() {
  clearTimeout(searchTimeout)
  searchTimeout = setTimeout(() => {
    pagination.page = 1
    loadChannels()
  }, 300)
}

function handlePageChange(page: number) {
  pagination.page = page
  loadChannels()
}

function handlePageSizeChange(pageSize: number) {
  pagination.page_size = pageSize
  pagination.page = 1
  loadChannels()
}

// ── Dialog ──
function resetForm() {
  form.name = ''
  form.description = ''
  form.status = 'active'
  form.restrict_models = false
  form.group_ids = []
  form.model_pricing = []
  form.model_mapping = {}
  form.billing_model_source = 'requested'
  groupSearchQuery.value = ''
}

function openCreateDialog() {
  editingChannel.value = null
  resetForm()
  loadGroups()
  showDialog.value = true
}

function openEditDialog(channel: Channel) {
  editingChannel.value = channel
  form.name = channel.name
  form.description = channel.description || ''
  form.status = channel.status
  form.restrict_models = channel.restrict_models || false
  form.group_ids = [...(channel.group_ids || [])]
  form.model_pricing = apiPricingToForm(channel.model_pricing || [])
  form.model_mapping = { ...(channel.model_mapping || {}) }
  form.billing_model_source = channel.billing_model_source || 'requested'
  loadGroups()
  showDialog.value = true
}

function closeDialog() {
  showDialog.value = false
  editingChannel.value = null
  resetForm()
}

async function handleSubmit() {
  if (submitting.value) return
  if (!form.name.trim()) {
    appStore.showError(t('admin.channels.nameRequired', 'Please enter a channel name'))
    return
  }

  // 检查模型重复
  const allModels = form.model_pricing.flatMap(e => e.models.map(m => m.toLowerCase()))
  const duplicates = allModels.filter((m, i) => allModels.indexOf(m) !== i)
  if (duplicates.length > 0) {
    appStore.showError(t('admin.channels.duplicateModels', `模型 "${duplicates[0]}" 在多个定价条目中重复`))
    return
  }

  submitting.value = true
  try {
    if (editingChannel.value) {
      const req: UpdateChannelRequest = {
        name: form.name.trim(),
        description: form.description.trim() || undefined,
        status: form.status,
        group_ids: form.group_ids,
        model_pricing: formPricingToAPI(),
        model_mapping: Object.keys(form.model_mapping).length > 0 ? form.model_mapping : undefined,
        billing_model_source: form.billing_model_source,
        restrict_models: form.restrict_models
      }
      await adminAPI.channels.update(editingChannel.value.id, req)
      appStore.showSuccess(t('admin.channels.updateSuccess', 'Channel updated'))
    } else {
      const req: CreateChannelRequest = {
        name: form.name.trim(),
        description: form.description.trim() || undefined,
        group_ids: form.group_ids,
        model_pricing: formPricingToAPI(),
        model_mapping: Object.keys(form.model_mapping).length > 0 ? form.model_mapping : undefined,
        billing_model_source: form.billing_model_source,
        restrict_models: form.restrict_models
      }
      await adminAPI.channels.create(req)
      appStore.showSuccess(t('admin.channels.createSuccess', 'Channel created'))
    }
    closeDialog()
    loadChannels()
  } catch (error: any) {
    const msg = error.response?.data?.detail || (editingChannel.value
      ? t('admin.channels.updateError', 'Failed to update channel')
      : t('admin.channels.createError', 'Failed to create channel'))
    appStore.showError(msg)
    console.error('Error saving channel:', error)
  } finally {
    submitting.value = false
  }
}

// ── Delete ──
function handleDelete(channel: Channel) {
  deletingChannel.value = channel
  showDeleteDialog.value = true
}

async function confirmDelete() {
  if (!deletingChannel.value) return

  try {
    await adminAPI.channels.remove(deletingChannel.value.id)
    appStore.showSuccess(t('admin.channels.deleteSuccess', 'Channel deleted'))
    showDeleteDialog.value = false
    deletingChannel.value = null
    loadChannels()
  } catch (error: any) {
    appStore.showError(error.response?.data?.detail || t('admin.channels.deleteError', 'Failed to delete channel'))
    console.error('Error deleting channel:', error)
  }
}

// ── Lifecycle ──
onMounted(() => {
  loadChannels()
})

onUnmounted(() => {
  clearTimeout(searchTimeout)
  abortController?.abort()
})
</script>
