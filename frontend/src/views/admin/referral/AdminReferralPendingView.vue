<template>
  <AppLayout>
    <div class="space-y-4">
      <div>
        <h2 class="text-xl font-semibold text-gray-900 dark:text-white">待审核推广员</h2>
        <p class="mt-1 text-sm text-gray-500 dark:text-dark-400">当前仅展示状态为待审核的推广员记录。若尚未开放用户申请，该列表通常为空。</p>
      </div>

      <div class="card overflow-hidden">
        <div v-if="loading" class="flex items-center justify-center py-12">
          <LoadingSpinner />
        </div>
        <div v-else-if="items.length === 0" class="px-6 py-12 text-center text-sm text-gray-500 dark:text-dark-400">
          当前没有待审核推广员。
        </div>
        <div v-else class="overflow-x-auto">
          <table class="w-full min-w-[760px] text-left text-sm">
            <thead>
              <tr class="border-b border-gray-200 bg-gray-50 text-gray-500 dark:border-dark-700 dark:bg-dark-900 dark:text-dark-400">
                <th class="px-4 py-3 font-medium">账号</th>
                <th class="px-4 py-3 font-medium">推广码</th>
                <th class="px-4 py-3 font-medium">来源</th>
                <th class="px-4 py-3 font-medium">状态</th>
                <th class="px-4 py-3 font-medium">操作</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="item in items" :key="item.id" class="border-b border-gray-100 last:border-b-0 dark:border-dark-800">
                <td class="px-4 py-3">
                  <div class="font-medium text-gray-900 dark:text-white">{{ item.email || '-' }}</div>
                  <div class="text-xs text-gray-500 dark:text-dark-400">{{ item.username || '-' }}</div>
                </td>
                <td class="px-4 py-3 font-mono text-gray-700 dark:text-gray-300">{{ item.invite_code }}</td>
                <td class="px-4 py-3 text-gray-700 dark:text-gray-300">{{ item.source_type || '-' }}</td>
                <td class="px-4 py-3"><span class="rounded-full bg-amber-100 px-2.5 py-1 text-xs font-medium text-amber-700 dark:bg-amber-900/30 dark:text-amber-300">待审核</span></td>
                <td class="px-4 py-3">
                  <div class="flex flex-wrap items-center gap-2">
                    <button class="btn btn-secondary btn-sm" @click="openApproveDialog(item)">批准</button>
                    <button class="btn btn-secondary btn-sm" @click="reject(item)">驳回</button>
                  </div>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
        <Pagination
          v-if="pagination.total > 0"
          :page="pagination.page"
          :page-size="pagination.page_size"
          :total="pagination.total"
          @update:page="handlePageChange"
          @update:pageSize="handlePageSizeChange"
        />
      </div>

      <BaseDialog :show="approveDialog.visible" title="批准推广员" @close="closeApproveDialog">
        <div class="space-y-4 text-sm">
          <div class="text-gray-600 dark:text-dark-300">
            {{ approveDialog.target?.email || approveDialog.target?.username || '-' }}
          </div>
          <div>
            <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">专属返佣比例</label>
            <input v-model.number="approveDialog.rateOverride" type="number" min="0" max="100" step="0.01" class="input" />
            <p class="mt-1 text-xs text-gray-500 dark:text-dark-400">留空表示使用全局返佣比例。</p>
          </div>
        </div>
        <template #footer>
          <button class="btn btn-secondary" @click="closeApproveDialog">取消</button>
          <button class="btn btn-primary" :disabled="approving" @click="approveSelected">{{ approving ? '提交中...' : '确认批准' }}</button>
        </template>
      </BaseDialog>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import AppLayout from '@/components/layout/AppLayout.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import Pagination from '@/components/common/Pagination.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import adminReferralAPI from '@/api/admin/referral'
import type { CustomAffiliate } from '@/types'
import { useAdminReferralBadgeStore } from '@/stores'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'

const appStore = useAppStore()
const adminReferralBadgeStore = useAdminReferralBadgeStore()
const loading = ref(false)
const approving = ref(false)
const items = ref<CustomAffiliate[]>([])
const pagination = reactive({ page: 1, page_size: 20, total: 0 })
const approveDialog = reactive<{
  visible: boolean
  target: CustomAffiliate | null
  rateOverride: number | string | null
}>({
  visible: false,
  target: null,
  rateOverride: null,
})

async function loadItems(): Promise<void> {
  loading.value = true
  try {
    const data = await adminReferralAPI.listAffiliates({
      page: pagination.page,
      page_size: pagination.page_size,
      status: 'pending',
    })
    items.value = data.items || []
    pagination.total = data.total || 0
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, '加载待审核推广员失败'))
  } finally {
    loading.value = false
  }
}

function normalizeApproveRate(): number | null {
  const value = approveDialog.rateOverride
  if (value === null || value === undefined || value === '') {
    return null
  }
  const numeric = Number(value)
  if (Number.isNaN(numeric)) {
    return null
  }
  if (numeric < 0) return 0
  if (numeric > 100) return 100
  return numeric
}

function openApproveDialog(item: CustomAffiliate): void {
  approveDialog.visible = true
  approveDialog.target = item
  approveDialog.rateOverride = item.rate_override ?? null
}

function closeApproveDialog(): void {
  approveDialog.visible = false
  approveDialog.target = null
  approveDialog.rateOverride = null
}

async function approveSelected(): Promise<void> {
  if (approving.value || !approveDialog.target) return
  approving.value = true
  try {
    await adminReferralAPI.approveAffiliate(approveDialog.target.user_id, { rate_override: normalizeApproveRate() })
    appStore.showSuccess('推广员已批准')
    closeApproveDialog()
    await loadItems()
    await adminReferralBadgeStore.refresh(true)
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, '批准推广员失败'))
  } finally {
    approving.value = false
  }
}

async function reject(item: CustomAffiliate): Promise<void> {
  try {
    await adminReferralAPI.rejectAffiliate(item.user_id, { reason: '管理员驳回' })
    appStore.showSuccess('推广员已驳回')
    await loadItems()
    await adminReferralBadgeStore.refresh(true)
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, '驳回推广员失败'))
  }
}

function handlePageChange(page: number): void {
  pagination.page = page
  loadItems()
}

function handlePageSizeChange(pageSize: number): void {
  pagination.page_size = pageSize
  pagination.page = 1
  loadItems()
}

onMounted(() => {
  loadItems()
})
</script>
