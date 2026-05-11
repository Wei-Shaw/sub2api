<template>
  <AppLayout>
    <div class="space-y-4">
      <div class="card p-4">
        <div class="flex flex-wrap items-center gap-3">
          <div class="w-full sm:w-44">
            <Select v-model="filters.status" :options="statusOptions" @change="loadWithdrawals" />
          </div>
          <div class="flex items-center justify-end gap-2 sm:ml-auto">
            <button class="btn btn-secondary" :disabled="loading" @click="loadWithdrawals">
              <Icon name="refresh" size="sm" :class="loading ? 'animate-spin' : ''" />
            </button>
          </div>
        </div>
      </div>

      <div class="card overflow-hidden">
        <div v-if="loading" class="flex items-center justify-center py-12">
          <LoadingSpinner />
        </div>
        <div v-else-if="items.length === 0" class="px-6 py-12 text-center text-sm text-gray-500 dark:text-dark-400">
          暂无提现记录。
        </div>
        <div v-else class="overflow-x-auto">
          <table class="w-full min-w-[960px] text-left text-sm">
            <thead>
              <tr class="border-b border-gray-200 bg-gray-50 text-gray-500 dark:border-dark-700 dark:bg-dark-900 dark:text-dark-400">
                <th class="px-4 py-3 font-medium">推广员</th>
                <th class="px-4 py-3 font-medium">申请金额</th>
                <th class="px-4 py-3 font-medium">手续费</th>
                <th class="px-4 py-3 font-medium">实际打款金额</th>
                <th class="px-4 py-3 font-medium">收款方式</th>
                <th class="px-4 py-3 font-medium">状态</th>
                <th class="px-4 py-3 font-medium">申请时间</th>
                <th class="px-4 py-3 font-medium">处理时限</th>
                <th class="px-4 py-3 font-medium">操作</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="item in items" :key="item.id" class="border-b border-gray-100 last:border-b-0 dark:border-dark-800">
                <td class="px-4 py-3">
                  <div class="font-medium text-gray-900 dark:text-white">{{ item.affiliate_email || `#${item.affiliate_user_id}` }}</div>
                  <div class="text-xs text-gray-500 dark:text-dark-400">{{ item.invite_code || '-' }}</div>
                </td>
                <td class="px-4 py-3 text-gray-900 dark:text-white">{{ formatMoney(item.amount) }}</td>
                <td class="px-4 py-3 text-gray-700 dark:text-gray-300">{{ formatMoney(item.fee_amount) }}</td>
                <td class="px-4 py-3 font-medium text-gray-900 dark:text-white">{{ formatMoney(item.net_amount) }}</td>
                <td class="px-4 py-3 text-gray-700 dark:text-gray-300">{{ accountTypeLabel(item) }}</td>
                <td class="px-4 py-3">
                  <span :class="statusClass(item.status)" class="rounded-full px-2.5 py-1 text-xs font-medium">{{ statusLabel(item.status) }}</span>
                </td>
                <td class="px-4 py-3 text-gray-700 dark:text-gray-300">{{ formatDateTime(item.submitted_at) }}</td>
                <td class="px-4 py-3 text-gray-700 dark:text-gray-300">{{ formatDateTime(item.payout_deadline_at) || '-' }}</td>
                <td class="px-4 py-3">
                  <div class="flex flex-wrap items-center gap-2">
                    <button class="btn btn-secondary btn-sm" @click="openDetail(item)">详情</button>
                    <button v-if="item.status === 'pending'" class="btn btn-secondary btn-sm" @click="openActionDialog('approve', item)">审核通过</button>
                    <button v-if="item.status === 'pending'" class="btn btn-secondary btn-sm" @click="openActionDialog('reject', item)">拒绝</button>
                    <button v-if="item.status === 'approved'" class="btn btn-secondary btn-sm" @click="openActionDialog('pay', item)">标记已打款</button>
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

      <BaseDialog :show="dialog.visible" :title="dialogTitle" @close="closeDialog">
        <div class="space-y-4">
          <div class="text-sm text-gray-600 dark:text-dark-300">{{ dialog.target?.affiliate_email || '-' }}</div>
          <div>
            <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">处理备注</label>
            <textarea v-model.trim="dialog.adminNote" rows="4" class="input min-h-[110px]" />
          </div>
          <div v-if="dialog.mode === 'reject'">
            <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">拒绝原因</label>
            <textarea v-model.trim="dialog.rejectReason" rows="4" class="input min-h-[110px]" />
          </div>
          <template v-if="dialog.mode === 'pay'">
            <div>
              <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">交易单号 / 链上哈希</label>
              <input v-model.trim="dialog.paymentTxnNo" type="text" class="input" />
            </div>
            <div>
              <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">打款凭证（可选）</label>
              <input type="file" accept="image/*" class="input" @change="handleProofFileChange" />
            </div>
            <div v-if="dialog.paymentProofURL" class="rounded-lg border border-gray-200 p-4 dark:border-dark-700">
              <div class="mb-2 text-sm font-medium text-gray-700 dark:text-gray-300">打款凭证预览</div>
              <button
                type="button"
                class="block rounded-lg focus:outline-none focus:ring-2 focus:ring-primary-500"
                @click="openImagePreview(dialog.paymentProofURL, '打款凭证')"
              >
                <img :src="dialog.paymentProofURL" alt="" class="max-h-56 rounded-lg border border-gray-200 dark:border-dark-700" />
              </button>
              <div class="mt-2 text-xs text-gray-500 dark:text-dark-400">点击图片可放大预览</div>
            </div>
          </template>
        </div>
        <template #footer>
          <button class="btn btn-secondary" @click="closeDialog">取消</button>
          <button class="btn btn-primary" :disabled="submitting" @click="submitDialog">{{ submitting ? '提交中...' : '确认' }}</button>
        </template>
      </BaseDialog>

      <BaseDialog :show="detailVisible" title="提现详情" width="wide" @close="detailVisible = false">
        <div v-if="detailTarget" class="grid gap-4 md:grid-cols-2">
          <div>
            <div class="text-sm text-gray-500 dark:text-dark-400">推广员</div>
            <div class="mt-1 text-sm text-gray-900 dark:text-white">{{ detailTarget.affiliate_email || `#${detailTarget.affiliate_user_id}` }}</div>
          </div>
          <div>
            <div class="text-sm text-gray-500 dark:text-dark-400">推广码</div>
            <div class="mt-1 font-mono text-sm text-gray-900 dark:text-white">{{ detailTarget.invite_code || '-' }}</div>
          </div>
          <div>
            <div class="text-sm text-gray-500 dark:text-dark-400">申请金额</div>
            <div class="mt-1 text-sm text-gray-900 dark:text-white">{{ formatMoney(detailTarget.amount) }}</div>
          </div>
          <div>
            <div class="text-sm text-gray-500 dark:text-dark-400">实际打款金额</div>
            <div class="mt-1 text-sm text-gray-900 dark:text-white">{{ formatMoney(detailTarget.net_amount) }}</div>
          </div>
          <div>
            <div class="text-sm text-gray-500 dark:text-dark-400">收款方式</div>
            <div class="mt-1 text-sm text-gray-900 dark:text-white">{{ accountTypeLabel(detailTarget) }}</div>
          </div>
          <div v-if="detailTarget.account_name">
            <div class="text-sm text-gray-500 dark:text-dark-400">收款人姓名</div>
            <div class="mt-1 text-sm text-gray-900 dark:text-white">{{ detailTarget.account_name }}</div>
          </div>
          <div class="md:col-span-2">
            <div class="text-sm text-gray-500 dark:text-dark-400">收款账号</div>
            <div class="mt-1 break-all text-sm text-gray-900 dark:text-white">{{ detailTarget.account_no || '-' }}</div>
          </div>
          <div class="md:col-span-2" v-if="detailTarget.applicant_note">
            <div class="text-sm text-gray-500 dark:text-dark-400">申请备注</div>
            <div class="mt-1 whitespace-pre-wrap text-sm text-gray-900 dark:text-white">{{ detailTarget.applicant_note }}</div>
          </div>
          <div class="md:col-span-2" v-if="detailTarget.admin_note">
            <div class="text-sm text-gray-500 dark:text-dark-400">处理备注</div>
            <div class="mt-1 whitespace-pre-wrap text-sm text-gray-900 dark:text-white">{{ detailTarget.admin_note }}</div>
          </div>
          <div class="md:col-span-2" v-if="detailTarget.reject_reason">
            <div class="text-sm text-gray-500 dark:text-dark-400">拒绝原因</div>
            <div class="mt-1 whitespace-pre-wrap text-sm text-red-600 dark:text-red-400">{{ detailTarget.reject_reason }}</div>
          </div>
          <div class="md:col-span-2" v-if="detailTarget.qr_image_url">
            <div class="mb-2 text-sm text-gray-500 dark:text-dark-400">收款二维码</div>
            <button
              type="button"
              class="block rounded-lg focus:outline-none focus:ring-2 focus:ring-primary-500"
              @click="openImagePreview(detailTarget.qr_image_url, '收款二维码')"
            >
              <img :src="detailTarget.qr_image_url" alt="" class="max-h-72 rounded-lg border border-gray-200 dark:border-dark-700" />
            </button>
            <div class="mt-2 text-xs text-gray-500 dark:text-dark-400">点击图片可放大预览</div>
          </div>
          <div class="md:col-span-2" v-if="detailTarget.payment_proof_url">
            <div class="mb-2 text-sm text-gray-500 dark:text-dark-400">打款凭证</div>
            <button
              type="button"
              class="block rounded-lg focus:outline-none focus:ring-2 focus:ring-primary-500"
              @click="openImagePreview(detailTarget.payment_proof_url, '打款凭证')"
            >
              <img :src="detailTarget.payment_proof_url" alt="" class="max-h-72 rounded-lg border border-gray-200 dark:border-dark-700" />
            </button>
            <div class="mt-2 text-xs text-gray-500 dark:text-dark-400">点击图片可放大预览</div>
          </div>
        </div>
      </BaseDialog>
      <BaseDialog :show="imagePreview.visible" :title="imagePreview.title" width="full" @close="closeImagePreview">
        <div class="flex justify-center">
          <img
            v-if="imagePreview.url"
            :src="imagePreview.url"
            :alt="imagePreview.title"
            class="max-h-[75vh] max-w-full rounded-lg border border-gray-200 object-contain dark:border-dark-700"
          />
        </div>
        <template #footer>
          <button class="btn btn-secondary" @click="closeImagePreview">关闭</button>
        </template>
      </BaseDialog>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import AppLayout from '@/components/layout/AppLayout.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import Pagination from '@/components/common/Pagination.vue'
