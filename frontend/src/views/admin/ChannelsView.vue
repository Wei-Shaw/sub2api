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

          <template #cell-status="{ row }">
            <Toggle
              :modelValue="row.status === 'active'"
              @update:modelValue="toggleChannelStatus(row)"
            />
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
      <div class="channel-dialog-body">
        <!-- Tab Bar -->
        <div class="flex items-center border-b border-gray-200 dark:border-dark-700 flex-shrink-0 -mx-4 sm:-mx-6 px-4 sm:px-6 -mt-3 sm:-mt-4">
          <!-- Basic Settings Tab -->
          <button
            type="button"
            @click="activeTab = 'basic'"
            class="channel-tab"
            :class="activeTab === 'basic' ? 'channel-tab-active' : 'channel-tab-inactive'"
          >
            {{ t('admin.channels.form.basicSettings', '基础设置') }}
          </button>
          <!-- Platform Tabs (only enabled) -->
          <button
            v-for="section in form.platforms.filter(s => s.enabled)"
            :key="section.platform"
            type="button"
            @click="activeTab = section.platform"
            class="channel-tab group"
            :class="activeTab === section.platform ? 'channel-tab-active' : 'channel-tab-inactive'"
          >
            <PlatformIcon :platform="section.platform" size="xs" :class="getPlatformTextColor(section.platform)" />
            <span :class="getPlatformTextColor(section.platform)">{{ t('admin.groups.platforms.' + section.platform, section.platform) }}</span>
          </button>
        </div>

        <!-- Tab Content -->
        <form id="channel-form" @submit.prevent="handleSubmit" class="flex-1 overflow-y-auto pt-4">
          <!-- Basic Settings Tab -->
          <div v-show="activeTab === 'basic'" class="space-y-5">
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

            <!-- Billing Basis -->
            <div>
              <label class="input-label">{{ t('admin.channels.form.billingModelSource', 'Billing Basis') }}</label>
              <Select v-model="form.billing_model_source" :options="billingModelSourceOptions" />
              <p class="mt-1 text-xs text-gray-400">
                {{ t('admin.channels.form.billingModelSourceHint', 'Controls which model name is used for pricing lookup') }}
              </p>
            </div>

            <!-- Platform Management -->
            <div class="space-y-3">
              <label class="input-label mb-0">{{ t('admin.channels.form.platformConfig', '平台配置') }}</label>
              <div class="flex flex-wrap gap-2">
                <label
                  v-for="p in platformOrder"
                  :key="p"
                  class="inline-flex cursor-pointer items-center gap-1.5 rounded-md border px-3 py-1.5 text-sm transition-colors"
                  :class="activePlatforms.includes(p)
                    ? 'bg-primary-50 border-primary-300 dark:bg-primary-900/20 dark:border-primary-700'
                    : 'border-gray-200 hover:bg-gray-50 dark:border-dark-600 dark:hover:bg-dark-700'"
                >
                  <input
                    type="checkbox"
                    :checked="activePlatforms.includes(p)"
                    class="h-3.5 w-3.5 rounded border-gray-300 text-primary-600 focus:ring-primary-500"
                    @change="togglePlatform(p)"
                  />
                  <PlatformIcon :platform="p" size="xs" :class="getPlatformTextColor(p)" />
                  <span :class="getPlatformTextColor(p)">{{ t('admin.groups.platforms.' + p, p) }}</span>
                </label>
              </div>
            </div>
          </div>

          <!-- Platform Tab Content -->
          <div
            v-for="(section, sIdx) in form.platforms"
            :key="'tab-' + section.platform"
            v-show="section.enabled && activeTab === section.platform"
            class="space-y-4"
          >
            <!-- Groups -->
            <div>
              <label class="input-label text-xs">
                {{ t('admin.channels.form.groups', 'Associated Groups') }} <span class="text-red-500">*</span>
                <span v-if="section.group_ids.length > 0" class="ml-1 font-normal text-gray-400">
                  ({{ t('admin.channels.form.selectedCount', { count: section.group_ids.length }, `已选 ${section.group_ids.length} 个`) }})
                </span>
              </label>
              <div class="max-h-40 overflow-auto rounded-lg border border-gray-200 bg-gray-50 p-2 dark:border-dark-600 dark:bg-dark-900">
                <div v-if="groupsLoading" class="py-2 text-center text-xs text-gray-500">
                  {{ t('common.loading', 'Loading...') }}
                </div>
                <div v-else-if="getGroupsForPlatform(section.platform).length === 0" class="py-2 text-center text-xs text-gray-500">
                  {{ t('admin.channels.form.noGroupsAvailable', 'No groups available') }}
                </div>
                <div v-else class="flex flex-wrap gap-1">
                  <label
                    v-for="group in getGroupsForPlatform(section.platform)"
                    :key="group.id"
                    class="inline-flex cursor-pointer items-center gap-1.5 rounded-md border border-gray-200 px-2 py-1 text-xs transition-colors hover:bg-gray-50 dark:border-dark-600 dark:hover:bg-dark-700"
                    :class="[
                      section.group_ids.includes(group.id) ? 'bg-primary-50 border-primary-300 dark:bg-primary-900/20 dark:border-primary-700' : '',
                      isGroupInOtherChannel(group.id, section.platform) ? 'opacity-40' : ''
                    ]"
                  >
                    <input
                      type="checkbox"
                      :checked="section.group_ids.includes(group.id)"
                      :disabled="isGroupInOtherChannel(group.id, section.platform)"
                      class="h-3 w-3 rounded border-gray-300 text-primary-600 focus:ring-primary-500"
                      @change="toggleGroupInSection(sIdx, group.id)"
                    />
                    <span :class="['font-medium', getPlatformTextColor(group.platform)]">{{ group.name }}</span>
                    <span
                      :class="['rounded-full px-1 py-0 text-[10px]', getRateBadgeClass(group.platform)]"
                    >{{ group.rate_multiplier }}x</span>
                    <span class="text-[10px] text-gray-400">{{ group.account_count || 0 }}</span>
                    <span
                      v-if="isGroupInOtherChannel(group.id, section.platform)"
                      class="text-[10px] text-gray-400"
                    >{{ getGroupInOtherChannelLabel(group.id) }}</span>
                  </label>
                </div>
              </div>
            </div>

            <!-- Model Mapping -->
            <div>
              <div class="mb-1 flex items-center justify-between">
                <label class="input-label text-xs mb-0">{{ t('admin.channels.form.modelMapping', 'Model Mapping') }}</label>
                <button type="button" @click="addMappingEntry(sIdx)" class="text-xs text-primary-600 hover:text-primary-700">
                  + {{ t('common.add', 'Add') }}
                </button>
              </div>
              <div
                v-if="Object.keys(section.model_mapping).length === 0"
                class="rounded border border-dashed border-gray-300 p-2 text-center text-xs text-gray-400 dark:border-dark-500"
              >
                {{ t('admin.channels.form.noMappingRules', 'No mapping rules. Click "Add" to create one.') }}
              </div>
              <div v-else class="space-y-1">
                <div
                  v-for="(_, srcModel) in section.model_mapping"
                  :key="srcModel"
                  class="flex items-center gap-2"
                >
                  <input
                    :value="srcModel"
                    type="text"
                    class="input flex-1 text-xs"
                    :class="getPlatformTextColor(section.platform)"
                    :placeholder="t('admin.channels.form.mappingSource', 'Source model')"
                    @change="renameMappingKey(sIdx, srcModel, ($event.target as HTMLInputElement).value)"
                  />
                  <span class="text-gray-400 text-xs">→</span>
                  <input
                    :value="section.model_mapping[srcModel]"
                    type="text"
                    class="input flex-1 text-xs"
                    :class="getPlatformTextColor(section.platform)"
                    :placeholder="t('admin.channels.form.mappingTarget', 'Target model')"
                    @input="section.model_mapping[srcModel] = ($event.target as HTMLInputElement).value"
                  />
                  <button
                    type="button"
                    @click="removeMappingEntry(sIdx, srcModel)"
                    class="rounded p-0.5 text-gray-400 hover:text-red-500"
                  >
                    <Icon name="trash" size="sm" />
                  </button>
                </div>
              </div>
            </div>

            <!-- Model Pricing -->
            <div>
              <div class="mb-1 flex items-center justify-between">
                <label class="input-label text-xs mb-0">{{ t('admin.channels.form.modelPricing', 'Model Pricing') }}</label>
                <button type="button" @click="addPricingEntry(sIdx)" class="text-xs text-primary-600 hover:text-primary-700">
                  + {{ t('common.add', 'Add') }}
                </button>
              </div>
              <div
                v-if="section.model_pricing.length === 0"
                class="rounded border border-dashed border-gray-300 p-2 text-center text-xs text-gray-400 dark:border-dark-500"
              >
                {{ t('admin.channels.form.noPricingRules', 'No pricing rules yet. Click "Add" to create one.') }}
              </div>
              <div v-else class="space-y-2">
                <PricingEntryCard
                  v-for="(entry, idx) in section.model_pricing"
                  :key="idx"
                  :entry="entry"
                  :platform="section.platform"
                  @update="updatePricingEntry(sIdx, idx, $event)"
                  @remove="removePricingEntry(sIdx, idx)"
                />
              </div>
            </div>
          </div>
        </form>
      </div>

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
import { mTokToPerToken, perTokenToMTok, apiIntervalsToForm, formIntervalsToAPI, findModelConflict } from '@/components/admin/channel/types'
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
import Toggle from '@/components/common/Toggle.vue'
import PricingEntryCard from '@/components/admin/channel/PricingEntryCard.vue'
import { getPersistedPageSize } from '@/composables/usePersistedPageSize'

