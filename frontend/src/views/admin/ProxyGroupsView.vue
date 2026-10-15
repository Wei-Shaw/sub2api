<template>
  <AppLayout>
    <div class="flex h-full min-h-0 flex-col gap-5 p-4 sm:p-6">
      <div class="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h1 class="text-xl font-semibold text-gray-900 dark:text-white">{{ t('admin.proxyGroups.title') }}</h1>
          <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('admin.proxyGroups.description') }}</p>
        </div>
        <div class="flex items-center gap-2">
          <button type="button" class="btn btn-secondary" :disabled="loading" @click="loadGroups">
            <Icon name="refresh" size="sm" :class="loading ? 'animate-spin' : ''" />
          </button>
          <button type="button" class="btn btn-primary" @click="openCreate">
            <Icon name="plus" size="sm" class="mr-1.5" />
            {{ t('admin.proxyGroups.create') }}
          </button>
        </div>
      </div>

      <div v-if="errorMessage" role="alert" class="rounded-lg border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700 dark:border-red-800/50 dark:bg-red-900/20 dark:text-red-300">
        {{ errorMessage }}
      </div>

      <div class="min-h-0 flex-1 overflow-auto rounded-xl border border-gray-200 bg-white dark:border-dark-600 dark:bg-dark-800">
        <table class="min-w-full divide-y divide-gray-200 text-sm dark:divide-dark-600">
          <thead class="bg-gray-50 dark:bg-dark-700/60">
            <tr>
              <th class="px-4 py-3 text-left font-medium text-gray-500 dark:text-gray-400">{{ t('admin.proxyGroups.name') }}</th>
              <th class="px-4 py-3 text-left font-medium text-gray-500 dark:text-gray-400">{{ t('admin.proxyGroups.proxies') }}</th>
              <th class="px-4 py-3 text-left font-medium text-gray-500 dark:text-gray-400">{{ t('admin.proxyGroups.members') }}</th>
              <th class="px-4 py-3 text-left font-medium text-gray-500 dark:text-gray-400">{{ t('admin.proxyGroups.availableMembers') }}</th>
              <th class="px-4 py-3 text-left font-medium text-gray-500 dark:text-gray-400">{{ t('admin.proxyGroups.accounts') }}</th>
              <th class="px-4 py-3 text-left font-medium text-gray-500 dark:text-gray-400">{{ t('admin.proxyGroups.status') }}</th>
              <th class="px-4 py-3 text-right font-medium text-gray-500 dark:text-gray-400">{{ t('common.actions') }}</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-gray-200 dark:divide-dark-600">
            <tr v-if="loading">
              <td colspan="7" class="px-4 py-12 text-center text-gray-500 dark:text-gray-400">{{ t('common.loading') }}</td>
            </tr>
            <tr v-else-if="groups.length === 0">
              <td colspan="7" class="px-4 py-12 text-center text-gray-500 dark:text-gray-400">
                <p>{{ t('admin.proxyGroups.noGroups') }}</p>
                <p class="mt-1 text-xs">{{ t('admin.proxyGroups.createFirst') }}</p>
              </td>
            </tr>
            <template v-else>
            <tr v-for="group in groups" :key="group.id" class="hover:bg-gray-50 dark:hover:bg-dark-700/40">
              <td class="px-4 py-3">
                <div class="font-medium text-gray-900 dark:text-white">{{ group.name }}</div>
                <div v-if="group.description" class="mt-0.5 max-w-xs truncate text-xs text-gray-500 dark:text-gray-400">{{ group.description }}</div>
              </td>
              <td class="px-4 py-3 text-gray-600 dark:text-gray-300">{{ proxyNames(group.proxy_ids) }}</td>
              <td class="px-4 py-3 text-gray-600 dark:text-gray-300">{{ group.member_count }}</td>
              <td class="px-4 py-3 text-gray-600 dark:text-gray-300">{{ group.available_member_count }}</td>
              <td class="px-4 py-3 text-gray-600 dark:text-gray-300">{{ group.account_count }}</td>
              <td class="px-4 py-3">
                <span :class="['badge', group.status === 'active' ? 'badge-success' : 'badge-gray']">
                  {{ group.status === 'active' ? t('admin.proxyGroups.active') : t('admin.proxyGroups.inactive') }}
                </span>
              </td>
              <td class="px-4 py-3">
                <div class="flex justify-end gap-1">
                  <button type="button" class="btn-icon" :title="t('common.edit')" @click="openEdit(group)">
                    <Icon name="edit" size="sm" />
                  </button>
                  <button type="button" class="btn-icon text-red-500 hover:text-red-600" :title="t('common.delete')" @click="openDelete(group)">
                    <Icon name="trash" size="sm" />
                  </button>
                </div>
              </td>
            </tr>
            </template>
          </tbody>
        </table>
      </div>

      <div v-if="pagination.total > pagination.pageSize" class="flex items-center justify-between text-sm text-gray-500 dark:text-gray-400">
        <span>{{ pagination.total }}</span>
        <div class="flex gap-2">
          <button type="button" class="btn btn-secondary" :disabled="pagination.page <= 1 || loading" @click="changePage(pagination.page - 1)">‹</button>
          <span class="px-2 py-2">{{ pagination.page }}</span>
          <button type="button" class="btn btn-secondary" :disabled="pagination.page * pagination.pageSize >= pagination.total || loading" @click="changePage(pagination.page + 1)">›</button>
        </div>
      </div>
    </div>

    <BaseDialog :show="showEditor" :title="editingGroup ? t('admin.proxyGroups.edit') : t('admin.proxyGroups.create')" width="wide" @close="closeEditor">
      <form class="space-y-5" @submit.prevent="saveGroup">
        <div>
          <label class="input-label">{{ t('admin.proxyGroups.name') }}</label>
          <input v-model="form.name" class="input" :placeholder="t('admin.proxyGroups.namePlaceholder')" required />
        </div>
        <div>
          <label class="input-label">{{ t('admin.proxyGroups.descriptionLabel') }}</label>
          <textarea v-model="form.description" rows="2" class="input" :placeholder="t('admin.proxyGroups.descriptionPlaceholder')" />
        </div>
        <div>
          <label class="input-label">{{ t('admin.proxyGroups.status') }}</label>
          <select v-model="form.status" class="input">
            <option value="active">{{ t('admin.proxyGroups.active') }}</option>
            <option value="inactive">{{ t('admin.proxyGroups.inactive') }}</option>
          </select>
        </div>
        <div>
          <label class="input-label">{{ t('admin.proxyGroups.selectProxies') }}</label>
          <div class="mt-2 grid max-h-64 gap-2 overflow-y-auto rounded-lg border border-gray-200 p-3 sm:grid-cols-2 dark:border-dark-600">
            <label v-for="proxy in proxies" :key="proxy.id" class="flex cursor-pointer items-start gap-2 rounded-lg p-2 hover:bg-gray-50 dark:hover:bg-dark-700">
              <input v-model="form.proxy_ids" type="checkbox" :value="proxy.id" class="mt-0.5 rounded border-gray-300 text-primary-600 focus:ring-primary-500" />
              <span class="min-w-0">
                <span class="block truncate text-sm text-gray-900 dark:text-white">{{ proxy.name }}</span>
                <span class="block truncate text-xs text-gray-500 dark:text-gray-400">{{ proxy.protocol }}://{{ proxy.host }}:{{ proxy.port }}</span>
              </span>
            </label>
            <p v-if="proxies.length === 0" class="text-sm text-gray-500 dark:text-gray-400">{{ t('admin.proxies.noProxiesYet') }}</p>
          </div>
        </div>
        <p v-if="editorError" role="alert" class="text-sm text-red-600 dark:text-red-400">{{ editorError }}</p>
      </form>
      <template #footer>
        <div class="flex justify-end gap-3">
          <button type="button" class="btn btn-secondary" @click="closeEditor">{{ t('common.cancel') }}</button>
          <button type="button" class="btn btn-primary" :disabled="saving" @click="saveGroup">{{ saving ? t('common.saving') : t('admin.proxyGroups.save') }}</button>
        </div>
      </template>
    </BaseDialog>

    <ConfirmDialog
      :show="showDeleteDialog"
      :title="t('admin.proxyGroups.delete')"
      :message="t('admin.proxyGroups.deleteConfirm', { name: deletingGroup?.name || '' })"
      :confirm-text="t('common.delete')"
      :cancel-text="t('common.cancel')"
      :danger="true"
      @confirm="confirmDelete"
      @cancel="showDeleteDialog = false"
    />
  </AppLayout>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import type { Proxy, ProxyGroup } from '@/types'
