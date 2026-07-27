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
              :placeholder="t('admin.proxyGroups.search')"
              class="input pl-10"
            />
          </div>
          <div class="flex flex-1 flex-wrap items-center justify-end gap-2">
            <button @click="loadGroups" :disabled="loading" class="btn btn-secondary" :title="t('common.refresh')">
              <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
            </button>
            <button @click="openCreate" class="btn btn-primary">
              <Icon name="plus" size="md" class="mr-2" />
              {{ t('admin.proxyGroups.create') }}
            </button>
          </div>
        </div>
      </template>

      <template #table>
        <DataTable :columns="columns" :data="filteredGroups" :loading="loading">
          <template #cell-name="{ row }">
            <div class="font-medium text-gray-900 dark:text-white">{{ row.name }}</div>
            <div v-if="row.description" class="text-xs text-gray-500">{{ row.description }}</div>
          </template>
          <template #cell-strategy="{ row }">
            <span class="badge badge-gray">{{ strategyLabel(row) }}</span>
          </template>
          <template #cell-proxy_count="{ row }">
            {{ row.proxy_count ?? row.proxies?.length ?? 0 }}
          </template>
          <template #cell-status="{ row }">
            <span :class="row.status === 'active' ? 'badge badge-success' : 'badge badge-gray'">
              {{ row.status === 'active' ? t('common.active') : t('common.inactive') }}
            </span>
          </template>
          <template #cell-actions="{ row }">
            <div class="flex items-center gap-2">
              <button class="btn-icon" :title="t('common.edit')" @click="openEdit(row)">
                <Icon name="edit" size="sm" />
              </button>
              <button class="btn-icon text-red-500" :title="t('common.delete')" @click="confirmDelete(row)">
                <Icon name="trash" size="sm" />
              </button>
            </div>
          </template>
          <template #empty>
            <EmptyState
              :title="t('admin.proxyGroups.emptyTitle')"
              :description="t('admin.proxyGroups.emptyDesc')"
            />
          </template>
        </DataTable>
      </template>

      <template #pagination>
        <Pagination
          :page="page"
          :page-size="pageSize"
          :total="total"
          @update:page="(p: number) => { page = p; loadGroups() }"
          @update:page-size="(s: number) => { pageSize = s; page = 1; loadGroups() }"
        />
      </template>
    </TablePageLayout>

    <BaseDialog
      :show="showForm"
      :title="editing ? t('admin.proxyGroups.edit') : t('admin.proxyGroups.create')"
      width="wide"
      @close="closeForm"
    >
      <div class="space-y-4">
        <div>
          <label class="input-label">{{ t('admin.proxyGroups.name') }}</label>
          <input v-model="form.name" type="text" class="input" :placeholder="t('admin.proxyGroups.namePlaceholder')" />
        </div>
        <div>
          <label class="input-label">{{ t('admin.proxyGroups.descriptionLabel') }}</label>
          <textarea v-model="form.description" class="input" rows="2" />
        </div>
        <div class="grid grid-cols-2 gap-4">
          <div>
            <label class="input-label">{{ t('admin.proxyGroups.strategy') }}</label>
            <Select v-model="form.strategy" :options="strategyOptions" />
          </div>
          <div>
            <label class="input-label">{{ t('admin.proxyGroups.status') }}</label>
            <Select v-model="form.status" :options="statusOptions" />
          </div>
        </div>
        <div class="flex items-center gap-2">
          <input id="sticky" v-model="form.sticky_by_account" type="checkbox" class="h-4 w-4 rounded" />
          <label for="sticky" class="text-sm text-gray-700 dark:text-gray-300">
            {{ t('admin.proxyGroups.stickyByAccount') }}
          </label>
        </div>
        <p class="text-xs text-gray-500">{{ t('admin.proxyGroups.stickyHint') }}</p>

        <!-- Member proxies: search + batch select + pagination -->
        <div>
          <div class="mb-2 flex flex-wrap items-center justify-between gap-2">
            <label class="input-label mb-0">{{ t('admin.proxyGroups.members') }}</label>
            <span class="text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.proxyGroups.membersSelected', { count: form.proxy_ids.length }) }}
              ·
              {{ t('admin.proxyGroups.membersFiltered', { count: filteredMemberProxies.length }) }}
            </span>
          </div>

          <div class="relative mb-2">
            <Icon
              name="search"
              size="sm"
              class="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-gray-400"
            />
            <input
              v-model="memberSearch"
              type="text"
              class="input pl-9 text-sm"
              :placeholder="t('admin.proxyGroups.membersSearch')"
            />
          </div>

          <div class="mb-2 flex flex-wrap items-center gap-2">
            <label class="inline-flex cursor-pointer items-center gap-1.5 text-sm text-gray-700 dark:text-gray-300">
              <input
                type="checkbox"
                class="h-4 w-4 rounded"
                :checked="allPageMembersSelected"
                :indeterminate.prop="somePageMembersSelected && !allPageMembersSelected"
                :disabled="pagedMemberProxies.length === 0"
                @change="togglePageMembers(($event.target as HTMLInputElement).checked)"
              />
              {{ t('admin.proxyGroups.selectPage') }}
            </label>
            <button
              type="button"
              class="btn btn-secondary btn-sm"
              :disabled="filteredMemberProxies.length === 0"
              @click="selectAllFilteredMembers"
            >
              {{ t('admin.proxyGroups.selectAllFiltered') }}
            </button>
            <button
              type="button"
              class="btn btn-secondary btn-sm"
              :disabled="form.proxy_ids.length === 0"
              @click="clearMemberSelection"
            >
              {{ t('admin.proxyGroups.clearSelection') }}
            </button>
          </div>

          <div class="max-h-64 space-y-1 overflow-auto rounded border border-gray-200 p-2 dark:border-dark-600">
            <label
              v-for="proxy in pagedMemberProxies"
              :key="proxy.id"
              class="flex cursor-pointer items-center gap-2 rounded px-2 py-1.5 hover:bg-gray-50 dark:hover:bg-dark-800"
            >
              <input v-model="form.proxy_ids" type="checkbox" :value="proxy.id" class="h-4 w-4 shrink-0 rounded" />
              <span class="min-w-0 flex-1 truncate text-sm font-medium text-gray-900 dark:text-gray-100">
                {{ proxy.name }}
              </span>
              <span class="shrink-0 text-xs text-gray-400">
                {{ proxy.protocol }}://{{ proxy.host }}:{{ proxy.port }}
              </span>
            </label>
            <div
              v-if="allProxies.length === 0"
              class="py-4 text-center text-sm text-gray-500"
            >
              {{ t('admin.proxyGroups.noProxies') }}
            </div>
            <div
              v-else-if="filteredMemberProxies.length === 0"
              class="py-4 text-center text-sm text-gray-500"
            >
              {{ t('admin.proxyGroups.noMatchingProxies') }}
            </div>
          </div>

          <div
            v-if="filteredMemberProxies.length > 0"
            class="mt-2 flex flex-wrap items-center justify-between gap-2 border-t border-gray-100 pt-2 dark:border-dark-700"
          >
            <div class="flex items-center gap-2 text-xs text-gray-500">
              <span>{{ t('pagination.perPage') }}:</span>
              <select
                v-model.number="memberPageSize"
                class="input w-auto py-1 text-xs"
                @change="memberPage = 1"
              >
                <option v-for="size in memberPageSizeOptions" :key="size" :value="size">
                  {{ size }}
                </option>
              </select>
            </div>
            <div class="flex items-center gap-2">
              <button
                type="button"
                class="btn btn-secondary btn-sm"
                :disabled="memberPage <= 1"
                @click="memberPage--"
              >
                {{ t('pagination.previous') }}
              </button>
              <span class="text-xs text-gray-600 dark:text-gray-300">
                {{ t('pagination.pageOf', { page: memberPage, total: memberTotalPages }) }}
              </span>
              <button
                type="button"
                class="btn btn-secondary btn-sm"
                :disabled="memberPage >= memberTotalPages"
                @click="memberPage++"
              >
                {{ t('pagination.next') }}
              </button>
            </div>
          </div>
        </div>
      </div>
      <template #footer>
        <div class="flex justify-end gap-2">
          <button class="btn btn-secondary" @click="closeForm">{{ t('common.cancel') }}</button>
          <button class="btn btn-primary" :disabled="submitting" @click="submitForm">
            {{ submitting ? t('common.saving') : t('common.save') }}
          </button>
        </div>
      </template>
    </BaseDialog>

    <ConfirmDialog
      :show="!!deleting"
      :title="t('admin.proxyGroups.delete')"
      :message="t('admin.proxyGroups.deleteConfirm', { name: deleting?.name || '' })"
      @confirm="doDelete"
      @cancel="deleting = null"
    />
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, reactive, computed, watch, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { adminAPI } from '@/api/admin'
import type { Proxy, ProxyGroup, ProxyGroupStrategy } from '@/types'
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

