<template>
  <AppLayout>
    <TablePageLayout>
      <template #filters>
        <div class="flex flex-wrap items-center gap-3">
          <div class="relative w-full sm:w-64">
            <Icon
              name="search"
              size="md"
              class="absolute left-3 top-1/2 -translate-y-1/2 text-gray-400 dark:text-gray-500"
            />
            <input
              v-model="searchQuery"
              type="text"
              :placeholder="t('admin.billingPools.searchPlaceholder')"
              class="input pl-10"
              @input="handleSearch"
            />
          </div>

          <Select
            v-model="filters.status"
            :options="statusFilterOptions"
            class="w-full sm:w-40"
            @change="handleFilterChange"
          />

          <Select
            v-model="filters.platform_scope"
            :options="platformScopeFilterOptions"
            class="w-full sm:w-48"
            @change="handleFilterChange"
          />

          <div class="flex flex-1 flex-wrap items-center justify-end gap-2">
            <button
              type="button"
              class="btn btn-secondary"
              :disabled="loading"
              :title="t('common.refresh')"
              @click="loadBillingPools"
            >
              <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
            </button>
            <button type="button" class="btn btn-primary" @click="openCreateDialog">
              <Icon name="plus" size="md" class="mr-2" />
              {{ t('admin.billingPools.createPool') }}
            </button>
          </div>
        </div>
      </template>

      <template #table>
        <DataTable
          :columns="columns"
          :data="billingPools"
          :loading="loading"
          :server-side-sort="true"
          default-sort-key="updated_at"
          default-sort-order="desc"
          @sort="handleSort"
        >
          <template #cell-name="{ row }">
            <div class="min-w-0">
              <div class="font-medium text-gray-900 dark:text-white">{{ row.name }}</div>
              <code class="mt-1 block text-xs text-gray-500 dark:text-dark-400">{{ row.code }}</code>
            </div>
          </template>

          <template #cell-status="{ value }">
            <span :class="['badge', value === 'active' ? 'badge-success' : 'badge-gray']">
              {{ statusLabel(value) }}
            </span>
          </template>

          <template #cell-platform_scope="{ value }">
            <span class="badge badge-gray">{{ platformScopeLabel(value) }}</span>
          </template>

          <template #cell-options="{ row }">
            <div class="flex flex-wrap gap-1.5">
              <span v-if="row.allow_user_reorder" class="badge badge-primary">
                {{ t('admin.billingPools.allowUserReorderShort') }}
              </span>
              <span v-if="row.require_primary_subscription" class="badge badge-warning">
                {{ t('admin.billingPools.requirePrimaryShort') }}
              </span>
              <span v-if="row.allow_balance_fallback" class="badge badge-success">
                {{ t('admin.billingPools.balanceFallbackShort') }}
              </span>
            </div>
          </template>

          <template #cell-group_count="{ value }">
            <span class="text-sm text-gray-700 dark:text-gray-300">
              {{ t('admin.billingPools.groupCount', { count: value || 0 }) }}
            </span>
          </template>

          <template #cell-updated_at="{ value }">
            <span class="text-sm text-gray-500 dark:text-dark-400">
              {{ formatDateTime(value) }}
            </span>
          </template>

          <template #cell-actions="{ row }">
            <div class="flex items-center gap-1">
              <button
                type="button"
                class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-blue-50 hover:text-blue-600 dark:hover:bg-blue-900/20 dark:hover:text-blue-400"
                :title="t('admin.billingPools.members')"
                @click="openMembersDialog(row)"
              >
                <Icon name="users" size="sm" />
                <span class="text-xs">{{ t('admin.billingPools.members') }}</span>
              </button>
              <button
                type="button"
                class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-gray-100 hover:text-primary-600 dark:hover:bg-dark-700 dark:hover:text-primary-400"
                :title="t('common.edit')"
                @click="openEditDialog(row)"
              >
                <Icon name="edit" size="sm" />
                <span class="text-xs">{{ t('common.edit') }}</span>
              </button>
              <button
                type="button"
                class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-900/20 dark:hover:text-red-400"
                :title="t('common.delete')"
                @click="openDeleteDialog(row)"
              >
                <Icon name="trash" size="sm" />
                <span class="text-xs">{{ t('common.delete') }}</span>
              </button>
            </div>
          </template>

          <template #empty>
            <EmptyState
              :title="t('admin.billingPools.noPools')"
              :description="t('admin.billingPools.noPoolsHint')"
              :action-text="t('admin.billingPools.createPool')"
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

    <BaseDialog
      :show="showFormDialog"
      :title="isEditing ? t('admin.billingPools.editPool') : t('admin.billingPools.createPool')"
      width="wide"
      @close="closeFormDialog"
    >
      <form id="billing-pool-form" class="space-y-5" @submit.prevent="handleSavePool">
        <div class="grid grid-cols-1 gap-4 md:grid-cols-2">
          <div>
            <label class="input-label">{{ t('admin.billingPools.form.name') }}</label>
            <input v-model="form.name" type="text" class="input" required />
          </div>
          <div>
            <label class="input-label">{{ t('admin.billingPools.form.code') }}</label>
            <input v-model="form.code" type="text" class="input" required />
          </div>
        </div>

        <div>
          <label class="input-label">{{ t('admin.billingPools.form.description') }}</label>
          <textarea v-model="form.description" rows="3" class="input"></textarea>
        </div>

        <div class="grid grid-cols-1 gap-4 md:grid-cols-2">
          <div>
            <label class="input-label">{{ t('admin.billingPools.form.status') }}</label>
            <Select v-model="form.status" :options="statusOptions" />
          </div>
          <div>
            <label class="input-label">{{ t('admin.billingPools.form.platformScope') }}</label>
            <Select v-model="form.platform_scope" :options="platformScopeOptions" />
          </div>
        </div>

        <div class="rounded-xl border border-gray-200 p-4 dark:border-dark-600">
          <div class="mb-3 text-sm font-medium text-gray-900 dark:text-white">
            {{ t('admin.billingPools.form.options') }}
          </div>
          <div class="grid grid-cols-1 gap-3 md:grid-cols-3">
            <label class="flex cursor-pointer items-start gap-2 text-sm text-gray-700 dark:text-gray-300">
              <input v-model="form.allow_user_reorder" type="checkbox" class="mt-1 h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500" />
              <span>{{ t('admin.billingPools.form.allowUserReorder') }}</span>
            </label>
            <label class="flex cursor-pointer items-start gap-2 text-sm text-gray-700 dark:text-gray-300">
              <input v-model="form.require_primary_subscription" type="checkbox" class="mt-1 h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500" />
              <span>{{ t('admin.billingPools.form.requirePrimarySubscription') }}</span>
            </label>
            <label class="flex cursor-pointer items-start gap-2 text-sm text-gray-700 dark:text-gray-300">
              <input v-model="form.allow_balance_fallback" type="checkbox" class="mt-1 h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500" />
              <span>{{ t('admin.billingPools.form.allowBalanceFallback') }}</span>
            </label>
          </div>
        </div>
      </form>

      <template #footer>
        <div class="flex justify-end gap-3">
          <button type="button" class="btn btn-secondary" @click="closeFormDialog">
            {{ t('common.cancel') }}
          </button>
          <button type="submit" form="billing-pool-form" class="btn btn-primary" :disabled="saving">
            {{ saving ? t('common.saving') : t('common.save') }}
          </button>
        </div>
      </template>
    </BaseDialog>

    <BaseDialog
      :show="showMembersDialog"
      :title="t('admin.billingPools.membersTitle', { name: memberPool?.name || '' })"
      width="extra-wide"
      @close="closeMembersDialog"
    >
      <div class="space-y-4">
        <div class="flex flex-wrap items-center justify-between gap-3">
          <p class="text-sm text-gray-500 dark:text-dark-400">
            {{ t('admin.billingPools.membersHint') }}
          </p>
          <button
            type="button"
            class="btn btn-secondary"
            :disabled="loadingMembers || loadingGroups"
            @click="addMember"
          >
            <Icon name="plus" size="md" class="mr-2" />
            {{ t('admin.billingPools.addMember') }}
          </button>
        </div>

        <div v-if="loadingMembers" class="py-10 text-center text-sm text-gray-500 dark:text-dark-400">
          {{ t('common.loading') }}
        </div>

        <div v-else-if="memberRows.length === 0" class="rounded-xl border border-dashed border-gray-300 p-8 text-center dark:border-dark-600">
          <p class="text-sm font-medium text-gray-900 dark:text-white">
            {{ t('admin.billingPools.noMembers') }}
          </p>
          <p class="mt-1 text-sm text-gray-500 dark:text-dark-400">
            {{ t('admin.billingPools.noMembersHint') }}
          </p>
        </div>

        <div v-else class="overflow-x-auto rounded-xl border border-gray-200 dark:border-dark-700">
          <table class="w-full min-w-[760px] divide-y divide-gray-200 dark:divide-dark-700">
            <thead class="bg-gray-50 dark:bg-dark-800">
              <tr>
                <th class="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-dark-400">
                  {{ t('admin.billingPools.columns.chainOrder') }}
                </th>
                <th class="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-dark-400">
                  {{ t('admin.billingPools.columns.group') }}
                </th>
                <th class="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-dark-400">
                  {{ t('admin.billingPools.columns.flags') }}
                </th>
                <th class="px-4 py-3 text-right text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-dark-400">
                  {{ t('common.actions') }}
                </th>
              </tr>
            </thead>
            <tbody class="divide-y divide-gray-100 bg-white dark:divide-dark-800 dark:bg-dark-900">
              <tr v-for="(member, index) in memberRows" :key="`${member.group_id}-${index}`">
                <td class="px-4 py-3 align-middle text-sm text-gray-700 dark:text-gray-300">
                  <div class="flex items-center gap-2">
                    <span class="inline-flex h-7 min-w-7 items-center justify-center rounded-full bg-gray-100 px-2 text-xs font-medium text-gray-700 dark:bg-dark-700 dark:text-gray-300">
                      {{ index + 1 }}
                    </span>
                    <div class="flex gap-1">
                      <button
                        type="button"
                        class="rounded p-1 text-gray-400 hover:bg-gray-100 hover:text-gray-700 disabled:cursor-not-allowed disabled:opacity-40 dark:hover:bg-dark-700 dark:hover:text-gray-200"
                        :disabled="index === 0"
                        :title="t('admin.billingPools.moveUp')"
                        @click="moveMember(index, -1)"
                      >
                        <Icon name="chevronUp" size="sm" />
                      </button>
                      <button
                        type="button"
                        class="rounded p-1 text-gray-400 hover:bg-gray-100 hover:text-gray-700 disabled:cursor-not-allowed disabled:opacity-40 dark:hover:bg-dark-700 dark:hover:text-gray-200"
                        :disabled="index === memberRows.length - 1"
                        :title="t('admin.billingPools.moveDown')"
                        @click="moveMember(index, 1)"
                      >
                        <Icon name="chevronDown" size="sm" />
                      </button>
                    </div>
                  </div>
                </td>
                <td class="px-4 py-3 align-middle">
                  <Select
                    v-model="member.group_id"
                    :options="groupOptionsFor(member.group_id)"
                    searchable
                    @change="(value) => handleMemberGroupChange(member, value)"
                  >
                    <template #selected="{ option }">
                      <span v-if="option" class="flex items-center gap-2">
                        <PlatformIcon :platform="asGroupPlatform(selectOptionField(option, 'platform'))" size="xs" />
                        <span>{{ selectOptionField(option, 'label') }}</span>
                      </span>
                    </template>
                    <template #option="{ option }">
                      <span class="flex items-center gap-2">
                        <PlatformIcon :platform="asGroupPlatform(selectOptionField(option, 'platform'))" size="xs" />
                        <span>{{ selectOptionField(option, 'label') }}</span>
                      </span>
                    </template>
                  </Select>
                </td>
                <td class="px-4 py-3 align-middle">
                  <div class="flex flex-wrap gap-3">
                    <label class="flex cursor-pointer items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
                      <input v-model="member.can_be_primary" type="checkbox" class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500" />
                      {{ t('admin.billingPools.canBePrimary') }}
                    </label>
                    <label class="flex cursor-pointer items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
                      <input v-model="member.can_be_fallback" type="checkbox" class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500" />
                      {{ t('admin.billingPools.canBeFallback') }}
                    </label>
                  </div>
                </td>
                <td class="px-4 py-3 text-right align-middle">
                  <button
                    type="button"
                    class="rounded-lg p-2 text-gray-400 transition-colors hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-900/20 dark:hover:text-red-400"
                    :title="t('common.delete')"
                    @click="removeMember(index)"
                  >
                    <Icon name="trash" size="sm" />
                  </button>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>

      <template #footer>
        <div class="flex justify-end gap-3">
          <button type="button" class="btn btn-secondary" @click="closeMembersDialog">
            {{ t('common.cancel') }}
          </button>
          <button type="button" class="btn btn-primary" :disabled="savingMembers || loadingMembers" @click="saveMembers">
            {{ savingMembers ? t('common.saving') : t('common.save') }}
          </button>
        </div>
      </template>
    </BaseDialog>

    <ConfirmDialog
      :show="showDeleteDialog"
      :title="t('admin.billingPools.deletePool')"
      :message="t('admin.billingPools.deleteConfirm', { name: deletingPool?.name || '' })"
      :confirm-text="t('common.delete')"
      :cancel-text="t('common.cancel')"
      danger
      @confirm="confirmDelete"
      @cancel="showDeleteDialog = false"
    />
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import { getPersistedPageSize } from '@/composables/usePersistedPageSize'
import { useAppStore } from '@/stores/app'
import { formatDateTime } from '@/utils/format'
import type { Column } from '@/components/common/types'
import type {
  AdminGroup,
  BillingPool,
  BillingPoolGroupMember,
  BillingPoolMemberRequest,
  BillingPoolPlatformScope,
  BillingPoolStatus,
  GroupPlatform
} from '@/types'