import Select from '@/components/common/Select.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import adminReferralAPI from '@/api/admin/referral'
import type { CustomReferralWithdrawal } from '@/types'
import { useAdminReferralBadgeStore } from '@/stores'
import { useAppStore } from '@/stores/app'
import { formatDateTime } from '@/utils/format'
import { extractApiErrorMessage } from '@/utils/apiError'

const appStore = useAppStore()
const adminReferralBadgeStore = useAdminReferralBadgeStore()
const loading = ref(false)
const submitting = ref(false)
const items = ref<CustomReferralWithdrawal[]>([])
const detailTarget = ref<CustomReferralWithdrawal | null>(null)
const detailVisible = ref(false)
const imagePreview = reactive({
  visible: false,
  title: '',
  url: '',
})
const filters = reactive({ status: '' })
const pagination = reactive({ page: 1, page_size: 20, total: 0 })
const dialog = reactive({
  visible: false,
  mode: 'approve' as 'approve' | 'reject' | 'pay',
  target: null as CustomReferralWithdrawal | null,
  adminNote: '',
  rejectReason: '',
  paymentProofURL: '',
  paymentTxnNo: '',
})

const dialogTitle = computed(() => {
  if (dialog.mode === 'approve') return '审核通过'
  if (dialog.mode === 'reject') return '拒绝提现'
  return '标记已打款'
})

