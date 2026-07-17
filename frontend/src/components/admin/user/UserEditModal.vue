<template>
  <BaseDialog
    :show="show"
    :title="t('admin.users.editUser')"
    width="normal"
    @close="$emit('close')"
  >
    <form v-if="user" id="edit-user-form" @submit.prevent="handleUpdateUser" class="space-y-5">
      <div>
        <label class="input-label">{{ t('admin.users.email') }}</label>
        <input v-model="form.email" type="email" class="input" />
      </div>
      <div>
        <label class="input-label">{{ t('admin.users.password') }}</label>
        <div class="flex gap-2">
          <div class="relative flex-1">
            <input v-model="form.password" type="text" class="input pr-10" :placeholder="t('admin.users.enterNewPassword')" />
            <button v-if="form.password" type="button" @click="copyPassword" class="absolute right-2 top-1/2 -translate-y-1/2 rounded-lg p-1 transition-colors hover:bg-gray-100 dark:hover:bg-dark-700" :class="passwordCopied ? 'text-green-500' : 'text-gray-400'">
              <svg v-if="passwordCopied" class="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M5 13l4 4L19 7" /></svg>
              <svg v-else class="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24" stroke-width="1.5"><path stroke-linecap="round" stroke-linejoin="round" d="M15.666 3.888A2.25 2.25 0 0013.5 2.25h-3c-1.03 0-1.9.693-2.166 1.638m7.332 0c.055.194.084.4.084.612v0a.75.75 0 01-.75.75H9a.75.75 0 01-.75-.75v0c0-.212.03-.418.084-.612m7.332 0c.646.049 1.288.11 1.927.184 1.1.128 1.907 1.077 1.907 2.185V19.5a2.25 2.25 0 01-2.25 2.25H6.75A2.25 2.25 0 014.5 19.5V6.257c0-1.108.806-2.057 1.907-2.185a48.208 48.208 0 011.927-.184" /></svg>
            </button>
          </div>
          <button type="button" @click="generatePassword" class="btn btn-secondary px-3">
            <Icon name="refresh" size="md" />
          </button>
        </div>
      </div>
      <div>
        <label class="input-label">{{ t('admin.users.username') }}</label>
        <input v-model="form.username" type="text" class="input" />
      </div>
      <div>
        <label class="input-label">{{ t('admin.users.notes') }}</label>
        <textarea v-model="form.notes" rows="3" class="input"></textarea>
      </div>
      <div>
        <label class="input-label">{{ t('admin.users.form.roleLabel') }}</label>
        <Select
          v-model="form.role"
          :options="roleOptions"
        />
      </div>
      <div>
        <label class="input-label">{{ t('admin.users.columns.concurrency') }}</label>
        <input v-model.number="form.concurrency" type="number" class="input" />
      </div>
      <div>
        <label class="input-label">{{ t('admin.users.form.rpmLimit') }}</label>
        <input
          v-model.number="form.rpm_limit"
          type="number"
          min="0"
          step="1"
          class="input"
          :placeholder="t('admin.users.form.rpmLimitPlaceholder')"
        />
        <p class="input-hint">{{ t('admin.users.form.rpmLimitHint') }}</p>
      </div>
      <div v-if="form.role === 'usage_viewer'">
        <label class="input-label">
          {{ t('admin.users.allowedAccounts') }}
          <span class="font-normal text-gray-400">{{ t('common.selectedCount', { count: form.allowed_accounts.length }) }}</span>
        </label>
        <div class="rounded-lg border border-gray-200 bg-gray-50 p-2 dark:border-dark-600 dark:bg-dark-800">
          <div
            v-if="accountOptions.length > 5"
            class="mb-2 flex items-center gap-2 rounded-md bg-white px-3 py-2 dark:bg-dark-700"
          >
            <Icon name="search" size="sm" class="shrink-0 text-gray-400" />
            <input
              v-model="accountSearch"
              type="text"
              :placeholder="t('common.searchPlaceholder')"
              class="flex-1 bg-transparent text-sm text-gray-900 placeholder:text-gray-400 focus:outline-none dark:text-gray-100 dark:placeholder:text-dark-400"
            />
          </div>
          <div class="grid max-h-40 grid-cols-1 gap-1 overflow-y-auto sm:grid-cols-2">
            <label
              v-for="account in filteredAccountOptions"
              :key="account.id"
              class="flex cursor-pointer items-center gap-2 rounded px-2 py-1.5 text-sm transition-colors hover:bg-white dark:hover:bg-dark-700"
            >
              <input
                type="checkbox"
                :checked="form.allowed_accounts.includes(account.id)"
                class="h-3.5 w-3.5 shrink-0 rounded border-gray-300 text-primary-500 focus:ring-primary-500 dark:border-dark-500"
                @change="toggleAllowedAccount(account.id, ($event.target as HTMLInputElement).checked)"
              />
              <span class="min-w-0 flex-1 truncate text-gray-800 dark:text-gray-100">{{ account.name }}</span>
              <span class="shrink-0 rounded bg-gray-100 px-1.5 py-0.5 text-xs text-gray-500 dark:bg-dark-600 dark:text-dark-300">{{ account.platform }}</span>
            </label>
            <div
              v-if="filteredAccountOptions.length === 0"
              class="col-span-2 py-2 text-center text-sm text-gray-500 dark:text-gray-400"
            >
              {{ t('admin.users.noAccountsAvailable') }}
            </div>
          </div>
        </div>
        <p class="input-hint">{{ t('admin.users.allowedAccountsHint') }}</p>
      </div>
      <UserAttributeForm v-model="form.customAttributes" :user-id="user?.id" />
    </form>
    <template #footer>
      <div class="flex justify-end gap-3">
        <button @click="$emit('close')" type="button" class="btn btn-secondary">{{ t('common.cancel') }}</button>
        <button type="submit" form="edit-user-form" :disabled="submitting" class="btn btn-primary">
          {{ submitting ? t('admin.users.updating') : t('common.update') }}
        </button>
      </div>
    </template>
  </BaseDialog>

  <!-- 角色提升为管理员时后端要求 step-up 2FA，弹出 TOTP 验证后自动重试 -->
  <TotpStepUpDialog :controller="stepUp" />
