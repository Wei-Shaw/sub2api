<template>
  <div :class="props.embedded ? 'space-y-4' : 'card'">
    <div
      v-if="!props.embedded"
      class="border-b border-gray-100 px-6 py-4 dark:border-dark-700"
    >
      <h2 class="text-lg font-medium text-gray-900 dark:text-white">
        {{ t('profile.changePassword') REDACTEDREDACTED
      </h2>
    </div>
    <div :class="props.embedded ? '' : 'px-6 py-6'">
      <form @submit.prevent="handleChangePassword" class="space-y-4">
        <div v-if="props.embedded">
          <p class="text-sm font-semibold text-gray-900 dark:text-white">
            {{ t('profile.changePassword') REDACTEDREDACTED
          </p>
        </div>
        <div>
          <label for="old_password" class="input-label">
            {{ t('profile.currentPassword') REDACTEDREDACTED
          </label>
          <input
            id="old_password"
            v-model="form.old_password"
            type="password"
            required
            autocomplete="current-password"
            class="input"
          />
        </div>

        <div>
          <label for="new_password" class="input-label">
            {{ t('profile.newPassword') REDACTEDREDACTED
          </label>
          <input
            id="new_password"
            v-model="form.new_password"
            type="password"
            required
            autocomplete="new-password"
            class="input"
          />
          <p class="input-hint">
            {{ t('profile.passwordHint') REDACTEDREDACTED
          </p>
        </div>

        <div>
          <label for="confirm_password" class="input-label">
            {{ t('profile.confirmNewPassword') REDACTEDREDACTED
          </label>
          <input
            id="confirm_password"
            v-model="form.confirm_password"
            type="password"
            required
            autocomplete="new-password"
            class="input"
          />
        </div>

        <div class="flex justify-end pt-4">
          <button type="submit" :disabled="loading" class="btn btn-primary">
            {{ loading ? t('profile.changingPassword') : t('profile.changePasswordButton') REDACTEDREDACTED
          </button>
        </div>
      </form>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref REDACTED from 'vue'
import { useI18n REDACTED from 'vue-i18n'
import { useAppStore REDACTED from '@/stores/app'
import { userAPI REDACTED from '@/api'

const { t REDACTED = useI18n()
const appStore = useAppStore()
const props = withDefaults(defineProps<{
  embedded?: boolean
REDACTED>(), {
  embedded: false,
REDACTED)

const loading = ref(false)
const form = ref({
  old_password: '',
  new_password: '',
  confirm_password: ''
REDACTED)

const handleChangePassword = async () => {
  if (form.value.new_password !== form.value.confirm_password) {
    appStore.showError(t('profile.passwordsNotMatch'))
    return
  REDACTED

  if (form.value.new_password.length < 8) {
    appStore.showError(t('profile.passwordTooShort'))
    return
  REDACTED

  loading.value = true
  try {
    await userAPI.changePassword(form.value.old_password, form.value.new_password)
    form.value = { old_password: '', new_password: '', confirm_password: '' REDACTED
    appStore.showSuccess(t('profile.passwordChangeSuccess'))
  REDACTED catch (error: any) {
    appStore.showError(error.response?.data?.detail || t('profile.passwordChangeFailed'))
  REDACTED finally {
    loading.value = false
  REDACTED
REDACTED
</script>