const statusOptions = computed(() => [
  { value: '', label: '全部状态' },
  { value: 'pending', label: '待审核' },
  { value: 'approved', label: '申请已通过，等待打款' },
  { value: 'paid', label: '已打款' },
  { value: 'rejected', label: '申请未通过' },
  { value: 'canceled', label: '已撤回' },
])

function formatMoney(value: number): string {
  return `￥${value.toFixed(2)}`
}

function statusLabel(value: string): string {
  if (value === 'pending') return '待审核'
  if (value === 'approved') return '申请已通过，等待打款'
  if (value === 'paid') return '已打款'
  if (value === 'rejected') return '申请未通过'
  if (value === 'canceled') return '已撤回'
  return value || '-'
}

function statusClass(value: string): string {
  if (value === 'paid') return 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300'
  if (value === 'approved') return 'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-300'
  if (value === 'rejected') return 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-300'
  if (value === 'canceled') return 'bg-gray-200 text-gray-700 dark:bg-dark-700 dark:text-dark-300'
  return 'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300'
}

function accountTypeLabel(item: CustomReferralWithdrawal): string {
  if (item.account_type === 'usdt') {
    return item.account_network ? `USDT / ${item.account_network}` : 'USDT'
  }
  if (item.account_type === 'alipay') return '支付宝'
  if (item.account_type === 'wechat') return '微信'
  return item.account_type || '-'
}

