<template>
  <AppLayout>
    <TablePageLayout>
      <template #filters>
        <div class="flex flex-col gap-4 lg:flex-row lg:items-center lg:justify-between">
          <div class="flex flex-1 flex-wrap items-center gap-3">
            <div class="w-64">
              <SearchInput v-model="params.search" :placeholder="t('admin.users.searchUsers')" @search="reload" />
            </div>
            <div class="w-32">
              <Select v-model="params.role" :options="[{ value: '', label: t('admin.users.allRoles') REDACTED, { value: 'admin', label: t('admin.users.admin') REDACTED, { value: 'user', label: t('admin.users.user') REDACTED]" @change="reload" />
            </div>
            <div class="w-32">
              <Select v-model="params.status" :options="[{ value: '', label: t('admin.users.allStatus') REDACTED, { value: 'active', label: t('common.active') REDACTED, { value: 'disabled', label: t('admin.users.disabled') REDACTED]" @change="reload" />
            </div>
          </div>
          <div class="flex items-center gap-3">
            <button @click="load" :disabled="loading" class="btn btn-secondary"><svg :class="['h-5 w-5', loading ? 'animate-spin' : '']" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" d="M16.023 9.348h4.992v-.001M2.985 19.644v-4.992m0 0h4.992m-4.993 0l3.181 3.183a8.25 8.25 0 0013.803-3.7M4.031 9.865a8.25 8.25 0 0113.803-3.7l3.181 3.182m0-4.991v4.99" /></svg></button>
            <button @click="showCreateModal = true" class="btn btn-primary"><svg class="mr-2 h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" d="M12 4.5v15m7.5-7.5h-15" /></svg>{{ t('admin.users.createUser') REDACTEDREDACTED</button>
          </div>
        </div>
      </template>

      <template #table>
        <DataTable :columns="columns" :data="users" :loading="loading">
          <template #cell-email="{ value REDACTED"><div class="flex items-center gap-2"><div class="flex h-8 w-8 items-center justify-center rounded-full bg-primary-100 font-medium text-primary-700"><span>{{ value.charAt(0).toUpperCase() REDACTEDREDACTED</span></div><span class="font-medium text-gray-900 dark:text-white">{{ value REDACTEDREDACTED</span></div></template>
          <template #cell-role="{ value REDACTED"><span :class="['badge', value === 'admin' ? 'badge-purple' : 'badge-gray']">{{ t('admin.users.roles.' + value) REDACTEDREDACTED</span></template>
          <template #cell-balance="{ value REDACTED"><span class="font-medium">${{ value.toFixed(2) REDACTEDREDACTED</span></template>
          <template #cell-status="{ value REDACTED"><StatusBadge :status="value === 'disabled' ? 'inactive' : value" :label="t('admin.accounts.status.' + (value === 'disabled' ? 'inactive' : value))" /></template>
          <template #cell-actions="{ row REDACTED"><div class="flex gap-1"><button @click="handleEdit(row)" class="btn btn-sm btn-secondary">{{ t('common.edit') REDACTEDREDACTED</button><button @click="openActionMenu(row, $event)" class="btn btn-sm btn-secondary">{{ t('common.more') REDACTEDREDACTED</button></div></template>
        </DataTable>
      </template>

      <template #pagination>
        <Pagination v-if="pagination.total > 0" :page="pagination.page" :total="pagination.total" :page-size="pagination.page_size" @update:page="handlePageChange" />
      </template>
    </TablePageLayout>

    <Teleport to="body">
      <div v-if="activeMenuId !== null && menuPosition" ref="actionMenuEl" class="action-menu-content fixed z-[9999] w-48 max-h-[calc(100vh-16px)] overflow-auto rounded-xl bg-white shadow-lg ring-1 ring-black/5 dark:bg-dark-800" :style="{ top: menuPosition.top + 'px', left: menuPosition.left + 'px' REDACTED">
        <div class="py-1">
          <template v-for="user in users" :key="user.id">
            <template v-if="user.id === activeMenuId">
              <button @click="handleViewApiKeys(user); closeActionMenu()" class="flex w-full items-center gap-2 px-4 py-2 text-sm hover:bg-gray-100">{{ t('admin.users.apiKeys') REDACTEDREDACTED</button>
              <button @click="handleAllowedGroups(user); closeActionMenu()" class="flex w-full items-center gap-2 px-4 py-2 text-sm hover:bg-gray-100">{{ t('admin.users.groups') REDACTEDREDACTED</button>
              <button @click="handleDeposit(user); closeActionMenu()" class="flex w-full items-center gap-2 px-4 py-2 text-sm hover:bg-gray-100 text-emerald-600">{{ t('admin.users.deposit') REDACTEDREDACTED</button>
              <button @click="handleWithdraw(user); closeActionMenu()" class="flex w-full items-center gap-2 px-4 py-2 text-sm hover:bg-gray-100 text-amber-600">{{ t('admin.users.withdraw') REDACTEDREDACTED</button>
              <button v-if="user.role !== 'admin'" @click="handleToggleStatus(user); closeActionMenu()" class="flex w-full items-center gap-2 px-4 py-2 text-sm hover:bg-gray-100">{{ user.status === 'active' ? t('admin.users.disable') : t('admin.users.enable') REDACTEDREDACTED</button>
              <button v-if="user.role !== 'admin'" @click="handleDelete(user); closeActionMenu()" class="flex w-full items-center gap-2 px-4 py-2 text-sm text-red-600 hover:bg-red-50">{{ t('common.delete') REDACTEDREDACTED</button>
            </template>
          </template>
        </div>
      </div>
    </Teleport>

    <ConfirmDialog :show="showDeleteDialog" :title="t('admin.users.deleteUser')" :message="t('admin.users.deleteConfirm', { email: deletingUser?.email REDACTED)" :danger="true" @confirm="confirmDelete" @cancel="showDeleteDialog = false" />
    <UserCreateModal :show="showCreateModal" @close="showCreateModal = false" @success="reload" />
    <UserEditModal :show="showEditModal" :user="editingUser" @close="closeEditModal" @success="load" />
    <UserApiKeysModal :show="showApiKeysModal" :user="viewingUser" @close="closeApiKeysModal" />
    <UserAllowedGroupsModal :show="showAllowedGroupsModal" :user="allowedGroupsUser" @close="closeAllowedGroupsModal" @success="load" />
    <UserBalanceModal :show="showBalanceModal" :user="balanceUser" :operation="balanceOperation" @close="closeBalanceModal" @success="load" />
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onBeforeUnmount, nextTick REDACTED from 'vue'
import { useI18n REDACTED from 'vue-i18n'; import { useAppStore REDACTED from '@/stores/app'
import { adminAPI REDACTED from '@/api/admin'; import { useTableLoader REDACTED from '@/composables/useTableLoader'
import type { User REDACTED from '@/types'
import AppLayout from '@/components/layout/AppLayout.vue'; import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import DataTable from '@/components/common/DataTable.vue'; import Pagination from '@/components/common/Pagination.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'; import Select from '@/components/common/Select.vue'
import SearchInput from '@/components/common/SearchInput.vue'; import StatusBadge from '@/components/common/StatusBadge.vue'
import UserCreateModal from '@/components/admin/user/UserCreateModal.vue'
import UserEditModal from '@/components/admin/user/UserEditModal.vue'
import UserApiKeysModal from '@/components/admin/user/UserApiKeysModal.vue'
import UserAllowedGroupsModal from '@/components/admin/user/UserAllowedGroupsModal.vue'
import UserBalanceModal from '@/components/admin/user/UserBalanceModal.vue'

const { t REDACTED = useI18n(); const appStore = useAppStore()
const { items: users, loading, params, pagination, load, reload, handlePageChange REDACTED = useTableLoader<User, any>({ fetchFn: adminAPI.users.list, initialParams: { role: '', status: '', search: '' REDACTED REDACTED)

const showCreateModal = ref(false); const showEditModal = ref(false); const showDeleteDialog = ref(false); const showApiKeysModal = ref(false)
const editingUser = ref<User | null>(null); const deletingUser = ref<User | null>(null); const viewingUser = ref<User | null>(null)
const activeMenuId = ref<number | null>(null); const menuPosition = ref<{ top: number; left: number REDACTED | null>(null)
const actionMenuEl = ref<HTMLElement | null>(null)
const showAllowedGroupsModal = ref(false); const allowedGroupsUser = ref<User | null>(null); const showBalanceModal = ref(false); const balanceUser = ref<User | null>(null); const balanceOperation = ref<'add' | 'subtract'>('add')
const columns = computed(() => [{ key: 'email', label: t('admin.users.columns.user'), sortable: true REDACTED, { key: 'role', label: t('admin.users.columns.role'), sortable: true REDACTED, { key: 'balance', label: t('admin.users.columns.balance'), sortable: true REDACTED, { key: 'status', label: t('admin.users.columns.status'), sortable: true REDACTED, { key: 'actions', label: t('admin.users.columns.actions') REDACTED])

const handleEdit = (u: User) => { editingUser.value = u; showEditModal.value = true REDACTED
const closeEditModal = () => { showEditModal.value = false; editingUser.value = null REDACTED
const handleViewApiKeys = (u: User) => { viewingUser.value = u; showApiKeysModal.value = true REDACTED
const closeApiKeysModal = () => { showApiKeysModal.value = false; viewingUser.value = null REDACTED
const handleAllowedGroups = (u: User) => { allowedGroupsUser.value = u; showAllowedGroupsModal.value = true REDACTED
const closeAllowedGroupsModal = () => { showAllowedGroupsModal.value = false; allowedGroupsUser.value = null REDACTED
const handleDelete = (u: User) => { deletingUser.value = u; showDeleteDialog.value = true REDACTED
const confirmDelete = async () => { if (!deletingUser.value) return; try { await adminAPI.users.delete(deletingUser.value.id); appStore.showSuccess(t('common.success')); showDeleteDialog.value = false; reload() REDACTED catch {REDACTED REDACTED
const handleDeposit = (u: User) => { balanceUser.value = u; balanceOperation.value = 'add'; showBalanceModal.value = true REDACTED
const handleWithdraw = (u: User) => { balanceUser.value = u; balanceOperation.value = 'subtract'; showBalanceModal.value = true REDACTED
const closeBalanceModal = () => { showBalanceModal.value = false; balanceUser.value = null REDACTED
const handleToggleStatus = async (user: User) => { const next = user.status === 'active' ? 'disabled' : 'active'; try { await adminAPI.users.toggleStatus(user.id, next as any); appStore.showSuccess(t('common.success')); load() REDACTED catch {REDACTED REDACTED
const repositionActionMenu = (triggerRect?: DOMRect) => {
  if (!menuPosition.value || !actionMenuEl.value) return
  const rect = actionMenuEl.value.getBoundingClientRect()
  const margin = 8
  let top = menuPosition.value.top
  let left = menuPosition.value.left

  if (triggerRect) {
    const spaceBelow = window.innerHeight - triggerRect.bottom
    const spaceAbove = triggerRect.top
    if (rect.height > spaceBelow && spaceAbove > spaceBelow) top = Math.max(margin, triggerRect.top - rect.height - 4)
  REDACTED

  if (left + rect.width + margin > window.innerWidth) left = window.innerWidth - rect.width - margin
  if (left < margin) left = margin
  if (top + rect.height + margin > window.innerHeight) top = window.innerHeight - rect.height - margin
  if (top < margin) top = margin

  menuPosition.value = { top, left REDACTED
REDACTED
const openActionMenu = async (u: User, e: MouseEvent) => {
  e.stopPropagation()
  if (activeMenuId.value === u.id) { closeActionMenu(); return REDACTED

  const actionMenuWidthPx = 192 // w-48
  const triggerEl = e.currentTarget as HTMLElement | null
  const triggerRect = triggerEl?.getBoundingClientRect()

  activeMenuId.value = u.id
  if (triggerRect) menuPosition.value = { top: triggerRect.bottom + 4, left: triggerRect.right - actionMenuWidthPx REDACTED
  else menuPosition.value = { top: e.clientY, left: e.clientX - actionMenuWidthPx REDACTED

  await nextTick()
  repositionActionMenu(triggerRect)
REDACTED
const closeActionMenu = () => { activeMenuId.value = null; menuPosition.value = null REDACTED

const handleDocumentClick = (evt: MouseEvent) => { if (activeMenuId.value === null) return; const target = evt.target as Node | null; if (target && actionMenuEl.value?.contains(target)) return; closeActionMenu() REDACTED
const handleWindowResize = () => repositionActionMenu()
const handleAnyScroll = () => closeActionMenu()

onMounted(() => { load(); document.addEventListener('click', handleDocumentClick); window.addEventListener('resize', handleWindowResize); window.addEventListener('scroll', handleAnyScroll, true) REDACTED)
onBeforeUnmount(() => { document.removeEventListener('click', handleDocumentClick); window.removeEventListener('resize', handleWindowResize); window.removeEventListener('scroll', handleAnyScroll, true) REDACTED)
</script>
