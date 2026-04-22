<template>
  <div :class="props.embedded ? 'space-y-4' : 'card'">
    <div
      v-if="!props.embedded"
      class="border-b border-gray-100 px-6 py-4 dark:border-dark-700"
    >
      <h2 class="text-lg font-medium text-gray-900 dark:text-white">
        {{ t('profile.editProfile') REDACTEDREDACTED
      </h2>
    </div>
    <div :class="props.embedded ? '' : 'px-6 py-6'">
      <form @submit.prevent="handleUpdateProfile" class="space-y-4">
        <div v-if="props.embedded">
          <p class="text-sm font-semibold text-gray-900 dark:text-white">
            {{ t('profile.editProfile') REDACTEDREDACTED
          </p>
        </div>
        <div>
          <label for="username" class="input-label">
            {{ t('profile.username') REDACTEDREDACTED
          </label>
          <input
            id="username"
            v-model="username"
            type="text"
            class="input"
            :placeholder="t('profile.enterUsername')"
          />
        </div>

        <div class="flex justify-end pt-4">
          <button type="submit" :disabled="loading" class="btn btn-primary">
            {{ loading ? t('profile.updating') : t('profile.updateProfile') REDACTEDREDACTED
          </button>
        </div>
      </form>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, watch REDACTED from 'vue'
import { useI18n REDACTED from 'vue-i18n'
import { useAuthStore REDACTED from '@/stores/auth'
import { useAppStore REDACTED from '@/stores/app'
import { userAPI REDACTED from '@/api'

const props = withDefaults(defineProps<{
  initialUsername: string
  embedded?: boolean
REDACTED>(), {
  embedded: false,
REDACTED)

const { t REDACTED = useI18n()
const authStore = useAuthStore()
const appStore = useAppStore()

const username = ref(props.initialUsername)
const loading = ref(false)

watch(() => props.initialUsername, (val) => {
  username.value = val
REDACTED)

const handleUpdateProfile = async () => {
  if (!username.value.trim()) {
    appStore.showError(t('profile.usernameRequired'))
    return
  REDACTED

  loading.value = true
  try {
    const updatedUser = await userAPI.updateProfile({
      username: username.value
    REDACTED)
    authStore.user = updatedUser
    appStore.showSuccess(t('profile.updateSuccess'))
  REDACTED catch (error: any) {
    appStore.showError(error.response?.data?.detail || t('profile.updateFailed'))
  REDACTED finally {
    loading.value = false
  REDACTED
REDACTED
</script>