const { t } = useI18n()
const appStore = useAppStore()

const loading = ref(false)
const submitting = ref(false)
const groups = ref<ProxyGroup[]>([])
const allProxies = ref<Proxy[]>([])
const page = ref(1)
const pageSize = ref(20)
const total = ref(0)
const searchQuery = ref('')
const showForm = ref(false)
const editing = ref<ProxyGroup | null>(null)
const deleting = ref<ProxyGroup | null>(null)

// Member proxies picker state (dialog)
const memberSearch = ref('')
const memberPage = ref(1)
const memberPageSize = ref(20)
const memberPageSizeOptions = [10, 20, 50, 100]

const form = reactive({
  name: '',
  description: '',
  strategy: 'round_robin' as ProxyGroupStrategy,
  sticky_by_account: false,
  status: 'active' as 'active' | 'inactive',
  proxy_ids: [] as number[]
})

const columns = computed<Column[]>(() => [
  { key: 'name', label: t('admin.proxyGroups.columns.name'), sortable: false },
  { key: 'strategy', label: t('admin.proxyGroups.columns.strategy'), sortable: false },
  { key: 'proxy_count', label: t('admin.proxyGroups.columns.members'), sortable: false },
  { key: 'status', label: t('admin.proxyGroups.columns.status'), sortable: false },
  { key: 'actions', label: t('admin.proxyGroups.columns.actions'), sortable: false }
])