const { t } = useI18n()
const appStore = useAppStore()

// ── Platform Section type ──
interface PlatformSection {
  platform: GroupPlatform
  enabled: boolean
  collapsed: boolean
  group_ids: number[]
  model_mapping: Record<string, string>
  model_pricing: PricingFormEntry[]
}

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
  { value: 'channel_mapped', label: t('admin.channels.form.billingModelSourceChannelMapped', 'Bill by channel-mapped model') },
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
const activeTab = ref<string>('basic')

// Groups
const allGroups = ref<AdminGroup[]>([])
const groupsLoading = ref(false)

// Form data
const form = reactive({
  name: '',
  description: '',
  status: 'active',
  restrict_models: false,
  billing_model_source: 'channel_mapped' as string,
  platforms: [] as PlatformSection[]
})

let abortController: AbortController | null = null

// ── Platform config ──
const platformOrder: GroupPlatform[] = ['anthropic', 'openai', 'gemini', 'antigravity']

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

// ── Helpers ──
function formatDate(value: string): string {
  if (!value) return '-'
  return new Date(value).toLocaleDateString()
}

// ── Platform section helpers ──
const activePlatforms = computed(() => form.platforms.filter(s => s.enabled).map(s => s.platform))

function addPlatformSection(platform: GroupPlatform) {
  form.platforms.push({
    platform,
    enabled: true,
    collapsed: false,
    group_ids: [],
    model_mapping: {},
    model_pricing: []
  })
}