import { useAppStore } from '@/stores/app'
import AppLayout from '@/components/layout/AppLayout.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import Icon from '@/components/icons/Icon.vue'

const { t } = useI18n()
const appStore = useAppStore()
const groups = ref<ProxyGroup[]>([])
const proxies = ref<Proxy[]>([])
const loading = ref(false)
const saving = ref(false)
const showEditor = ref(false)
const showDeleteDialog = ref(false)
const editingGroup = ref<ProxyGroup | null>(null)
const deletingGroup = ref<ProxyGroup | null>(null)
const errorMessage = ref('')
const editorError = ref('')
const pagination = reactive({ page: 1, pageSize: 20, total: 0 })
const form = reactive({
  name: '',
  description: '',
  status: 'active' as 'active' | 'inactive',
  proxy_ids: [] as number[]
})

const errorText = (error: any, fallback: string) =>
  error?.response?.data?.message || error?.response?.data?.detail || error?.message || fallback

const proxyNames = (ids: number[]) => {
  const names = ids.map((id) => proxies.value.find((proxy) => proxy.id === id)?.name || `#${id}`)
  return names.length > 0 ? names.join(', ') : '-'
}

const loadGroups = async () => {
  loading.value = true
  errorMessage.value = ''
  try {
    const [groupResult, proxyResult] = await Promise.all([adminAPI.proxyGroups.list(pagination.page, pagination.pageSize), adminAPI.proxies.getAll()])
    groups.value = groupResult.items
    pagination.total = groupResult.total
    proxies.value = proxyResult
  } catch (error) {
    errorMessage.value = errorText(error, t('admin.proxyGroups.failedToLoad'))
    appStore.showError(errorMessage.value)
  } finally {
    loading.value = false
  }
}