import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import DataTable from '@/components/common/DataTable.vue'
import Pagination from '@/components/common/Pagination.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import Select from '@/components/common/Select.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import Icon from '@/components/icons/Icon.vue'
import PlatformIcon from '@/components/common/PlatformIcon.vue'

const { t } = useI18n()
const appStore = useAppStore()

interface EditableMember extends BillingPoolMemberRequest {
  group_name?: string
  platform?: string
  subscription_type?: string
}

interface GroupSelectOption {
  [key: string]: unknown
  value: number
  label: string
  platform?: string
  disabled: boolean
}

const billingPools = ref<BillingPool[]>([])
const subscriptionGroups = ref<AdminGroup[]>([])
const loading = ref(false)
const loadingGroups = ref(false)
const groupsLoaded = ref(false)
const saving = ref(false)
const savingMembers = ref(false)
const loadingMembers = ref(false)
const searchQuery = ref('')

const filters = reactive({
  status: '' as '' | BillingPoolStatus,
  platform_scope: '' as '' | BillingPoolPlatformScope
})

const pagination = reactive({
  page: 1,
  page_size: getPersistedPageSize(),
  total: 0,
  pages: 0
})

const sortState = reactive({
  sort_by: 'updated_at',
  sort_order: 'desc' as 'asc' | 'desc'
})