function togglePlatform(platform: GroupPlatform) {
  const section = form.platforms.find(s => s.platform === platform)
  if (section) {
    section.enabled = !section.enabled
    if (!section.enabled && activeTab.value === platform) {
      activeTab.value = 'basic'
    }
  } else {
    addPlatformSection(platform)
  }
}

function getGroupsForPlatform(platform: GroupPlatform): AdminGroup[] {
  return allGroups.value.filter(g => g.platform === platform)
}

// ── Group helpers ──
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

function isGroupInOtherChannel(groupId: number, _platform: string): boolean {
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

function toggleGroupInSection(sectionIdx: number, groupId: number) {
  const section = form.platforms[sectionIdx]
  const idx = section.group_ids.indexOf(groupId)
  if (idx >= 0) {
    section.group_ids.splice(idx, 1)
  } else {
    section.group_ids.push(groupId)
  }
}

// ── Pricing helpers ──
function addPricingEntry(sectionIdx: number) {
  form.platforms[sectionIdx].model_pricing.push({
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

function updatePricingEntry(sectionIdx: number, idx: number, updated: PricingFormEntry) {
  form.platforms[sectionIdx].model_pricing.splice(idx, 1, updated)
}

function removePricingEntry(sectionIdx: number, idx: number) {
  form.platforms[sectionIdx].model_pricing.splice(idx, 1)
}

// ── Model Mapping helpers ──
function addMappingEntry(sectionIdx: number) {
  const mapping = form.platforms[sectionIdx].model_mapping
  let key = ''
  let i = 1
  while (key === '' || key in mapping) {
    key = `model-${i}`
    i++
  }
  mapping[key] = ''
}

function removeMappingEntry(sectionIdx: number, key: string) {
  delete form.platforms[sectionIdx].model_mapping[key]
}

function renameMappingKey(sectionIdx: number, oldKey: string, newKey: string) {
  newKey = newKey.trim()
  if (!newKey || newKey === oldKey) return
  const mapping = form.platforms[sectionIdx].model_mapping
  if (newKey in mapping) return
  const value = mapping[oldKey]
  delete mapping[oldKey]
  mapping[newKey] = value
}

// ── Form ↔ API conversion ──
function formToAPI(): { group_ids: number[], model_pricing: ChannelModelPricing[], model_mapping: Record<string, Record<string, string>> } {
  const group_ids: number[] = []
  const model_pricing: ChannelModelPricing[] = []
  const model_mapping: Record<string, Record<string, string>> = {}

  for (const section of form.platforms) {
    if (!section.enabled) continue
    group_ids.push(...section.group_ids)

    // Model mapping per platform
    if (Object.keys(section.model_mapping).length > 0) {
      model_mapping[section.platform] = { ...section.model_mapping }
    }

    // Model pricing with platform tag
    for (const entry of section.model_pricing) {
      if (entry.models.length === 0) continue
      model_pricing.push({
        platform: section.platform,
        models: entry.models,
        billing_mode: entry.billing_mode,
        input_price: mTokToPerToken(entry.input_price),
        output_price: mTokToPerToken(entry.output_price),
        cache_write_price: mTokToPerToken(entry.cache_write_price),
        cache_read_price: mTokToPerToken(entry.cache_read_price),
        image_output_price: mTokToPerToken(entry.image_output_price),
        per_request_price: entry.per_request_price != null && entry.per_request_price !== '' ? Number(entry.per_request_price) : null,
        intervals: formIntervalsToAPI(entry.intervals || [])
      })
    }
  }

  return { group_ids, model_pricing, model_mapping }
}

function apiToForm(channel: Channel): PlatformSection[] {
  // Build a map: groupID → platform
  const groupPlatformMap = new Map<number, GroupPlatform>()
  for (const g of allGroups.value) {
    groupPlatformMap.set(g.id, g.platform)
  }

  // Determine which platforms are active (from groups + pricing + mapping)
  const activePlatforms = new Set<GroupPlatform>()
  for (const gid of channel.group_ids || []) {
    const p = groupPlatformMap.get(gid)
    if (p) activePlatforms.add(p)
  }
  for (const p of channel.model_pricing || []) {
    if (p.platform) activePlatforms.add(p.platform as GroupPlatform)
  }
  for (const p of Object.keys(channel.model_mapping || {})) {
    if (platformOrder.includes(p as GroupPlatform)) activePlatforms.add(p as GroupPlatform)
  }

  // Build sections in platform order
  const sections: PlatformSection[] = []
  for (const platform of platformOrder) {
    if (!activePlatforms.has(platform)) continue

    const groupIds = (channel.group_ids || []).filter(gid => groupPlatformMap.get(gid) === platform)
    const mapping = (channel.model_mapping || {})[platform] || {}
    const pricing = (channel.model_pricing || [])
      .filter(p => (p.platform || 'anthropic') === platform)
      .map(p => ({
        models: p.models || [],
        billing_mode: p.billing_mode,
        input_price: perTokenToMTok(p.input_price),
        output_price: perTokenToMTok(p.output_price),
        cache_write_price: perTokenToMTok(p.cache_write_price),
        cache_read_price: perTokenToMTok(p.cache_read_price),
        image_output_price: perTokenToMTok(p.image_output_price),
        per_request_price: p.per_request_price,
        intervals: apiIntervalsToForm(p.intervals || [])
      } as PricingFormEntry))

    sections.push({
      platform,
      enabled: true,
      collapsed: false,
      group_ids: groupIds,
      model_mapping: { ...mapping },
      model_pricing: pricing
    })
  }

  return sections
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
  form.billing_model_source = 'channel_mapped'
  form.platforms = []
  activeTab.value = 'basic'
}

async function openCreateDialog() {
  editingChannel.value = null
  resetForm()
  await loadGroups()
  showDialog.value = true
}

async function openEditDialog(channel: Channel) {
  editingChannel.value = channel
  form.name = channel.name
  form.description = channel.description || ''
  form.status = channel.status
  form.restrict_models = channel.restrict_models || false
  form.billing_model_source = channel.billing_model_source || 'channel_mapped'
  // Must load groups first so apiToForm can map groupID → platform
  await loadGroups()
  form.platforms = apiToForm(channel)
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

  // Check for pricing entries with empty models (would be silently skipped)
  for (const section of form.platforms.filter(s => s.enabled)) {
    if (section.group_ids.length === 0) {
      const platformLabel = t('admin.groups.platforms.' + section.platform, section.platform)
      appStore.showError(t('admin.channels.noGroupsSelected', { platform: platformLabel }, `${platformLabel} 平台未选择分组，请至少选择一个分组或禁用该平台`))
      activeTab.value = section.platform
      return
    }
    for (const entry of section.model_pricing) {
      if (entry.models.length === 0) {
        const platformLabel = t('admin.groups.platforms.' + section.platform, section.platform)
        appStore.showError(t('admin.channels.emptyModelsInPricing', { platform: platformLabel }, `${platformLabel} 平台下有定价条目未添加模型，请添加模型或删除该条目`))
        activeTab.value = section.platform
        return
      }
    }
  }

  // Check model pattern conflicts per platform (duplicate / wildcard overlap)
  for (const section of form.platforms.filter(s => s.enabled)) {
    // Collect all pricing models for this platform
    const allModels: string[] = []
    for (const entry of section.model_pricing) {
      allModels.push(...entry.models)
    }
    const pricingConflict = findModelConflict(allModels)
    if (pricingConflict) {
      appStore.showError(
        t('admin.channels.modelConflict',
          { model1: pricingConflict[0], model2: pricingConflict[1] },
          `模型模式 '${pricingConflict[0]}' 和 '${pricingConflict[1]}' 冲突：匹配范围重叠`)
      )
      activeTab.value = section.platform
      return
    }
    // Check model mapping source pattern conflicts
    const mappingKeys = Object.keys(section.model_mapping)
    if (mappingKeys.length > 0) {
      const mappingConflict = findModelConflict(mappingKeys)
      if (mappingConflict) {
        appStore.showError(
          t('admin.channels.mappingConflict',
            { model1: mappingConflict[0], model2: mappingConflict[1] },
            `模型映射源 '${mappingConflict[0]}' 和 '${mappingConflict[1]}' 冲突：匹配范围重叠`)
        )
        activeTab.value = section.platform
        return
      }
    }
  }

  // 校验 per_request/image 模式必须有价格 (只校验启用的平台)
  for (const section of form.platforms.filter(s => s.enabled)) {
    for (const entry of section.model_pricing) {
      if (entry.models.length === 0) continue
      if ((entry.billing_mode === 'per_request' || entry.billing_mode === 'image') &&
          (entry.per_request_price == null || entry.per_request_price === '') &&
          (!entry.intervals || entry.intervals.length === 0)) {
        appStore.showError(t('admin.channels.form.perRequestPriceRequired', '按次/图片计费模式必须设置默认价格或至少一个计费层级'))
        return
      }
    }
  }

  const { group_ids, model_pricing, model_mapping } = formToAPI()

  submitting.value = true
  try {
    if (editingChannel.value) {
      const req: UpdateChannelRequest = {
        name: form.name.trim(),
        description: form.description.trim() || undefined,
        status: form.status,
        group_ids,
        model_pricing,
        model_mapping: Object.keys(model_mapping).length > 0 ? model_mapping : {},
        billing_model_source: form.billing_model_source,
        restrict_models: form.restrict_models
      }
      await adminAPI.channels.update(editingChannel.value.id, req)
      appStore.showSuccess(t('admin.channels.updateSuccess', 'Channel updated'))
    } else {
      const req: CreateChannelRequest = {
        name: form.name.trim(),
        description: form.description.trim() || undefined,
        group_ids,
        model_pricing,
        model_mapping: Object.keys(model_mapping).length > 0 ? model_mapping : {},
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

// ── Toggle status ──
async function toggleChannelStatus(channel: Channel) {
  const newStatus = channel.status === 'active' ? 'disabled' : 'active'
  try {
    await adminAPI.channels.update(channel.id, { status: newStatus })
    channel.status = newStatus
  } catch (error) {
    appStore.showError(t('admin.channels.updateError', 'Failed to update channel'))
    console.error('Error toggling channel status:', error)
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
  loadGroups()
})

onUnmounted(() => {
  clearTimeout(searchTimeout)
  abortController?.abort()
})
</script>

<style scoped>
.channel-dialog-body {
  display: flex;
  flex-direction: column;
  height: 70vh;
  min-height: 400px;
}

.channel-tab {
  @apply flex items-center gap-1.5 px-3 py-2.5 text-sm font-medium border-b-2 transition-colors whitespace-nowrap;
}

.channel-tab-active {
  @apply border-primary-600 text-primary-600 dark:border-primary-400 dark:text-primary-400;
}

.channel-tab-inactive {
  @apply border-transparent text-gray-500 hover:text-gray-700 hover:border-gray-300 dark:text-gray-400 dark:hover:text-gray-300;
}
</style>
