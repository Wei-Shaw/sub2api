<template>
  <BaseDialog
    :show="show"
    :title="t('admin.users.createUser')"
    width="normal"
    @close="$emit('close')"
  >
    <form id="create-user-form" @submit.prevent="submit" class="space-y-5">
      <div>
        <label class="input-label">{{ t('admin.users.email') REDACTEDREDACTED</label>
        <input v-model="form.email" type="email" required class="input" :placeholder="t('admin.users.enterEmail')" />
      </div>
      <div>
        <label class="input-label">{{ t('admin.users.password') REDACTEDREDACTED</label>
        <div class="flex gap-2">
          <div class="relative flex-1">
            <input v-model="form.password" type="text" required class="input pr-10" :placeholder="t('admin.users.enterPassword')" />
          </div>
          <button type="button" @click="generateRandomPassword" class="btn btn-secondary px-3">
            <Icon name="refresh" size="md" />
          </button>
        </div>
      </div>
      <div>
        <label class="input-label">{{ t('admin.users.username') REDACTEDREDACTED</label>
        <input v-model="form.username" type="text" class="input" :placeholder="t('admin.users.enterUsername')" />
      </div>
      <div>
        <label class="input-label">{{ t('admin.users.form.roleLabel') REDACTEDREDACTED</label>
        <select v-model="form.role" class="input">
          <option value="user">{{ t('admin.users.roles.user') REDACTEDREDACTED</option>
          <option value="admin">{{ t('admin.users.roles.admin') REDACTEDREDACTED</option>
        </select>
      </div>
      <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
        <div>
          <label class="input-label">{{ t('admin.users.columns.balance') REDACTEDREDACTED</label>
          <input v-model="form.balance" type="number" step="any" class="input" />
        </div>
        <div>
          <label class="input-label">{{ t('admin.users.columns.concurrency') REDACTEDREDACTED</label>
          <input v-model.number="form.concurrency" type="number" class="input" />
        </div>
      </div>
      <div>
        <label class="input-label">{{ t('admin.users.form.rpmLimit') REDACTEDREDACTED</label>
        <input
          v-model.number="form.rpm_limit"
          type="number"
          min="0"
          step="1"
          class="input"
          :placeholder="t('admin.users.form.rpmLimitPlaceholder')"
        />
        <p class="input-hint">{{ t('admin.users.form.rpmLimitHint') REDACTEDREDACTED</p>
      </div>
    </form>
    <template #footer>
      <div class="flex justify-end gap-3">
        <button @click="$emit('close')" type="button" class="btn btn-secondary">{{ t('common.cancel') REDACTEDREDACTED</button>
        <button type="submit" form="create-user-form" :disabled="loading" class="btn btn-primary">
          {{ loading ? t('admin.users.creating') : t('common.create') REDACTEDREDACTED
        </button>
      </div>
    </template>
  </BaseDialog>

  <!-- 创建管理员账号时后端要求 step-up 2FA，弹出 TOTP 验证后自动重试 -->
  <TotpStepUpDialog :controller="stepUp" />
</template>

<script setup lang="ts">
import { reactive, ref, watch REDACTED from 'vue'
import { useI18n REDACTED from 'vue-i18n'; import { adminAPI REDACTED from '@/api/admin'
import { useAppStore REDACTED from '@/stores/app'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import { useStepUp, isStepUpBlocked, isStepUpCancelled, stepUpBlockReason REDACTED from '@/composables/useStepUp'
import TotpStepUpDialog from '@/components/auth/TotpStepUpDialog.vue'

const props = defineProps<{ show: boolean REDACTED>()
const emit = defineEmits(['close', 'success']); const { t REDACTED = useI18n()
const appStore = useAppStore()

const form = reactive({ email: '', password: '', username: '', notes: '', role: 'user' as 'user' | 'admin', balance: '', concurrency: 1, rpm_limit: 0 REDACTED)

const stepUp = useStepUp()
const loading = ref(false)

const submit = async () => {
  if (loading.value) return
  loading.value = true
  try {
    const { balance: rawBalance, ...rest REDACTED = { ...form REDACTED
    const balance = String(rawBalance).trim()
    const payload: typeof rest & { balance?: number REDACTED = { ...rest REDACTED
    if (balance !== '') {
      payload.balance = Number(balance)
    REDACTED
    // 创建管理员属敏感操作：后端返回 STEP_UP_REQUIRED 时弹 TOTP 验证并重试
    await stepUp.run(() => adminAPI.users.create(payload))
    appStore.showSuccess(t('admin.users.userCreated'))
    emit('success'); emit('close')
  REDACTED catch (e: any) {
    if (isStepUpCancelled(e)) {
      // 用户主动取消二次验证：静默返回，表单保持打开。
    REDACTED else if (isStepUpBlocked(e)) {
      appStore.showError(
        stepUpBlockReason(e) === 'STEP_UP_ADMIN_API_KEY_FORBIDDEN'
          ? t('stepUp.adminApiKeyForbidden')
          : t('stepUp.notEnabled')
      )
    REDACTED else {
      appStore.showError(e?.message || t('admin.users.failedToCreate'))
    REDACTED
  REDACTED finally { loading.value = false REDACTED
REDACTED

watch(() => props.show, (v) => { if(v) Object.assign(form, { email: '', password: '', username: '', notes: '', role: 'user', balance: '', concurrency: 1, rpm_limit: 0 REDACTED) REDACTED)

const generateRandomPassword = () => {
  const chars = 'ABCDEFGHJKLMNPQRSTUVWXYZabcdefghjkmnpqrstuvwxyz23456789!@#$%^&*'
  let p = ''; for (let i = 0; i < 16; i++) p += chars.charAt(Math.floor(Math.random() * chars.length))
  form.password = p
REDACTED
</script>