const form = reactive({
  id: null as number | null,
  name: '',
  code: '',
  description: '',
  status: 'active' as BillingPoolStatus,
  platform_scope: 'same_platform' as BillingPoolPlatformScope,
  allow_user_reorder: false,
  require_primary_subscription: true,
  allow_balance_fallback: true
})

const showFormDialog = ref(false)
const showMembersDialog = ref(false)
const showDeleteDialog = ref(false)
const memberPool = ref<BillingPool | null>(null)
const memberRows = ref<EditableMember[]>([])
const deletingPool = ref<BillingPool | null>(null)

const isEditing = computed(() => form.id !== null)

const statusFilterOptions = computed(() => [
  { value: '', label: t('admin.billingPools.allStatus') },
  { value: 'active', label: t('common.active') },
  { value: 'inactive', label: t('common.inactive') }
])

const statusOptions = computed(() => [
  { value: 'active', label: t('common.active') },
  { value: 'inactive', label: t('common.inactive') }
])

const platformScopeFilterOptions = computed(() => [
  { value: '', label: t('admin.billingPools.allPlatformScopes') },
  { value: 'same_platform', label: t('admin.billingPools.platformScopes.same_platform') },
  { value: 'mixed_platform', label: t('admin.billingPools.platformScopes.mixed_platform') }
])

