<template>
  <BaseDialog
    :show="show"
    :title="t('admin.users.createUser')"
    width="normal"
    @close="$emit('close')"
  >
    <form id="create-user-form" @submit.prevent="handleCreateUser" class="space-y-5">
      <div>
        <label class="input-label">{{ t('admin.users.email') REDACTEDREDACTED</label>
        <input
          v-model="form.email"
          type="email"
          required
          class="input"
          :placeholder="t('admin.users.enterEmail')"
        />
      </div>
      <div>
        <label class="input-label">{{ t('admin.users.password') REDACTEDREDACTED</label>
        <div class="flex gap-2">
          <div class="relative flex-1">
            <input
              v-model="form.password"
              type="text"
              required
              class="input pr-10"
              :placeholder="t('admin.users.enterPassword')"
            />
            <button
              v-if="form.password"
              type="button"
              @click="copyPassword"
              class="absolute right-2 top-1/2 -translate-y-1/2 rounded-lg p-1 transition-colors hover:bg-gray-100 dark:hover:bg-dark-700"
              :class="passwordCopied ? 'text-green-500' : 'text-gray-400'"
            >
              <svg v-if="passwordCopied" class="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24" stroke-width="2">
                <path stroke-linecap="round" stroke-linejoin="round" d="M5 13l4 4L19 7" />
              </svg>
              <svg v-else class="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24" stroke-width="1.5">
                <path stroke-linecap="round" stroke-linejoin="round" d="M15.666 3.888A2.25 2.25 0 0013.5 2.25h-3c-1.03 0-1.9.693-2.166 1.638m7.332 0c.055.194.084.4.084.612v0a.75.75 0 01-.75.75H9a.75.75 0 01-.75-.75v0c0-.212.03-.418.084-.612m7.332 0c.646.049 1.288.11 1.927.184 1.1.128 1.907 1.077 1.907 2.185V19.5a2.25 2.25 0 01-2.25 2.25H6.75A2.25 2.25 0 014.5 19.5V6.257c0-1.108.806-2.057 1.907-2.185a48.208 48.208 0 011.927-.184" />
              </svg>
            </button>
          </div>
          <button type="button" @click="generateRandomPassword" class="btn btn-secondary px-3">
            <svg class="h-5 w-5" fill="none" stroke="currentColor" viewBox="0 0 24 24" stroke-width="1.5">
              <path stroke-linecap="round" stroke-linejoin="round" d="M16.023 9.348h4.992v-.001M2.985 19.644v-4.992m0 0h4.992m-4.993 0l3.181 3.183a8.25 8.25 0 0013.803-3.7M4.031 9.865a8.25 8.25 0 0113.803-3.7l3.181 3.182m0-4.991v4.99" />
            </svg>
          </button>
        </div>
      </div>
      <div>
        <label class="input-label">{{ t('admin.users.username') REDACTEDREDACTED</label>
        <input v-model="form.username" type="text" class="input" :placeholder="t('admin.users.enterUsername')" />
      </div>
      <div>
        <label class="input-label">{{ t('admin.users.notes') REDACTEDREDACTED</label>
        <textarea v-model="form.notes" rows="3" class="input" :placeholder="t('admin.users.enterNotes')"></textarea>
      </div>
      <div class="grid grid-cols-2 gap-4">
        <div>
          <label class="input-label">{{ t('admin.users.columns.balance') REDACTEDREDACTED</label>
          <input v-model.number="form.balance" type="number" step="any" class="input" />
        </div>
        <div>
          <label class="input-label">{{ t('admin.users.columns.concurrency') REDACTEDREDACTED</label>
          <input v-model.number="form.concurrency" type="number" class="input" />
        </div>
      </div>
    </form>
    <template #footer>
      <div class="flex justify-end gap-3">
        <button @click="$emit('close')" type="button" class="btn btn-secondary">{{ t('common.cancel') REDACTEDREDACTED</button>
        <button type="submit" form="create-user-form" :disabled="submitting" class="btn btn-primary">
          {{ submitting ? t('admin.users.creating') : t('common.create') REDACTEDREDACTED
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { ref, reactive, watch REDACTED from 'vue'
import { useI18n REDACTED from 'vue-i18n'
import { useAppStore REDACTED from '@/stores/app'
import { useClipboard REDACTED from '@/composables/useClipboard'
import { adminAPI REDACTED from '@/api/admin'
import BaseDialog from '@/components/common/BaseDialog.vue'

const props = defineProps<{ show: boolean REDACTED>()
const emit = defineEmits(['close', 'success'])
const { t REDACTED = useI18n(); const appStore = useAppStore(); const { copyToClipboard REDACTED = useClipboard()

const submitting = ref(false); const passwordCopied = ref(false)
const form = reactive({ email: '', password: '', username: '', notes: '', balance: 0, concurrency: 1 REDACTED)

watch(() => props.show, (v) => { if(v) { Object.assign(form, { email: '', password: '', username: '', notes: '', balance: 0, concurrency: 1 REDACTED); passwordCopied.value = false REDACTED REDACTED)

const generateRandomPassword = () => {
  const chars = 'ABCDEFGHJKLMNPQRSTUVWXYZabcdefghjkmnpqrstuvwxyz23456789!@#$%^&*'
  let p = ''; for (let i = 0; i < 16; i++) p += chars.charAt(Math.floor(Math.random() * chars.length))
  form.password = p
REDACTED
const copyPassword = async () => {
  if (form.password && await copyToClipboard(form.password, t('admin.users.passwordCopied'))) {
    passwordCopied.value = true; setTimeout(() => passwordCopied.value = false, 2000)
  REDACTED
REDACTED
const handleCreateUser = async () => {
  submitting.value = true
  try {
    await adminAPI.users.create(form); appStore.showSuccess(t('admin.users.userCreated'))
    emit('success'); emit('close')
  REDACTED catch (e: any) {
    appStore.showError(e.response?.data?.message || e.response?.data?.detail || t('admin.users.failedToCreate'))
  REDACTED finally { submitting.value = false REDACTED
REDACTED
</script>