const strategyOptions = computed(() => [
  { value: 'round_robin', label: t('admin.proxyGroups.strategies.round_robin') },
  { value: 'random', label: t('admin.proxyGroups.strategies.random') },
  { value: 'sticky', label: t('admin.proxyGroups.strategies.sticky') }
])

const statusOptions = computed(() => [
  { value: 'active', label: t('common.active') },
  { value: 'inactive', label: t('common.inactive') }
])

const filteredGroups = computed(() => {
  const q = searchQuery.value.trim().toLowerCase()
  if (!q) return groups.value
  return groups.value.filter(
    (g) => g.name.toLowerCase().includes(q) || (g.description || '').toLowerCase().includes(q)
  )
})

const filteredMemberProxies = computed(() => {
  const q = memberSearch.value.trim().toLowerCase()
  if (!q) return allProxies.value
  return allProxies.value.filter((p) => {
    const hay = `${p.name} ${p.protocol} ${p.host} ${p.port}`.toLowerCase()
    return hay.includes(q)
  })
})

const memberTotalPages = computed(() =>
  Math.max(1, Math.ceil(filteredMemberProxies.value.length / memberPageSize.value))
)

const pagedMemberProxies = computed(() => {
  const start = (memberPage.value - 1) * memberPageSize.value
  return filteredMemberProxies.value.slice(start, start + memberPageSize.value)
})

const pageMemberIds = computed(() => pagedMemberProxies.value.map((p) => p.id))

const allPageMembersSelected = computed(() => {
  const ids = pageMemberIds.value
  if (ids.length === 0) return false
  const selected = new Set(form.proxy_ids)
  return ids.every((id) => selected.has(id))
})

const somePageMembersSelected = computed(() => {
  const ids = pageMemberIds.value
  if (ids.length === 0) return false
  const selected = new Set(form.proxy_ids)
  return ids.some((id) => selected.has(id))
})

watch(memberSearch, () => {
  memberPage.value = 1
})

watch([filteredMemberProxies, memberPageSize], () => {
  if (memberPage.value > memberTotalPages.value) {
    memberPage.value = memberTotalPages.value
  }
})