const platformScopeOptions = computed(() => [
  { value: 'same_platform', label: t('admin.billingPools.platformScopes.same_platform') },
  { value: 'mixed_platform', label: t('admin.billingPools.platformScopes.mixed_platform') }
])

const columns = computed<Column[]>(() => [
  { key: 'name', label: t('admin.billingPools.columns.name'), sortable: true },
  { key: 'status', label: t('admin.billingPools.columns.status'), sortable: true },
  { key: 'platform_scope', label: t('admin.billingPools.columns.platformScope'), sortable: true },
  { key: 'options', label: t('admin.billingPools.columns.options') },
  { key: 'group_count', label: t('admin.billingPools.columns.groupCount') },
  { key: 'updated_at', label: t('admin.billingPools.columns.updatedAt'), sortable: true },
  { key: 'actions', label: t('admin.billingPools.columns.actions') }
])

const selectedGroupIds = computed(() => {
  return new Set(memberRows.value.map((member) => member.group_id).filter((id) => id > 0))
})

let currentController: AbortController | null = null

function errorMessage(error: unknown, fallback: string): string {
  if (error && typeof error === 'object' && 'message' in error) {
    const message = (error as { message?: unknown }).message
    if (typeof message === 'string' && message.trim()) return message
  }
  return fallback
}

