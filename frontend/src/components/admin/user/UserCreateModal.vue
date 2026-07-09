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
</template>

<script setup lang="ts">
import { reactive, watch REDACTED from 'vue'
import { useI18n REDACTED from 'vue-i18n'; import { adminAPI REDACTED from '@/api/admin'
import { useForm REDACTED from '@/composables/useForm'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Icon from '@/components/icons/Icon.vue'

const props = defineProps<{ show: boolean REDACTED>()
const emit = defineEmits(['close', 'success']); const { t REDACTED = useI18n()

const form = reactive({ email: '', password: '', username: '', notes: '', role: 'user' as 'user' | 'admin', balance: '', concurrency: 1, rpm_limit: 0 REDACTED)

const { loading, submit REDACTED = useForm({
  form,
  submitFn: async (data) => {
    const { balance: rawBalance, ...rest REDACTED = data
    const balance = String(rawBalance).trim()
    const payload: typeof rest & { balance?: number REDACTED = { ...rest REDACTED
    if (balance !== '') {
      payload.balance = Number(balance)
    REDACTED
    await adminAPI.users.create(payload)
    emit('success'); emit('close')
  REDACTED,
  successMsg: t('admin.users.userCreated')
REDACTED)

watch(() => props.show, (v) => { if(v) Object.assign(form, { email: '', password: '', username: '', notes: '', role: 'user', balance: '', concurrency: 1, rpm_limit: 0 REDACTED) REDACTED)

const generateRandomPassword = () => {
  const chars = 'ABCDEFGHJKLMNPQRSTUVWXYZabcdefghjkmnpqrstuvwxyz23456789!@#$%^&*'
  let p = ''; for (let i = 0; i < 16; i++) p += chars.charAt(Math.floor(Math.random() * chars.length))
  form.password = p
REDACTED
</script>
