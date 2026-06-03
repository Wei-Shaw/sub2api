<template>
  <BaseDialog
    :show="show"
    :title="t('admin.users.usageOverride.title')"
    width="wide"
    @close="$emit('close')"
  >
    <form v-if="user" id="usage-override-form" class="space-y-5" @submit.prevent="handleSubmit">
      <div class="flex items-center gap-3 rounded-lg bg-gray-50 p-4 dark:bg-dark-700">
        <div class="flex h-10 w-10 items-center justify-center rounded-full bg-primary-100 dark:bg-primary-900/30">
          <span class="text-lg font-medium text-primary-700 dark:text-primary-300">
            {{ user.email.charAt(0).toUpperCase() }}
          </span>
        </div>
        <div class="min-w-0 flex-1">
          <p class="truncate font-medium text-gray-900 dark:text-white">{{ user.email }}</p>
          <p class="text-sm text-gray-500 dark:text-gray-400">
            {{ t('admin.users.usageOverride.currentBalance') }}: ${{ formatMoney(user.balance) }}
          </p>
        </div>
      </div>

      <div v-if="loading" class="py-8 text-center text-gray-500 dark:text-gray-400">
        {{ t('common.loading') }}
      </div>

      <div v-else class="space-y-5">
        <div class="grid gap-4 md:grid-cols-2">
          <div class="rounded-lg border border-gray-200 p-4 dark:border-dark-700">
            <h4 class="mb-3 text-sm font-semibold text-gray-900 dark:text-white">
              {{ t('admin.users.usageOverride.usageSection') }}
            </h4>
            <div class="grid gap-3 sm:grid-cols-2">
              <label class="block">
                <span class="input-label">{{ t('admin.users.usageOverride.todayActualCost') }}</span>
                <input v-model="form.today_actual_cost" type="number" min="0" step="0.000001" class="input" :placeholder="placeholder(currentUsage?.today_actual_cost)" />
              </label>
              <label class="block">
                <span class="input-label">{{ t('admin.users.usageOverride.totalActualCost') }}</span>
                <input v-model="form.total_actual_cost" type="number" min="0" step="0.000001" class="input" :placeholder="placeholder(currentUsage?.total_actual_cost)" />
              </label>
              <label class="block">
                <span class="input-label">{{ t('admin.users.usageOverride.todayRequests') }}</span>
                <input v-model="form.today_requests" type="number" min="0" step="1" class="input" :placeholder="placeholder(currentUsage?.today_requests)" />
              </label>
              <label class="block">
                <span class="input-label">{{ t('admin.users.usageOverride.todayTokens') }}</span>
                <input v-model="form.today_tokens" type="number" min="0" step="1" class="input" :placeholder="placeholder(currentUsage?.today_tokens)" />
              </label>
              <label class="block">
                <span class="input-label">{{ t('admin.users.usageOverride.totalTokens') }}</span>
                <input v-model="form.total_tokens" type="number" min="0" step="1" class="input" :placeholder="placeholder(currentUsage?.total_tokens)" />
              </label>
            </div>
            <p class="mt-3 text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.users.usageOverride.blankHint') }}
            </p>
          </div>

          <div class="rounded-lg border border-gray-200 p-4 dark:border-dark-700">
            <h4 class="mb-3 text-sm font-semibold text-gray-900 dark:text-white">
              {{ t('admin.users.usageOverride.balanceSection') }}
            </h4>
            <label class="block">
              <span class="input-label">{{ t('admin.users.usageOverride.setBalance') }}</span>
              <div class="relative">
                <span class="absolute left-3 top-1/2 -translate-y-1/2 text-sm font-medium text-gray-500">$</span>
                <input v-model="form.balance" type="number" min="0" step="0.000001" class="input pl-8" :placeholder="formatMoney(user.balance)" />
              </div>
            </label>
            <p class="mt-3 text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.users.usageOverride.balanceHint') }}
            </p>

            <label class="mt-4 block">
              <span class="input-label">{{ t('admin.users.notes') }}</span>
              <textarea v-model="form.notes" rows="4" class="input" :placeholder="t('admin.users.usageOverride.notesPlaceholder')"></textarea>
            </label>
          </div>
        </div>
      </div>
    </form>

    <template #footer>
      <div class="flex flex-wrap justify-end gap-3">
        <button type="button" class="btn btn-secondary" @click="$emit('close')">
          {{ t('common.cancel') }}
        </button>
        <button type="button" class="btn btn-secondary" :disabled="loading || submitting || clearing" @click="handleClear">
          {{ clearing ? t('common.saving') : t('admin.users.usageOverride.clear') }}
        </button>
        <button type="submit" form="usage-override-form" class="btn btn-primary" :disabled="loading || submitting || clearing">
          {{ submitting ? t('common.saving') : t('admin.users.usageOverride.save') }}
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { adminAPI } from '@/api/admin'
import type { AdminUser } from '@/types'
import type { BatchUserUsageStats } from '@/api/admin/dashboard'
import BaseDialog from '@/components/common/BaseDialog.vue'