function statusLabel(status: string): string {
  return status === 'active' ? t('common.active') : t('common.inactive')
}

function platformScopeLabel(scope: string): string {
  return scope === 'mixed_platform'
    ? t('admin.billingPools.platformScopes.mixed_platform')
    : t('admin.billingPools.platformScopes.same_platform')
}

function selectOptionField(option: unknown, field: string): string {
  if (option && typeof option === 'object' && field in option) {
    const value = (option as Record<string, unknown>)[field]
    return typeof value === 'string' ? value : ''
  }
  return ''
}

function asGroupPlatform(platform: unknown): GroupPlatform | undefined {
  return typeof platform === 'string' ? (platform as GroupPlatform) : undefined
}

async function loadBillingPools() {
  currentController?.abort()
  const requestController = new AbortController()
  currentController = requestController
  const { signal } = requestController

  try {
    loading.value = true
    const response = await adminAPI.billingPools.list(
      pagination.page,
      pagination.page_size,
      {
        status: filters.status || undefined,
        platform_scope: filters.platform_scope || undefined,
        search: searchQuery.value.trim() || undefined,
        sort_by: sortState.sort_by,
        sort_order: sortState.sort_order
      },
      { signal }
    )

    if (signal.aborted || currentController !== requestController) return

    billingPools.value = response.items
    pagination.page = response.page
    pagination.page_size = response.page_size
    pagination.total = response.total
    pagination.pages = response.pages
  } catch (error) {
    if (!signal.aborted) {
      appStore.showError(errorMessage(error, t('admin.billingPools.failedToLoad')))
    }
  } finally {
    if (currentController === requestController) {
      currentController = null
      loading.value = false
    }
  }
}

async function loadGroups() {
  try {
    loadingGroups.value = true
    const groups = await adminAPI.groups.getAll()
    subscriptionGroups.value = groups.filter((group) => group.subscription_type === 'subscription')
    groupsLoaded.value = true
  } catch (error) {
    appStore.showError(errorMessage(error, t('admin.billingPools.failedToLoadGroups')))
  } finally {
    loadingGroups.value = false
  }
}

async function ensureGroupsLoaded() {
  if (!groupsLoaded.value && !loadingGroups.value) {
    await loadGroups()
  }
}

function handleSearch() {
  pagination.page = 1
  loadBillingPools()
}

function handleFilterChange() {
  pagination.page = 1
  loadBillingPools()
}