</template>

<script setup lang="ts">
import { computed, ref, reactive, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { useClipboard } from '@/composables/useClipboard'
import { adminAPI } from '@/api/admin'
import type { Account, AdminUser, UserAttributeValuesMap } from '@/types'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Select from '@/components/common/Select.vue'
import UserAttributeForm from '@/components/user/UserAttributeForm.vue'
import Icon from '@/components/icons/Icon.vue'
import { useStepUp, isStepUpBlocked, isStepUpCancelled, stepUpBlockReason } from '@/composables/useStepUp'
import TotpStepUpDialog from '@/components/auth/TotpStepUpDialog.vue'

const props = defineProps<{ show: boolean, user: AdminUser | null }>()
const emit = defineEmits(['close', 'success'])
const { t } = useI18n(); const appStore = useAppStore(); const { copyToClipboard } = useClipboard()

const submitting = ref(false); const passwordCopied = ref(false)
const accountOptions = ref<Account[]>([])
const accountSearch = ref('')
type EditableUserRole = 'admin' | 'user' | 'usage_viewer'

const roleOptions = [
  { value: 'admin', label: t('admin.users.roles.admin') },
  { value: 'user', label: t('admin.users.roles.user') },
  { value: 'usage_viewer', label: t('admin.users.roles.usage_viewer') }
]
const form = reactive({
  email: '',
  password: '',
  username: '',
  notes: '',
  role: 'user' as EditableUserRole,
  concurrency: 1,
  rpm_limit: 0,
  allowed_accounts: [] as number[],
  customAttributes: {} as UserAttributeValuesMap
})

const filteredAccountOptions = computed(() => {
  const q = accountSearch.value.trim().toLowerCase()
  if (!q) return accountOptions.value
  return accountOptions.value.filter((account) => {
    return account.name.toLowerCase().includes(q) || account.platform.toLowerCase().includes(q)
  })
})

watch(() => props.user, (u) => {
  if (u) {
    Object.assign(form, {
      email: u.email,
      password: '',
      username: u.username || '',
      notes: u.notes || '',
      role: u.role,
      concurrency: u.concurrency,
      rpm_limit: u.rpm_limit ?? 0,
      allowed_accounts: [...(u.allowed_accounts || [])],
      customAttributes: {}
    })
    passwordCopied.value = false
    accountSearch.value = ''
  }
}, { immediate: true })

watch(() => props.show, (show) => {
  if (show) void loadAccountOptions()
})

const loadAccountOptions = async () => {
  try {
    const res = await adminAPI.accounts.list(1, 10000, { sort_by: 'name', sort_order: 'asc' })
    accountOptions.value = res.items || []
  } catch (e: any) {
    appStore.showError(e.response?.data?.detail || t('admin.users.failedToLoadAccounts'))
  }
}

const toggleAllowedAccount = (accountId: number, checked: boolean) => {
  if (checked) {
    if (!form.allowed_accounts.includes(accountId)) form.allowed_accounts.push(accountId)
    return
  }
  form.allowed_accounts = form.allowed_accounts.filter((id) => id !== accountId)
}

const generatePassword = () => {
  const chars = 'ABCDEFGHJKLMNPQRSTUVWXYZabcdefghjkmnpqrstuvwxyz23456789!@#$%^&*'
  let p = ''; for (let i = 0; i < 16; i++) p += chars.charAt(Math.floor(Math.random() * chars.length))
  form.password = p
}
const copyPassword = async () => {
  if (form.password && await copyToClipboard(form.password, t('admin.users.passwordCopied'))) {
    passwordCopied.value = true; setTimeout(() => passwordCopied.value = false, 2000)
  }
}
const stepUp = useStepUp()

const handleUpdateUser = async () => {
  if (!props.user) return
  if (!form.email.trim()) {
    appStore.showError(t('admin.users.emailRequired'))
    return
  }
  if (form.concurrency < 1) {
    appStore.showError(t('admin.users.concurrencyMin'))
    return
  }
  const userId = props.user.id
  submitting.value = true
  try {
    const data: any = { email: form.email, username: form.username, notes: form.notes, role: form.role, concurrency: form.concurrency, rpm_limit: form.rpm_limit }
    data.allowed_accounts = form.role === 'usage_viewer' ? [...form.allowed_accounts] : []
    if (form.password.trim()) data.password = form.password.trim()
    // 提升为管理员属敏感操作：后端返回 STEP_UP_REQUIRED 时弹 TOTP 验证并重试
    await stepUp.run(() => adminAPI.users.update(userId, data))
    if (Object.keys(form.customAttributes).length > 0) await adminAPI.userAttributes.updateUserAttributeValues(userId, form.customAttributes)
    appStore.showSuccess(t('admin.users.userUpdated'))
    emit('success'); emit('close')
  } catch (e: any) {
    if (isStepUpCancelled(e)) {
      // 用户主动取消二次验证：静默返回，表单保持打开。
    } else if (isStepUpBlocked(e)) {
      appStore.showError(
        stepUpBlockReason(e) === 'STEP_UP_ADMIN_API_KEY_FORBIDDEN'
          ? t('stepUp.adminApiKeyForbidden')
          : t('stepUp.notEnabled')
      )
    } else {
      appStore.showError(e?.message || t('admin.users.failedToUpdate'))
    }
  } finally { submitting.value = false }
}
</script>