const props = defineProps<{
  show: boolean
  user: AdminUser | null
  currentUsage?: BatchUserUsageStats | null
}>()

const emit = defineEmits(['close', 'success'])

const { t } = useI18n()
const appStore = useAppStore()

const loading = ref(false)
const submitting = ref(false)
const clearing = ref(false)

const form = reactive({
  today_requests: '',
  today_tokens: '',
  today_actual_cost: '',
  total_tokens: '',
  total_actual_cost: '',
  balance: '',
  notes: '',
})

function resetForm() {
  form.today_requests = ''
  form.today_tokens = ''
  form.today_actual_cost = ''
  form.total_tokens = ''
  form.total_actual_cost = ''
  form.balance = ''
  form.notes = ''
}

watch(
  () => props.show,
  (show) => {
    if (show) load()
  }
)

async function load() {
  resetForm()
  if (!props.user) return
  loading.value = true
  try {
    const override = await adminAPI.users.getUserUsageOverride(props.user.id)
    form.today_requests = toField(override.today_requests)
    form.today_tokens = toField(override.today_tokens)
    form.today_actual_cost = toField(override.today_actual_cost)
    form.total_tokens = toField(override.total_tokens)
    form.total_actual_cost = toField(override.total_actual_cost)
    form.notes = override.notes || ''
  } catch (e: any) {
    appStore.showError(e?.response?.data?.message || t('admin.users.usageOverride.loadFailed'))
  } finally {
    loading.value = false
  }
}

function toField(value: number | null | undefined): string {
  return value === null || value === undefined ? '' : String(value)
}

function placeholder(value: number | null | undefined): string {
  return value === null || value === undefined ? t('admin.users.usageOverride.realValue') : String(value)
}

function parseOptionalNumber(value: string, integer = false): number | null {
  const trimmed = value.trim()
  if (!trimmed) return null
  const n = Number(trimmed)
  if (!Number.isFinite(n) || n < 0) {
    throw new Error(t('admin.users.usageOverride.invalidNumber'))
  }
  return integer ? Math.trunc(n) : n
}

function formatMoney(value: number | null | undefined): string {
  const n = Number(value || 0)
  if (!Number.isFinite(n)) return '0.00'
  return n.toFixed(6).replace(/\.?0+$/, '') || '0'
}

async function handleSubmit() {
  if (!props.user) return
  submitting.value = true
  try {
    const payload = {
      today_requests: parseOptionalNumber(form.today_requests, true),
      today_tokens: parseOptionalNumber(form.today_tokens, true),
      today_actual_cost: parseOptionalNumber(form.today_actual_cost),
      total_tokens: parseOptionalNumber(form.total_tokens, true),
      total_actual_cost: parseOptionalNumber(form.total_actual_cost),
      notes: form.notes.trim() || null,
    }
    await adminAPI.users.updateUserUsageOverride(props.user.id, payload)

    const nextBalance = parseOptionalNumber(form.balance)
    if (nextBalance !== null && Math.abs(nextBalance - props.user.balance) > 0.0000001) {
      await adminAPI.users.updateBalance(props.user.id, nextBalance, 'set', form.notes.trim())
    }

    appStore.showSuccess(t('admin.users.usageOverride.saveSuccess'))
    emit('success')
    emit('close')
  } catch (e: any) {
    appStore.showError(e?.response?.data?.message || e?.message || t('admin.users.usageOverride.saveFailed'))
  } finally {
    submitting.value = false
  }
}

async function handleClear() {
  if (!props.user) return
  clearing.value = true
  try {
    await adminAPI.users.deleteUserUsageOverride(props.user.id)
    resetForm()
    appStore.showSuccess(t('admin.users.usageOverride.clearSuccess'))
    emit('success')
  } catch (e: any) {
    appStore.showError(e?.response?.data?.message || t('admin.users.usageOverride.clearFailed'))
  } finally {
    clearing.value = false
  }
}
</script>