function handleSort(key: string, order: 'asc' | 'desc') {
  sortState.sort_by = key
  sortState.sort_order = order
  pagination.page = 1
  loadBillingPools()
}

function handlePageChange(page: number) {
  pagination.page = page
  loadBillingPools()
}

function handlePageSizeChange(pageSize: number) {
  pagination.page = 1
  pagination.page_size = pageSize
  loadBillingPools()
}

function resetForm(pool?: BillingPool) {
  form.id = pool?.id ?? null
  form.name = pool?.name ?? ''
  form.code = pool?.code ?? ''
  form.description = pool?.description ?? ''
  form.status = pool?.status ?? 'active'
  form.platform_scope = pool?.platform_scope ?? 'same_platform'
  form.allow_user_reorder = pool?.allow_user_reorder ?? false
  form.require_primary_subscription = pool?.require_primary_subscription ?? true
  form.allow_balance_fallback = pool?.allow_balance_fallback ?? true
}

function openCreateDialog() {
  resetForm()
  showFormDialog.value = true
}

function openEditDialog(pool: BillingPool) {
  resetForm(pool)
  showFormDialog.value = true
}

function closeFormDialog() {
  if (saving.value) return
  showFormDialog.value = false
}

async function handleSavePool() {
  const name = form.name.trim()
  const code = form.code.trim()
  if (!name || !code) {
    appStore.showError(t('admin.billingPools.nameCodeRequired'))
    return
  }

  try {
    saving.value = true
    const payload = {
      name,
      code,
      description: form.description.trim(),
      status: form.status,
      platform_scope: form.platform_scope,
      allow_user_reorder: form.allow_user_reorder,
      require_primary_subscription: form.require_primary_subscription,
      allow_balance_fallback: form.allow_balance_fallback
    }

    if (form.id) {
      await adminAPI.billingPools.update(form.id, payload)
      appStore.showSuccess(t('admin.billingPools.poolUpdated'))
    } else {
      await adminAPI.billingPools.create({ ...payload, groups: [] })
      appStore.showSuccess(t('admin.billingPools.poolCreated'))
    }

    showFormDialog.value = false
    await loadBillingPools()
  } catch (error) {
    appStore.showError(errorMessage(error, t('admin.billingPools.failedToSave')))
  } finally {
    saving.value = false
  }
}

function normalizeMembers(members: BillingPoolGroupMember[] | undefined): EditableMember[] {
  return [...(members ?? [])]
    .sort((a, b) => {
      if (a.chain_order === b.chain_order) return a.group_id - b.group_id
      return a.chain_order - b.chain_order
    })
    .map((member, index) => ({
      group_id: member.group_id,
      group_name: member.group_name,
      platform: member.platform,
      subscription_type: member.subscription_type,
      chain_order: index + 1,
      can_be_primary: member.can_be_primary,
      can_be_fallback: member.can_be_fallback
    }))
}

async function openMembersDialog(pool: BillingPool) {
  memberPool.value = pool
  memberRows.value = []
  showMembersDialog.value = true
  loadingMembers.value = true

  try {
    await ensureGroupsLoaded()
    const detail = await adminAPI.billingPools.getById(pool.id)
    memberPool.value = detail
    memberRows.value = normalizeMembers(detail.groups)
  } catch (error) {
    appStore.showError(errorMessage(error, t('admin.billingPools.failedToLoadMembers')))
  } finally {
    loadingMembers.value = false
  }
}

function closeMembersDialog() {
  if (savingMembers.value) return
  showMembersDialog.value = false
  memberPool.value = null
  memberRows.value = []
}

function syncChainOrder() {
  memberRows.value = memberRows.value.map((member, index) => ({
    ...member,
    chain_order: index + 1
  }))
}

function moveMember(index: number, direction: -1 | 1) {
  const nextIndex = index + direction
  if (nextIndex < 0 || nextIndex >= memberRows.value.length) return
  const next = [...memberRows.value]
  const current = next[index]
  next[index] = next[nextIndex]
  next[nextIndex] = current
  memberRows.value = next
  syncChainOrder()
}

function removeMember(index: number) {
  memberRows.value.splice(index, 1)
  syncChainOrder()
}