const changePage = async (page: number) => {
  pagination.page = page
  await loadGroups()
}

const resetForm = () => {
  form.name = ''
  form.description = ''
  form.status = 'active'
  form.proxy_ids = []
  editorError.value = ''
}

const openCreate = () => {
  editingGroup.value = null
  resetForm()
  showEditor.value = true
}

const openEdit = (group: ProxyGroup) => {
  editingGroup.value = group
  form.name = group.name
  form.description = group.description || ''
  form.status = group.status
  form.proxy_ids = [...group.proxy_ids]
  editorError.value = ''
  showEditor.value = true
}

const closeEditor = () => {
  if (saving.value) return
  showEditor.value = false
}

const saveGroup = async () => {
  const name = form.name.trim()
  if (!name) {
    editorError.value = t('admin.proxyGroups.nameRequired')
    return
  }
  if (form.proxy_ids.length === 0) {
    editorError.value = t('admin.proxyGroups.memberRequired')
    return
  }
  saving.value = true
  editorError.value = ''
  const payload = {
    name,
    description: form.description.trim() || null,
    status: form.status,
    proxy_ids: form.proxy_ids
  }
  try {
    if (editingGroup.value) {
      await adminAPI.proxyGroups.update(editingGroup.value.id, payload)
      appStore.showSuccess(t('admin.proxyGroups.updateSuccess'))
    } else {
      await adminAPI.proxyGroups.create(payload)
      appStore.showSuccess(t('admin.proxyGroups.createSuccess'))
    }
    showEditor.value = false
    await loadGroups()
  } catch (error) {
    editorError.value = errorText(error, t('admin.proxyGroups.failedToSave'))
    appStore.showError(editorError.value)
  } finally {
    saving.value = false
  }
}

const openDelete = (group: ProxyGroup) => {
  deletingGroup.value = group
  errorMessage.value = ''
  showDeleteDialog.value = true
}

const confirmDelete = async () => {
  const group = deletingGroup.value
  if (!group) return
  try {
    await adminAPI.proxyGroups.deleteGroup(group.id)
    showDeleteDialog.value = false
    appStore.showSuccess(t('admin.proxyGroups.deleteSuccess'))
    await loadGroups()
  } catch (error: any) {
    const data = error?.response?.data
    const reason = error?.reason || data?.reason || data?.error?.reason || data?.error
    const metadata = error?.metadata || data?.metadata || data?.error?.metadata
    const count = Number(metadata?.account_count || error?.account_count || data?.account_count || group.account_count || 0)
    if (reason === 'PROXY_GROUP_IN_USE' || count > 0) {
      errorMessage.value = t('admin.proxyGroups.deleteInUse', { name: group.name, count })
    } else {
      errorMessage.value = `${group.name}: ${errorText(error, t('admin.proxyGroups.failedToDelete'))}`
    }
    appStore.showError(errorMessage.value)
  }
}

onMounted(loadGroups)
</script>
