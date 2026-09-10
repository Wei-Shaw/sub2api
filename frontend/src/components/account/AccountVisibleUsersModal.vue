<template>
  <BaseDialog
    :show="show"
    :title="t('admin.accounts.visibleUsers.title')"
    width="wide"
    :z-index="60"
    @close="emit('close')"
  >
    <div class="space-y-4">
      <div class="rounded-md border border-blue-100 bg-blue-50 px-3 py-2 text-sm text-blue-700 dark:border-blue-900/60 dark:bg-blue-950/30 dark:text-blue-300">
        {{ t('admin.accounts.visibleUsers.description', { account: account?.name || '' }) }}
      </div>

      <div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <SearchInput
          v-model="search"
          class="w-full sm:w-80"
          :placeholder="t('admin.accounts.visibleUsers.searchPlaceholder')"
          @search="loadCandidates"
        />
        <span class="text-sm text-gray-500 dark:text-gray-400">
          {{ t('admin.accounts.visibleUsers.selectedCount', { count: selectedIds.size }) }}
        </span>
      </div>

      <div v-if="selectedUsers.length" class="flex max-h-28 flex-wrap gap-2 overflow-y-auto rounded-md border border-gray-200 bg-gray-50 p-3 dark:border-dark-600 dark:bg-dark-800">
        <button
          v-for="user in selectedUsers"
          :key="user.id"
          type="button"
          class="inline-flex max-w-full items-center gap-1.5 rounded-md border border-gray-200 bg-white px-2 py-1 text-xs text-gray-700 hover:border-red-200 hover:text-red-600 dark:border-dark-600 dark:bg-dark-700 dark:text-gray-200"
          :title="t('admin.accounts.visibleUsers.removeUser')"
          @click="removeUser(user.id)"
        >
          <span class="max-w-56 truncate">{{ user.username || user.email }}</span>
          <Icon name="x" size="xs" />
        </button>
      </div>

      <div class="overflow-hidden rounded-md border border-gray-200 dark:border-dark-600">
        <div v-if="loading" class="flex min-h-72 items-center justify-center text-sm text-gray-500 dark:text-gray-400">
          {{ t('common.loading') }}
        </div>
        <div v-else-if="candidates.length === 0" class="flex min-h-72 flex-col items-center justify-center text-center text-sm text-gray-500 dark:text-gray-400">
          <Icon name="users" size="lg" class="mb-3 text-gray-300 dark:text-dark-500" />
          {{ t('admin.accounts.visibleUsers.noUsers') }}
        </div>
        <div v-else class="max-h-80 divide-y divide-gray-100 overflow-y-auto dark:divide-dark-700">
          <label
            v-for="user in candidates"
            :key="user.id"
            class="flex cursor-pointer items-center gap-3 px-4 py-3 hover:bg-gray-50 dark:hover:bg-dark-700"
          >
            <input
              type="checkbox"
              class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500 dark:border-dark-500"
              :checked="selectedIds.has(user.id)"
              @change="toggleUser(user)"
            />
            <span class="min-w-0 flex-1">
              <span class="block truncate text-sm font-medium text-gray-900 dark:text-gray-100">
                {{ user.username || user.email }}
              </span>
              <span v-if="user.username" class="block truncate text-xs text-gray-500 dark:text-gray-400">{{ user.email }}</span>
            </span>
            <span class="rounded bg-gray-100 px-2 py-0.5 text-xs text-gray-500 dark:bg-dark-600 dark:text-gray-300">
              {{ user.role === 'admin' ? t('admin.accounts.visibleUsers.adminRole') : t('admin.accounts.visibleUsers.userRole') }}
            </span>
          </label>
        </div>
      </div>

      <Pagination
        v-if="total > pageSize"
        :page="page"
        :total="total"
        :page-size="pageSize"
        :show-page-size-selector="false"
        @update:page="handlePageChange"
      />
    </div>

    <template #footer>
      <div class="flex justify-end gap-3">
        <button type="button" class="btn btn-secondary" @click="emit('close')">{{ t('common.cancel') }}</button>
        <button type="button" class="btn btn-primary" :disabled="saving || loadingInitial" @click="save">
          {{ saving ? t('common.saving') : t('common.save') }}
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { useDebounceFn } from '@vueuse/core'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import type { Account, AdminUser } from '@/types'
import type { AccountVisibleUser } from '@/api/admin/accounts'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import BaseDialog from '@/components/common/BaseDialog.vue'
import SearchInput from '@/components/common/SearchInput.vue'
import Pagination from '@/components/common/Pagination.vue'
import Icon from '@/components/icons/Icon.vue'

const props = defineProps<{ show: boolean; account: Account | null }>()
const emit = defineEmits<{ close: []; saved: [count: number] }>()
const { t } = useI18n()
const appStore = useAppStore()

const candidates = ref<AdminUser[]>([])
const selectedIds = ref(new Set<number>())
const selectedUsers = ref<AccountVisibleUser[]>([])
const search = ref('')
const page = ref(1)
const pageSize = 20
const total = ref(0)
const loading = ref(false)
const loadingInitial = ref(false)
const saving = ref(false)

const rememberUser = (user: Pick<AccountVisibleUser, 'id' | 'email' | 'username' | 'role' | 'status'>) => {
  const existing = selectedUsers.value.findIndex((item) => item.id === user.id)
  const normalized: AccountVisibleUser = {
    id: user.id,
    email: user.email,
    username: user.username || '',
    role: user.role,
    status: user.status
  }
  if (existing >= 0) selectedUsers.value[existing] = normalized
  else selectedUsers.value.push(normalized)
}

const loadCandidates = async () => {
  loading.value = true
  try {
    const result = await adminAPI.users.list(page.value, pageSize, {
      search: search.value.trim() || undefined,
      sort_by: 'email',
      sort_order: 'asc'
    })
    candidates.value = result.items
    total.value = result.total
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t('common.error')))
  } finally {
    loading.value = false
  }
}

const load = async () => {
  if (!props.account) return
  loadingInitial.value = true
  search.value = ''
  page.value = 1
  try {
    const [visibleUsers] = await Promise.all([
      adminAPI.accounts.listVisibleUsers(props.account.id),
      loadCandidates()
    ])
    selectedUsers.value = [...visibleUsers]
    selectedIds.value = new Set(visibleUsers.map((user) => user.id))
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t('common.error')))
  } finally {
    loadingInitial.value = false
  }
}

const toggleUser = (user: AdminUser) => {
  const next = new Set(selectedIds.value)
  if (next.has(user.id)) {
    next.delete(user.id)
    selectedUsers.value = selectedUsers.value.filter((item) => item.id !== user.id)
  } else {
    next.add(user.id)
    rememberUser(user)
  }
  selectedIds.value = next
}

const removeUser = (userId: number) => {
  const next = new Set(selectedIds.value)
  next.delete(userId)
  selectedIds.value = next
  selectedUsers.value = selectedUsers.value.filter((item) => item.id !== userId)
}

const save = async () => {
  if (!props.account) return
  saving.value = true
  try {
    await adminAPI.accounts.updateVisibleUsers(props.account.id, [...selectedIds.value])
    appStore.showSuccess(t('admin.accounts.visibleUsers.saved'))
    emit('saved', selectedIds.value.size)
    emit('close')
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t('common.error')))
  } finally {
    saving.value = false
  }
}

const handlePageChange = (value: number) => {
  page.value = value
  loadCandidates()
}

const debouncedSearch = useDebounceFn(() => {
  page.value = 1
  loadCandidates()
}, 300)

watch(search, debouncedSearch)
watch(() => props.show, (show) => {
  if (show) load()
})
</script>
