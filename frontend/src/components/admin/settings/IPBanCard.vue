<template>
  <div class="card" data-testid="ip-ban-card">
    <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
      <div class="flex items-center gap-2">
        <Icon name="ban" size="md" class="text-red-500" />
        <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('admin.settings.ipBan.title') }}</h2>
      </div>
      <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('admin.settings.ipBan.description') }}</p>
    </div>
    <div class="space-y-5 p-6">
      <form class="flex flex-col gap-3 sm:flex-row sm:items-end" @submit.prevent="addBan">
        <div class="min-w-0 flex-1">
          <label for="ip-ban-address" class="input-label">{{ t('admin.settings.ipBan.address') }}</label>
          <input id="ip-ban-address" v-model="ipAddress" type="text" class="input font-mono" :placeholder="t('admin.settings.ipBan.addressPlaceholder')" :disabled="saving" />
        </div>
        <button type="submit" class="btn btn-danger sm:mb-0.5" :disabled="saving || !ipAddress.trim()">
          <Icon name="plus" size="sm" class="mr-1.5" />
          {{ saving ? t('common.processing') : t('admin.settings.ipBan.add') }}
        </button>
      </form>

      <div class="overflow-hidden rounded-lg border border-gray-200 dark:border-dark-600">
        <table class="min-w-full divide-y divide-gray-200 dark:divide-dark-600">
          <thead class="bg-gray-50 dark:bg-dark-700/50">
            <tr>
              <th class="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">{{ t('admin.settings.ipBan.address') }}</th>
              <th class="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">{{ t('admin.settings.ipBan.createdAt') }}</th>
              <th class="px-4 py-3 text-right text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">{{ t('admin.settings.ipBan.actions') }}</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-gray-100 bg-white dark:divide-dark-700 dark:bg-dark-800">
            <tr v-for="ban in bans" :key="ban.id">
              <td class="whitespace-nowrap px-4 py-3 font-mono text-sm text-gray-900 dark:text-white">{{ ban.ip_address }}</td>
              <td class="whitespace-nowrap px-4 py-3 text-sm text-gray-500 dark:text-gray-400">{{ formatDate(ban.created_at) }}</td>
              <td class="whitespace-nowrap px-4 py-3 text-right">
                <button type="button" class="btn btn-ghost btn-sm text-red-600 hover:text-red-700 dark:text-red-400" :disabled="deletingId === ban.id" @click="removeBan(ban)">
                  <Icon name="trash" size="sm" class="mr-1" />{{ t('common.delete') }}
                </button>
              </td>
            </tr>
            <tr v-if="!loading && bans.length === 0"><td colspan="3" class="px-4 py-8 text-center text-sm text-gray-500 dark:text-gray-400">{{ t('admin.settings.ipBan.empty') }}</td></tr>
            <tr v-if="loading"><td colspan="3" class="px-4 py-8 text-center text-sm text-gray-500 dark:text-gray-400">{{ t('common.loading') }}</td></tr>
          </tbody>
        </table>
        <Pagination v-if="total > 0" :page="page" :total="total" :page-size="pageSize" @update:page="changePage" @update:page-size="changePageSize" />
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api'
import { useAppStore } from '@/stores'
import { formatDateTime } from '@/utils/format'
import Icon from '@/components/icons/Icon.vue'
import Pagination from '@/components/common/Pagination.vue'
import type { IPBan } from '@/api/admin/settings'

const { t } = useI18n()
const appStore = useAppStore()
const ipAddress = ref('')
const bans = ref<IPBan[]>([])
const loading = ref(false)
const saving = ref(false)
const deletingId = ref<number | null>(null)
const page = ref(1)
const pageSize = ref(20)
const total = ref(0)

const formatDate = (value: string) => formatDateTime(value)

async function loadBans() {
  loading.value = true
  try {
    const result = await adminAPI.settings.listIPBans(page.value, pageSize.value)
    bans.value = result.items
    total.value = result.total
  } catch (error) {
    appStore.showError(t('admin.settings.ipBan.loadFailed'))
    console.error('Failed to load IP bans:', error)
  } finally {
    loading.value = false
  }
}

async function addBan() {
  const value = ipAddress.value.trim()
  if (!value || saving.value) return
  saving.value = true
  try {
    await adminAPI.settings.createIPBan(value)
    ipAddress.value = ''
    page.value = 1
    appStore.showSuccess(t('admin.settings.ipBan.added'))
    await loadBans()
  } catch (error: any) {
    appStore.showError(error.response?.data?.detail || t('admin.settings.ipBan.addFailed'))
  } finally {
    saving.value = false
  }
}

async function removeBan(ban: IPBan) {
  if (deletingId.value !== null || !confirm(t('admin.settings.ipBan.deleteConfirm', { ip: ban.ip_address }))) return
  deletingId.value = ban.id
  try {
    await adminAPI.settings.deleteIPBan(ban.id)
    if (bans.value.length === 1 && page.value > 1) page.value -= 1
    appStore.showSuccess(t('admin.settings.ipBan.deleted'))
    await loadBans()
  } catch (error) {
    appStore.showError(t('admin.settings.ipBan.deleteFailed'))
  } finally {
    deletingId.value = null
  }
}

const changePage = (value: number) => { page.value = value; loadBans() }
const changePageSize = (value: number) => { pageSize.value = value; page.value = 1; loadBans() }

onMounted(loadBans)
</script>