function strategyLabel(row: ProxyGroup) {
  if (row.sticky_by_account) return t('admin.proxyGroups.strategies.sticky')
  const key = row.strategy as ProxyGroupStrategy
  return t(`admin.proxyGroups.strategies.${key}`)
}

function resetMemberPicker() {
  memberSearch.value = ''
  memberPage.value = 1
}

function togglePageMembers(checked: boolean) {
  const pageIds = pageMemberIds.value
  if (pageIds.length === 0) return
  const set = new Set(form.proxy_ids)
  if (checked) {
    pageIds.forEach((id) => set.add(id))
  } else {
    pageIds.forEach((id) => set.delete(id))
  }
  form.proxy_ids = Array.from(set)
}

function selectAllFilteredMembers() {
  const set = new Set(form.proxy_ids)
  filteredMemberProxies.value.forEach((p) => set.add(p.id))
  form.proxy_ids = Array.from(set)
}

function clearMemberSelection() {
  form.proxy_ids = []
}

async function loadGroups() {
  loading.value = true
  try {
    const res = await adminAPI.proxyGroups.list(page.value, pageSize.value)
    groups.value = res.items || (res as unknown as { data?: ProxyGroup[] }).data || []
    total.value = res.total ?? groups.value.length
  } catch (e: unknown) {
    appStore.showError(t('admin.proxyGroups.failedToLoad'))
    console.error(e)
  } finally {
    loading.value = false
  }
}

async function loadProxies() {
  try {
    allProxies.value = await adminAPI.proxies.getAll()
  } catch (e) {
    console.error(e)
  }
}

function openCreate() {
  editing.value = null
  form.name = ''
  form.description = ''
  form.strategy = 'round_robin'
  form.sticky_by_account = false
  form.status = 'active'
  form.proxy_ids = []
  resetMemberPicker()
  showForm.value = true
}

async function openEdit(row: ProxyGroup) {
  editing.value = row
  form.name = row.name
  form.description = row.description || ''
  form.strategy = row.strategy || 'round_robin'
  form.sticky_by_account = !!row.sticky_by_account
  form.status = row.status === 'inactive' ? 'inactive' : 'active'
  resetMemberPicker()
  try {
    const detail = await adminAPI.proxyGroups.getById(row.id)
    form.proxy_ids = (detail.proxies || []).map((p) => p.id)
  } catch {
    form.proxy_ids = []
  }
  showForm.value = true
}

function closeForm() {
  showForm.value = false
  editing.value = null
}

async function submitForm() {
  if (!form.name.trim()) {
    appStore.showError(t('admin.proxyGroups.nameRequired'))
    return
  }
  submitting.value = true
  try {
    if (editing.value) {
      await adminAPI.proxyGroups.update(editing.value.id, {
        name: form.name.trim(),
        description: form.description,
        strategy: form.strategy,
        sticky_by_account: form.sticky_by_account,
        status: form.status,
        proxy_ids: form.proxy_ids
      })
      appStore.showSuccess(t('admin.proxyGroups.updated'))
    } else {
      await adminAPI.proxyGroups.create({
        name: form.name.trim(),
        description: form.description,
        strategy: form.strategy,
        sticky_by_account: form.sticky_by_account,
        status: form.status,
        proxy_ids: form.proxy_ids
      })
      appStore.showSuccess(t('admin.proxyGroups.created'))
    }
    closeForm()
    await loadGroups()
  } catch (e: unknown) {
    const msg = e instanceof Error ? e.message : t('admin.proxyGroups.failedToSave')
    appStore.showError(msg)
  } finally {
    submitting.value = false
  }
}

function confirmDelete(row: ProxyGroup) {
  deleting.value = row
}

async function doDelete() {
  if (!deleting.value) return
  try {
    await adminAPI.proxyGroups.delete(deleting.value.id)
    appStore.showSuccess(t('admin.proxyGroups.deleted'))
    deleting.value = null
    await loadGroups()
  } catch (e: unknown) {
    const msg = e instanceof Error ? e.message : t('admin.proxyGroups.failedToDelete')
    appStore.showError(msg)
  }
}

onMounted(async () => {
  await Promise.all([loadGroups(), loadProxies()])
})
</script>
