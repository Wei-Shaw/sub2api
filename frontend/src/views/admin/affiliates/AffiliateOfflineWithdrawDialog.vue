<template>
  <BaseDialog
    :show="show"
    :title="t('admin.affiliates.withdraw.title')"
    width="normal"
    @close="handleClose"
  >
    <form class="space-y-4" @submit.prevent="submit">
      <div>
        <label class="input-label">{{ t('admin.affiliates.withdraw.user') }}</label>
        <div
          v-if="selectedUser"
          class="flex items-center justify-between gap-3 rounded-md border border-primary-200 bg-primary-50 px-3 py-2 dark:border-primary-700/50 dark:bg-primary-900/20"
          data-test="withdraw-selected-user"
        >
          <div class="min-w-0 truncate text-sm">
            <span class="font-mono text-gray-500 dark:text-dark-400">#{{ selectedUser.id }}</span>
            <span class="ml-2 font-medium text-gray-900 dark:text-white">{{ selectedUser.email }}</span>
            <span v-if="selectedUser.username" class="ml-1 text-xs text-gray-500 dark:text-dark-400">({{ selectedUser.username }})</span>
          </div>
          <button
            type="button"
            class="text-lg leading-none text-gray-400 hover:text-red-600 disabled:opacity-50"
            :title="t('admin.affiliates.withdraw.changeUser')"
            :disabled="submitting"
            data-test="withdraw-clear-user"
            @click="clearUser"
          >
            ×
          </button>
        </div>
        <template v-else>
          <input
            v-model="userQuery"
            type="text"
            class="input"
            :placeholder="t('admin.affiliates.withdraw.userPlaceholder')"
            data-test="withdraw-user-search"
            @input="onUserQueryInput"
          />
          <div
            v-if="userResults.length > 0"
            class="mt-1 max-h-40 overflow-y-auto rounded border border-gray-200 dark:border-dark-700"
          >
            <button
              v-for="user in userResults"
              :key="user.id"
              type="button"
              class="w-full px-3 py-1.5 text-left text-sm text-gray-900 hover:bg-gray-100 dark:text-white dark:hover:bg-dark-800"
              data-test="withdraw-user-option"
              @click="selectUser(user)"
            >
              {{ user.email }} <span class="text-xs text-gray-500 dark:text-dark-400">({{ user.username }})</span>
            </button>
          </div>
          <p v-else-if="searched" class="input-hint">{{ t('admin.affiliates.withdraw.noUserFound') }}</p>
        </template>
      </div>

      <div
        v-if="selectedUser"
        class="rounded-lg border border-gray-100 bg-gray-50 p-3 dark:border-dark-700 dark:bg-dark-800"
      >
        <div class="flex items-center justify-between text-sm">
          <span class="text-gray-500 dark:text-dark-400">{{ t('admin.affiliates.withdraw.availableQuota') }}</span>
          <span
            v-if="overviewLoading"
            class="h-4 w-4 animate-spin rounded-full border-2 border-primary-500 border-t-transparent"
          ></span>
          <span v-else class="font-semibold text-gray-900 dark:text-white" data-test="withdraw-available-quota">
            ${{ formatPreciseAmount(availableQuota) }}
          </span>
        </div>
        <p class="mt-1 text-xs text-gray-500 dark:text-dark-400">{{ t('admin.affiliates.withdraw.frozenHint') }}</p>
      </div>

      <div>
        <label class="input-label">{{ t('admin.affiliates.withdraw.amount') }}</label>
        <div class="flex gap-2">
          <input
            v-model="amount"
            type="number"
            min="0"
            step="any"
            inputmode="decimal"
            class="input"
            :disabled="!selectedUser || overviewLoading || submitting"
            data-test="withdraw-amount"
          />
          <button
            type="button"
            class="btn btn-secondary shrink-0"
            :disabled="!selectedUser || overviewLoading || submitting || availableQuota <= 0"
            data-test="withdraw-fill-all"
            @click="fillAll"
          >
            {{ t('admin.affiliates.withdraw.fillAll') }}
          </button>
        </div>
        <p v-if="amountError" class="input-error-text" data-test="withdraw-amount-error">{{ amountError }}</p>
        <p v-else class="input-hint">{{ t('admin.affiliates.withdraw.amountHint') }}</p>
      </div>

      <div class="rounded-lg border border-amber-200 bg-amber-50 p-3 text-sm text-amber-800 dark:border-amber-700/50 dark:bg-amber-900/20 dark:text-amber-300">
        {{ t('admin.affiliates.withdraw.warning') }}
      </div>
    </form>

    <template #footer>
      <div class="flex justify-end gap-3">
        <button type="button" class="btn btn-secondary" :disabled="submitting" @click="handleClose">
          {{ t('common.cancel') }}
        </button>
        <button
          type="button"
          class="btn btn-primary"
          :disabled="!canSubmit"
          data-test="withdraw-submit"
          @click="submit"
        >
          {{ submitting ? t('admin.affiliates.withdraw.submitting') : t('admin.affiliates.withdraw.submit') }}
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import { useAppStore } from '@/stores/app'
import { affiliatesAPI, type AffiliateWithdrawResult, type SimpleUser } from '@/api/admin/affiliates'
import { extractApiErrorCode, extractI18nErrorMessage } from '@/utils/apiError'

