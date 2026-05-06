<template>
  <div :class="props.embedded ? 'space-y-4' : 'card'">
    <div
      v-if="!props.embedded"
      class="border-b border-gray-100 px-6 py-4 dark:border-dark-700"
    >
      <h2 class="text-lg font-medium text-gray-900 dark:text-white">
        {{ t('profile.editProfile') }}
      </h2>
    </div>
    <div :class="props.embedded ? '' : 'px-6 py-6'">
      <form @submit.prevent="handleUpdateProfile" class="space-y-4">
        <div v-if="props.embedded">
          <p class="text-sm font-semibold text-gray-900 dark:text-white">
            {{ t('profile.editProfile') }}
          </p>
        </div>
        <div>
          <label for="username" class="input-label">
            {{ t('profile.username') }}
          </label>
          <input
            id="username"
            v-model="username"
            type="text"
            class="input"
            :placeholder="t('profile.enterUsername')"
          />
        </div>

        <div>
          <label for="timezone" class="input-label">
            {{ t('profile.timezone') }}
          </label>
          <select
            id="timezone"
            v-model="timezone"
            class="input"
          >
            <option v-for="option in timezoneOptions" :key="option" :value="option">
              {{ option }} ({{ getTimezoneOffsetLabel(option) }})
            </option>
          </select>
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
            {{ t('profile.timezoneHelp', { timezone: serverTimezone || 'UTC' }) }}
          </p>
        </div>

        <div class="flex justify-end pt-4">
          <button type="submit" :disabled="loading" class="btn btn-primary">
            {{ loading ? t('profile.updating') : t('profile.updateProfile') }}
          </button>
        </div>
      </form>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAuthStore } from '@/stores/auth'
import { useAppStore } from '@/stores/app'
import { userAPI } from '@/api'
import { COMMON_TIMEZONE_OPTIONS, getTimezoneOffsetLabel } from '@/constants/timezone'

const props = withDefaults(defineProps<{
  initialUsername: string
  initialTimezone?: string
  serverTimezone?: string
  embedded?: boolean
}>(), {
  initialTimezone: '',
  serverTimezone: 'UTC',
  embedded: false,
})

const { t } = useI18n()
const authStore = useAuthStore()
const appStore = useAppStore()

const username = ref(props.initialUsername)
const timezone = ref(props.initialTimezone || props.serverTimezone)
const loading = ref(false)

const timezoneOptions = computed(() => {
  const options = new Set<string>(COMMON_TIMEZONE_OPTIONS)
  if (props.serverTimezone) options.add(props.serverTimezone)
  if (props.initialTimezone) options.add(props.initialTimezone)
  if (timezone.value) options.add(timezone.value)
  return Array.from(options)
})

watch(() => props.initialUsername, (val) => {
  username.value = val
})

watch(() => props.initialTimezone, (val) => {
  timezone.value = val || props.serverTimezone
})

watch(() => props.serverTimezone, (val) => {
  if (!timezone.value.trim()) {
    timezone.value = val || 'UTC'
  }
})

const handleUpdateProfile = async () => {
  if (!username.value.trim()) {
    appStore.showError(t('profile.usernameRequired'))
    return
  }

  loading.value = true
  try {
    const updatedUser = await userAPI.updateProfile({
      username: username.value,
      timezone: timezone.value.trim() || props.serverTimezone || 'UTC'
    })
    authStore.user = updatedUser
    appStore.showSuccess(t('profile.updateSuccess'))
  } catch (error: any) {
    appStore.showError(error.response?.data?.detail || t('profile.updateFailed'))
  } finally {
    loading.value = false
  }
}
</script>