async function addMember() {
  await ensureGroupsLoaded()
  const nextGroup = subscriptionGroups.value.find((group) => !selectedGroupIds.value.has(group.id))
  if (!nextGroup) {
    appStore.showWarning(t('admin.billingPools.noAvailableGroups'))
    return
  }

  memberRows.value.push({
    group_id: nextGroup.id,
    group_name: nextGroup.name,
    platform: nextGroup.platform,
    subscription_type: nextGroup.subscription_type,
    chain_order: memberRows.value.length + 1,
    can_be_primary: true,
    can_be_fallback: true
  })
}

function groupLabel(group: AdminGroup): string {
  return `${group.name} · ${platformName(group.platform)}`
}

function platformName(platform: string): string {
  switch (platform) {
    case 'anthropic':
      return t('admin.groups.platforms.anthropic')
    case 'openai':
      return t('admin.groups.platforms.openai')
    case 'gemini':
      return t('admin.groups.platforms.gemini')
    case 'antigravity':
      return t('admin.groups.platforms.antigravity')
    default:
      return platform
  }
}

function groupOptionsFor(currentGroupId: number): GroupSelectOption[] {
  const selected = selectedGroupIds.value
  const options: GroupSelectOption[] = subscriptionGroups.value.map((group) => ({
    value: group.id,
    label: groupLabel(group),
    platform: group.platform,
    disabled: group.id !== currentGroupId && selected.has(group.id)
  }))

  const hasCurrent = options.some((option) => option.value === currentGroupId)
  const currentMember = memberRows.value.find((member) => member.group_id === currentGroupId)
  if (!hasCurrent && currentMember && currentGroupId > 0) {
    options.push({
      value: currentGroupId,
      label: `${currentMember.group_name || `#${currentGroupId}`} · ${currentMember.platform || '-'}`,
      platform: currentMember.platform,
      disabled: false
    })
  }

  return options
}

function handleMemberGroupChange(member: EditableMember, value: string | number | boolean | null) {
  if (typeof value !== 'number') return
  const group = subscriptionGroups.value.find((item) => item.id === value)
  member.group_id = value
  if (group) {
    member.group_name = group.name
    member.platform = group.platform
    member.subscription_type = group.subscription_type
  }
}

function validateMembers(): boolean {
  const seen = new Set<number>()
  for (const member of memberRows.value) {
    if (!member.group_id || member.group_id <= 0) {
      appStore.showError(t('admin.billingPools.invalidMember'))
      return false
    }
    if (seen.has(member.group_id)) {
      appStore.showError(t('admin.billingPools.duplicateMember'))
      return false
    }
    seen.add(member.group_id)
  }
  return true
}

async function saveMembers() {
  if (!memberPool.value || !validateMembers()) return

  try {
    savingMembers.value = true
    const groups: BillingPoolMemberRequest[] = memberRows.value.map((member, index) => ({
      group_id: member.group_id,
      chain_order: index + 1,
      can_be_primary: member.can_be_primary,
      can_be_fallback: member.can_be_fallback
    }))

    await adminAPI.billingPools.replaceMembers(memberPool.value.id, { groups })
    appStore.showSuccess(t('admin.billingPools.membersUpdated'))
    showMembersDialog.value = false
    memberPool.value = null
    memberRows.value = []
    await loadBillingPools()
  } catch (error) {
    appStore.showError(errorMessage(error, t('admin.billingPools.failedToSaveMembers')))
  } finally {
    savingMembers.value = false
  }
}

function openDeleteDialog(pool: BillingPool) {
  deletingPool.value = pool
  showDeleteDialog.value = true
}

async function confirmDelete() {
  if (!deletingPool.value) return
  try {
    await adminAPI.billingPools.delete(deletingPool.value.id)
    appStore.showSuccess(t('admin.billingPools.poolDeleted'))
    showDeleteDialog.value = false
    deletingPool.value = null
    await loadBillingPools()
  } catch (error) {
    appStore.showError(errorMessage(error, t('admin.billingPools.failedToDelete')))
  }
}

onMounted(() => {
  loadBillingPools()
  loadGroups()
})

onUnmounted(() => {
  currentController?.abort()
})
</script>