const props = defineProps<{
  show: boolean
}>()

const emit = defineEmits<{
  close: []
  success: [result: AffiliateWithdrawResult]
}>()

// 返利额度按 8 位小数记账，金额比较在同一精度上进行。
const LEDGER_SCALE = 1e8

const { t } = useI18n()
const appStore = useAppStore()

const userQuery = ref('')
const userResults = ref<SimpleUser[]>([])
const searched = ref(false)
const selectedUser = ref<SimpleUser | null>(null)
const availableQuota = ref(0)
const overviewLoading = ref(false)
const amount = ref<number | string>('')
const submitting = ref(false)

let searchTimer: ReturnType<typeof setTimeout> | null = null
let searchSeq = 0
let overviewSeq = 0

const amountText = computed(() => String(amount.value ?? '').trim())
const parsedAmount = computed(() => (amountText.value === '' ? Number.NaN : Number(amountText.value)))

const amountError = computed(() => {
  if (!selectedUser.value || overviewLoading.value || amountText.value === '') return ''
  const value = parsedAmount.value
  if (!Number.isFinite(value) || Math.round(value * LEDGER_SCALE) <= 0) {
    return t('admin.affiliates.withdraw.amountRequired')
  }
  if (Math.round(value * LEDGER_SCALE) > Math.round(availableQuota.value * LEDGER_SCALE)) {
    return t('admin.affiliates.withdraw.amountExceeds')
  }
  return ''
})

const canSubmit = computed(
  () =>
    selectedUser.value !== null &&
    !overviewLoading.value &&
    !submitting.value &&
    amountText.value !== '' &&
    amountError.value === '',
)

function formatPreciseAmount(value: number): string {
  const [intPart, fraction = ''] = Number(value || 0).toFixed(8).replace(/0+$/, '').split('.')
  return `${intPart}.${fraction.padEnd(2, '0')}`
}

function showError(error: unknown) {
  appStore.showError(extractI18nErrorMessage(error, t, 'admin.affiliates.errors', t('common.error')))
}

function clearSearchTimer() {
  if (searchTimer) {
    clearTimeout(searchTimer)
    searchTimer = null
  }
}

function onUserQueryInput() {
  clearSearchTimer()
  searched.value = false
  const query = userQuery.value.trim()
  if (!query) {
    searchSeq++
    userResults.value = []
    return
  }
  searchTimer = setTimeout(() => {
    void searchUsers(query)
  }, 300)
}

async function searchUsers(query: string) {
  const seq = ++searchSeq
  try {
    const users = await affiliatesAPI.lookupUsers(query)
    if (seq !== searchSeq) return
    userResults.value = users || []
    searched.value = true
  } catch (error) {
    if (seq !== searchSeq) return
    showError(error)
  }
}

async function loadAvailableQuota(userId: number) {
  const seq = ++overviewSeq
  overviewLoading.value = true
  try {
    const overview = await affiliatesAPI.getUserOverview(userId)
    if (seq !== overviewSeq) return
    availableQuota.value = Number(overview.available_quota || 0)
  } catch (error) {
    if (seq !== overviewSeq) return
    availableQuota.value = 0
    // 从未产生过邀请返利的用户没有返利档案，概览接口返回 USER_NOT_FOUND，可提取额度即为 0。
    if (extractApiErrorCode(error) === 'USER_NOT_FOUND') return
    selectedUser.value = null
    showError(error)
  } finally {
    if (seq === overviewSeq) overviewLoading.value = false
  }
}

function selectUser(user: SimpleUser) {
  clearSearchTimer()
  searchSeq++
  selectedUser.value = user
  userQuery.value = ''
  userResults.value = []
  searched.value = false
  availableQuota.value = 0
  amount.value = ''
  void loadAvailableQuota(user.id)
}

function clearUser() {
  overviewSeq++
  selectedUser.value = null
  availableQuota.value = 0
  overviewLoading.value = false
  amount.value = ''
}

function fillAll() {
  amount.value = Math.round(availableQuota.value * LEDGER_SCALE) / LEDGER_SCALE
}

function resetForm() {
  clearSearchTimer()
  searchSeq++
  overviewSeq++
  userQuery.value = ''
  userResults.value = []
  searched.value = false
  selectedUser.value = null
  availableQuota.value = 0
  overviewLoading.value = false
  amount.value = ''
  submitting.value = false
}

function handleClose() {
  if (submitting.value) return
  emit('close')
}

async function submit() {
  const user = selectedUser.value
  if (!user || !canSubmit.value) return
  submitting.value = true
  try {
    const result = await affiliatesAPI.withdrawUserQuota(user.id, { amount: parsedAmount.value })
    appStore.showSuccess(
      t('admin.affiliates.withdraw.success', {
        amount: `$${formatPreciseAmount(result.amount)}`,
        remaining: `$${formatPreciseAmount(result.available_quota_after)}`,
      }),
    )
    emit('success', result)
  } catch (error) {
    showError(error)
    void loadAvailableQuota(user.id)
  } finally {
    submitting.value = false
  }
}

watch(
  () => props.show,
  (show) => {
    if (show) resetForm()
  },
  { immediate: true },
)

onBeforeUnmount(clearSearchTimer)
</script>
