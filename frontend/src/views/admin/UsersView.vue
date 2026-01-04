<template>
  <AppLayout>
    <TablePageLayout>
      <template #filters>
        <div class="flex flex-col gap-4 lg:flex-row lg:items-center lg:justify-between">
          <div class="flex flex-1 flex-wrap items-center gap-3">
            <div class="relative w-64">
              <svg class="absolute left-3 top-1/2 h-5 w-5 -translate-y-1/2 text-gray-400" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" d="M21 21l-5.197-5.197m0 0A7.5 7.5 0 105.196 5.196a7.5 7.5 0 0010.607 10.607z" /></svg>
              <input v-model="searchQuery" type="text" :placeholder="t('admin.users.searchUsers')" class="input pl-10" @input="handleSearch" />
            </div>
            <div v-if="visibleFilters.has('role')" class="w-32">
              <Select v-model="filters.role" :options="[{ value: '', label: t('admin.users.allRoles') REDACTED, { value: 'admin', label: t('admin.users.admin') REDACTED, { value: 'user', label: t('admin.users.user') REDACTED]" @change="applyFilter" />
            </div>
            <div v-if="visibleFilters.has('status')" class="w-32">
              <Select v-model="filters.status" :options="[{ value: '', label: t('admin.users.allStatus') REDACTED, { value: 'active', label: t('common.active') REDACTED, { value: 'disabled', label: t('admin.users.disabled') REDACTED]" @change="applyFilter" />
            </div>
          </div>
          <div class="flex items-center gap-3">
            <button @click="loadUsers" :disabled="loading" class="btn btn-secondary"><svg :class="['h-5 w-5', loading ? 'animate-spin' : '']" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" d="M16.023 9.348h4.992v-.001M2.985 19.644v-4.992m0 0h4.992m-4.993 0l3.181 3.183a8.25 8.25 0 0013.803-3.7M4.031 9.865a8.25 8.25 0 0113.803-3.7l3.181 3.182m0-4.991v4.99" /></svg></button>
            <button @click="showCreateModal = true" class="btn btn-primary"><svg class="mr-2 h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" d="M12 4.5v15m7.5-7.5h-15" /></svg>{{ t('admin.users.createUser') REDACTEDREDACTED</button>
          </div>
        </div>
      </template>

      <template #table>
        <DataTable :columns="columns" :data="users" :loading="loading" :actions-count="7">
          <template #cell-email="{ value REDACTED"><div class="flex items-center gap-2"><div class="flex h-8 w-8 items-center justify-center rounded-full bg-primary-100 font-medium text-primary-700"><span>{{ value.charAt(0).toUpperCase() REDACTEDREDACTED</span></div><span class="font-medium text-gray-900 dark:text-white">{{ value REDACTEDREDACTED</span></div></template>
          <template #cell-role="{ value REDACTED"><span :class="['badge', value === 'admin' ? 'badge-purple' : 'badge-gray']">{{ t('admin.users.roles.' + value) REDACTEDREDACTED</span></template>
          <template #cell-balance="{ value REDACTED"><span class="font-medium">${{ value.toFixed(2) REDACTEDREDACTED</span></template>
          <template #cell-status="{ value REDACTED"><div class="flex items-center gap-1.5"><span :class="['h-2 w-2 rounded-full', value === 'active' ? 'bg-green-500' : 'bg-red-500']"></span><span class="text-sm">{{ t('admin.accounts.status.' + (value === 'disabled' ? 'inactive' : value)) REDACTEDREDACTED</span></div></template>
          <template #cell-actions="{ row REDACTED"><div class="flex items-center gap-1"><button @click="handleEdit(row)" class="btn btn-sm btn-secondary">{{ t('common.edit') REDACTEDREDACTED</button><button @click="openActionMenu(row, $event)" class="btn btn-sm btn-secondary">{{ t('common.more') REDACTEDREDACTED</button></div></template>
        </DataTable>
      </template>

      <template #pagination>
        <Pagination v-if="pagination.total > 0" :page="pagination.page" :total="pagination.total" :page-size="pagination.page_size" @update:page="handlePageChange" @update:pageSize="handlePageSizeChange" />
      </template>
    </TablePageLayout>

    <Teleport to="body">
      <div v-if="activeMenuId !== null && menuPosition" class="action-menu-content fixed z-[9999] w-48 overflow-hidden rounded-xl bg-white shadow-lg ring-1 ring-black/5 dark:bg-dark-800" :style="{ top: menuPosition.top + 'px', left: menuPosition.left + 'px' REDACTED">
        <div class="py-1">
          <template v-for="user in users" :key="user.id">
            <template v-if="user.id === activeMenuId">
              <button @click="handleViewApiKeys(user); closeActionMenu()" class="flex w-full items-center gap-2 px-4 py-2 text-sm hover:bg-gray-100">{{ t('admin.users.apiKeys') REDACTEDREDACTED</button>
              <button @click="handleAllowedGroups(user); closeActionMenu()" class="flex w-full items-center gap-2 px-4 py-2 text-sm hover:bg-gray-100">{{ t('admin.users.groups') REDACTEDREDACTED</button>
              <button @click="handleDeposit(user); closeActionMenu()" class="flex w-full items-center gap-2 px-4 py-2 text-sm hover:bg-gray-100 text-emerald-600">{{ t('admin.users.deposit') REDACTEDREDACTED</button>
              <button @click="handleWithdraw(user); closeActionMenu()" class="flex w-full items-center gap-2 px-4 py-2 text-sm hover:bg-gray-100 text-amber-600">{{ t('admin.users.withdraw') REDACTEDREDACTED</button>
              <button v-if="user.role !== 'admin'" @click="handleDelete(user); closeActionMenu()" class="flex w-full items-center gap-2 px-4 py-2 text-sm text-red-600 hover:bg-red-50">{{ t('common.delete') REDACTEDREDACTED</button>
            </template>
          </template>
        </div>
      </div>
    </Teleport>

    <ConfirmDialog :show="showDeleteDialog" :title="t('admin.users.deleteUser')" :message="t('admin.users.deleteConfirm', { email: deletingUser?.email REDACTED)" :danger="true" @confirm="confirmDelete" @cancel="showDeleteDialog = false" />
    <UserCreateModal :show="showCreateModal" @close="showCreateModal = false" @success="loadUsers" />
    <UserEditModal :show="showEditModal" :user="editingUser" @close="closeEditModal" @success="loadUsers" />
    <UserApiKeysModal :show="showApiKeysModal" :user="viewingUser" @close="closeApiKeysModal" />
    <UserAllowedGroupsModal :show="showAllowedGroupsModal" :user="allowedGroupsUser" @close="closeAllowedGroupsModal" @success="loadUsers" />
    <UserBalanceModal :show="showBalanceModal" :user="balanceUser" :operation="balanceOperation" @close="closeBalanceModal" @success="loadUsers" />
    <UserAttributesConfigModal :show="showAttributesModal" @close="handleAttributesModalClose" />
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted REDACTED from 'vue'
import { useI18n REDACTED from 'vue-i18n'; import { useAppStore REDACTED from '@/stores/app'; import { formatDateTime REDACTED from '@/utils/format'
import { adminAPI REDACTED from '@/api/admin'; import type { User REDACTED from '@/types'
import AppLayout from '@/components/layout/AppLayout.vue'; import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import DataTable from '@/components/common/DataTable.vue'; import Pagination from '@/components/common/Pagination.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'; import Select from '@/components/common/Select.vue'
import UserAttributesConfigModal from '@/components/user/UserAttributesConfigModal.vue'
import UserCreateModal from '@/components/admin/user/UserCreateModal.vue'
import UserEditModal from '@/components/admin/user/UserEditModal.vue'
import UserApiKeysModal from '@/components/admin/user/UserApiKeysModal.vue'
import UserAllowedGroupsModal from '@/components/admin/user/UserAllowedGroupsModal.vue'
import UserBalanceModal from '@/components/admin/user/UserBalanceModal.vue'

const { t REDACTED = useI18n(); const appStore = useAppStore()
const users = ref<User[]>([]); const loading = ref(false); const searchQuery = ref('')
const filters = reactive({ role: '', status: '' REDACTED); const visibleFilters = reactive<Set<string>>(new Set(['role', 'status']))
const pagination = reactive({ page: 1, page_size: 20, total: 0 REDACTED)
const showCreateModal = ref(false); const showEditModal = ref(false); const showDeleteDialog = ref(false); const showApiKeysModal = ref(false); const showAttributesModal = ref(false)
const editingUser = ref<User | null>(null); const deletingUser = ref<User | null>(null); const viewingUser = ref<User | null>(null)
const activeMenuId = ref<number | null>(null); const menuPosition = ref<{ top: number; left: number REDACTED | null>(null)
const showAllowedGroupsModal = ref(false); const allowedGroupsUser = ref<User | null>(null); const showBalanceModal = ref(false); const balanceUser = ref<User | null>(null); const balanceOperation = ref<'add' | 'subtract'>('add')

const columns = computed(() => [{ key: 'email', label: t('admin.users.columns.user'), sortable: true REDACTED, { key: 'role', label: t('admin.users.columns.role'), sortable: true REDACTED, { key: 'balance', label: t('admin.users.columns.balance'), sortable: true REDACTED, { key: 'status', label: t('admin.users.columns.status'), sortable: true REDACTED, { key: 'actions', label: t('admin.users.columns.actions') REDACTED])

const loadUsers = async () => {
  loading.value = true
  try {
    const res = await adminAPI.users.list(pagination.page, pagination.page_size, { role: filters.role as any, status: filters.status as any, search: searchQuery.value || undefined REDACTED)
    users.value = res.items; pagination.total = res.total
  REDACTED catch {REDACTED finally { loading.value = false REDACTED
REDACTED
const handleSearch = () => { pagination.page = 1; loadUsers() REDACTED
const handlePageChange = (p: number) => { pagination.page = p; loadUsers() REDACTED
const handlePageSizeChange = (s: number) => { pagination.page_size = s; pagination.page = 1; loadUsers() REDACTED
const applyFilter = () => { pagination.page = 1; loadUsers() REDACTED
const handleEdit = (u: User) => { editingUser.value = u; showEditModal.value = true REDACTED
const closeEditModal = () => { showEditModal.value = false; editingUser.value = null REDACTED
const handleViewApiKeys = (u: User) => { viewingUser.value = u; showApiKeysModal.value = true REDACTED
const closeApiKeysModal = () => { showApiKeysModal.value = false; viewingUser.value = null REDACTED
const handleAllowedGroups = (u: User) => { allowedGroupsUser.value = u; showAllowedGroupsModal.value = true REDACTED
const closeAllowedGroupsModal = () => { showAllowedGroupsModal.value = false; allowedGroupsUser.value = null REDACTED
const handleDelete = (u: User) => { deletingUser.value = u; showDeleteDialog.value = true REDACTED
const confirmDelete = async () => { if (!deletingUser.value) return; try { await adminAPI.users.delete(deletingUser.value.id); appStore.showSuccess(t('common.success')); showDeleteDialog.value = false; loadUsers() REDACTED catch {REDACTED REDACTED
const handleDeposit = (u: User) => { balanceUser.value = u; balanceOperation.value = 'add'; showBalanceModal.value = true REDACTED
const handleWithdraw = (u: User) => { balanceUser.value = u; balanceOperation.value = 'subtract'; showBalanceModal.value = true REDACTED
const closeBalanceModal = () => { showBalanceModal.value = false; balanceUser.value = null REDACTED
const handleAttributesModalClose = () => { showAttributesModal.value = false; loadUsers() REDACTED
const getAttributeDefinitionName = (id: number) => String(id)
const getAttributeDefinition = (id: number) => ({REDACTED as any)

const openActionMenu = (u: User, e: MouseEvent) => {
  if (activeMenuId.value === u.id) { activeMenuId.value = null; menuPosition.value = null REDACTED
  else { activeMenuId.value = u.id; menuPosition.value = { top: e.clientY, left: e.clientX - 150 REDACTED REDACTED
REDACTED
const closeActionMenu = () => { activeMenuId.value = null; menuPosition.value = null REDACTED

onMounted(loadUsers)
</script>