async function loadWithdrawals(): Promise<void> {
  loading.value = true
  try {
    const data = await adminReferralAPI.listWithdrawals({
      page: pagination.page,
      page_size: pagination.page_size,
      status: filters.status || undefined,
    })
    items.value = data.items || []
    pagination.total = data.total || 0
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, '加载提现记录失败'))
  } finally {
    loading.value = false
  }
}

function openActionDialog(mode: 'approve' | 'reject' | 'pay', item: CustomReferralWithdrawal): void {
  dialog.visible = true
  dialog.mode = mode
  dialog.target = item
  dialog.adminNote = item.admin_note || ''
  dialog.rejectReason = ''
  dialog.paymentProofURL = ''
  dialog.paymentTxnNo = ''
}

function closeDialog(): void {
  dialog.visible = false
  dialog.target = null
  dialog.adminNote = ''
  dialog.rejectReason = ''
  dialog.paymentProofURL = ''
  dialog.paymentTxnNo = ''
}

function openDetail(item: CustomReferralWithdrawal): void {
  detailTarget.value = item
  detailVisible.value = true
}

function openImagePreview(url: string, title: string): void {
  imagePreview.visible = true
  imagePreview.title = title
  imagePreview.url = url
}

function closeImagePreview(): void {
  imagePreview.visible = false
  imagePreview.title = ''
  imagePreview.url = ''
}

function handleProofFileChange(event: Event): void {
  const target = event.target as HTMLInputElement
  const file = target.files?.[0]
  if (!file) return
  adminReferralAPI.uploadAsset(file)
    .then((result) => {
      dialog.paymentProofURL = result.url
      appStore.showSuccess('打款凭证已上传')
    })
    .catch((error) => {
      appStore.showError(extractApiErrorMessage(error, '上传打款凭证失败'))
    })
}

async function submitDialog(): Promise<void> {
  if (submitting.value || !dialog.target) return
  submitting.value = true
  try {
    if (dialog.mode === 'approve') {
      await adminReferralAPI.approveWithdrawal(dialog.target.id, { admin_note: dialog.adminNote })
      appStore.showSuccess('提现申请已通过')
    } else if (dialog.mode === 'reject') {
      await adminReferralAPI.rejectWithdrawal(dialog.target.id, {
        admin_note: dialog.adminNote,
        reject_reason: dialog.rejectReason,
      })
      appStore.showSuccess('提现申请已拒绝')
    } else {
      await adminReferralAPI.markWithdrawalPaid(dialog.target.id, {
        admin_note: dialog.adminNote,
        payment_proof_url: dialog.paymentProofURL,
        payment_txn_no: dialog.paymentTxnNo,
      })
      appStore.showSuccess('提现已标记为已打款')
    }
    closeDialog()
    await loadWithdrawals()
    await adminReferralBadgeStore.refresh(true)
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, '处理提现失败'))
  } finally {
    submitting.value = false
  }
}

function handlePageChange(page: number): void {
  pagination.page = page
  loadWithdrawals()
}

function handlePageSizeChange(pageSize: number): void {
  pagination.page_size = pageSize
  pagination.page = 1
  loadWithdrawals()
}

onMounted(() => {
  loadWithdrawals()
})
</script>
