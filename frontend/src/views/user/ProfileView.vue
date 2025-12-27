<template>
  <AppLayout>
    <div class="mx-auto max-w-4xl space-y-6">
      <!-- Account Stats Summary -->
      <div class="grid grid-cols-1 gap-6 sm:grid-cols-3">
        <StatCard
          :title="t('profile.accountBalance')"
          :value="formatCurrency(user?.balance || 0)"
          :icon="WalletIcon"
          icon-variant="success"
        />
        <StatCard
          :title="t('profile.concurrencyLimit')"
          :value="user?.concurrency || 0"
          :icon="BoltIcon"
          icon-variant="warning"
        />
        <StatCard
          :title="t('profile.memberSince')"
          :value="formatDate(user?.created_at || '', 'YYYY-MM')"
          :icon="CalendarIcon"
          icon-variant="primary"
        />
      </div>

      <!-- User Information -->
      <div class="card overflow-hidden">
        <div
          class="border-b border-gray-100 bg-gradient-to-r from-primary-500/10 to-primary-600/5 px-6 py-5 dark:border-dark-700 dark:from-primary-500/20 dark:to-primary-600/10"
        >
          <div class="flex items-center gap-4">
            <!-- Avatar -->
            <div
              class="flex h-16 w-16 items-center justify-center rounded-2xl bg-gradient-to-br from-primary-500 to-primary-600 text-2xl font-bold text-white shadow-lg shadow-primary-500/20"
            >
              {{ user?.email?.charAt(0).toUpperCase() || 'U' }}
            </div>
            <div class="min-w-0 flex-1">
              <h2 class="truncate text-lg font-semibold text-gray-900 dark:text-white">
                {{ user?.email }}
              </h2>
              <div class="mt-1 flex items-center gap-2">
                <span :class="['badge', user?.role === 'admin' ? 'badge-primary' : 'badge-gray']">
                  {{ user?.role === 'admin' ? t('profile.administrator') : t('profile.user') }}
                </span>
                <span
                  :class="['badge', user?.status === 'active' ? 'badge-success' : 'badge-danger']"
                >
                  {{ user?.status }}
                </span>
              </div>
            </div>
          </div>
        </div>
        <div class="px-6 py-4">
          <div class="space-y-3">
            <div class="flex items-center gap-3 text-sm text-gray-600 dark:text-gray-400">
              <svg
                class="h-4 w-4 text-gray-400 dark:text-gray-500"
                fill="none"
                viewBox="0 0 24 24"
                stroke="currentColor"
                stroke-width="1.5"
              >
                <path
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  d="M21.75 6.75v10.5a2.25 2.25 0 01-2.25 2.25h-15a2.25 2.25 0 01-2.25-2.25V6.75m19.5 0A2.25 2.25 0 0019.5 4.5h-15a2.25 2.25 0 00-2.25 2.25m19.5 0v.243a2.25 2.25 0 01-1.07 1.916l-7.5 4.615a2.25 2.25 0 01-2.36 0L3.32 8.91a2.25 2.25 0 01-1.07-1.916V6.75"
                />
              </svg>
              <span class="truncate">{{ user?.email }}</span>
            </div>
            <div
              v-if="user?.username"
              class="flex items-center gap-3 text-sm text-gray-600 dark:text-gray-400"
            >
              <svg
                class="h-4 w-4 text-gray-400 dark:text-gray-500"
                fill="none"
                viewBox="0 0 24 24"
                stroke="currentColor"
                stroke-width="1.5"
              >
                <path
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  d="M15.75 6a3.75 3.75 0 11-7.5 0 3.75 3.75 0 017.5 0zM4.501 20.118a7.5 7.5 0 0114.998 0A17.933 17.933 0 0112 21.75c-2.676 0-5.216-.584-7.499-1.632z"
                />
              </svg>
              <span class="truncate">{{ user.username }}</span>
            </div>
            <div
              v-if="user?.wechat"
              class="flex items-center gap-3 text-sm text-gray-600 dark:text-gray-400"
            >
              <svg
                class="h-4 w-4 text-gray-400 dark:text-gray-500"
                fill="none"
                viewBox="0 0 24 24"
                stroke="currentColor"
                stroke-width="1.5"
              >
                <path
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  d="M8.625 12a.375.375 0 11-.75 0 .375.375 0 01.75 0zm0 0H8.25m4.125 0a.375.375 0 11-.75 0 .375.375 0 01.75 0zm0 0H12m4.125 0a.375.375 0 11-.75 0 .375.375 0 01.75 0zm0 0h-.375M21 12c0 4.556-4.03 8.25-9 8.25a9.764 9.764 0 01-2.555-.337A5.972 5.972 0 015.41 20.97a5.969 5.969 0 01-.474-.065 4.48 4.48 0 00.978-2.025c.09-.457-.133-.901-.467-1.226C3.93 16.178 3 14.189 3 12c0-4.556 4.03-8.25 9-8.25s9 3.694 9 8.25z"
                />
              </svg>
              <span class="truncate">{{ user.wechat }}</span>
            </div>
          </div>
        </div>
      </div>

      <!-- Contact Support Section -->
      <div
        v-if="contactInfo"
        class="card border-primary-200 bg-gradient-to-r from-primary-50 to-primary-100/50 dark:border-primary-800/40 dark:from-primary-900/20 dark:to-primary-800/10"
      >
        <div class="px-6 py-5">
          <div class="flex items-center gap-4">
            <div
              class="flex h-12 w-12 flex-shrink-0 items-center justify-center rounded-xl bg-primary-100 dark:bg-primary-900/30"
            >
              <svg
                class="h-6 w-6 text-primary-600 dark:text-primary-400"
                fill="none"
                viewBox="0 0 24 24"
                stroke="currentColor"
                stroke-width="1.5"
              >
                <path
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  d="M8.625 12a.375.375 0 11-.75 0 .375.375 0 01.75 0zm0 0H8.25m4.125 0a.375.375 0 11-.75 0 .375.375 0 01.75 0zm0 0H12m4.125 0a.375.375 0 11-.75 0 .375.375 0 01.75 0zm0 0h-.375M21 12c0 4.556-4.03 8.25-9 8.25a9.764 9.764 0 01-2.555-.337A5.972 5.972 0 015.41 20.97a5.969 5.969 0 01-.474-.065 4.48 4.48 0 00.978-2.025c.09-.457-.133-.901-.467-1.226C3.93 16.178 3 14.189 3 12c0-4.556 4.03-8.25 9-8.25s9 3.694 9 8.25z"
                />
              </svg>
            </div>
            <div class="min-w-0 flex-1">
              <h3 class="text-sm font-semibold text-primary-800 dark:text-primary-200">
                {{ t('common.contactSupport') }}
              </h3>
              <p class="mt-1 text-sm font-medium text-primary-600 dark:text-primary-300">
                {{ contactInfo }}
              </p>
            </div>
          </div>
        </div>
      </div>

      <!-- Edit Profile Section -->
      <div class="card">
        <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
          <h2 class="text-lg font-medium text-gray-900 dark:text-white">
            {{ t('profile.editProfile') }}
          </h2>
        </div>
        <div class="px-6 py-6">
          <form @submit.prevent="handleUpdateProfile" class="space-y-4">
            <div>
              <label for="username" class="input-label">
                {{ t('profile.username') }}
              </label>
              <input
                id="username"
                v-model="profileForm.username"
                type="text"
                class="input"
                :placeholder="t('profile.enterUsername')"
              />
            </div>

            <div>
              <label for="wechat" class="input-label">
                {{ t('profile.wechat') }}
              </label>
              <input
                id="wechat"
                v-model="profileForm.wechat"
                type="text"
                class="input"
                :placeholder="t('profile.enterWechat')"
              />
            </div>

            <div class="flex justify-end pt-4">
              <button type="submit" :disabled="updatingProfile" class="btn btn-primary">
                {{ updatingProfile ? t('profile.updating') : t('profile.updateProfile') }}
              </button>
            </div>
          </form>
        </div>
      </div>

      <!-- Change Password Section -->
      <div class="card">
        <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
          <h2 class="text-lg font-medium text-gray-900 dark:text-white">
            {{ t('profile.changePassword') }}
          </h2>
        </div>
        <div class="px-6 py-6">
          <form @submit.prevent="handleChangePassword" class="space-y-4">
            <div>
              <label for="old_password" class="input-label">
                {{ t('profile.currentPassword') }}
              </label>
              <input
                id="old_password"
                v-model="passwordForm.old_password"
                type="password"
                required
                autocomplete="current-password"
                class="input"
              />
            </div>

            <div>
              <label for="new_password" class="input-label">
                {{ t('profile.newPassword') }}
              </label>
              <input
                id="new_password"
                v-model="passwordForm.new_password"
                type="password"
                required
                autocomplete="new-password"
                class="input"
              />
              <p class="input-hint">
                {{ t('profile.passwordHint') }}
              </p>
            </div>

            <div>
              <label for="confirm_password" class="input-label">
                {{ t('profile.confirmNewPassword') }}
              </label>
              <input
                id="confirm_password"
                v-model="passwordForm.confirm_password"
                type="password"
                required
                autocomplete="new-password"
                class="input"
              />
            </div>

            <div class="flex justify-end pt-4">
              <button type="submit" :disabled="changingPassword" class="btn btn-primary">
                {{
                  changingPassword
                    ? t('profile.changingPassword')
                    : t('profile.changePasswordButton')
                }}
              </button>
            </div>
          </form>
        </div>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, computed, h, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAuthStore } from '@/stores/auth'
import { useAppStore } from '@/stores/app'
import { formatDate } from '@/utils/format'

const { t } = useI18n()
import { userAPI, authAPI } from '@/api'
import AppLayout from '@/components/layout/AppLayout.vue'
import StatCard from '@/components/common/StatCard.vue'

// SVG Icon Components
const WalletIcon = {
  render: () =>
    h(
      'svg',
      { fill: 'none', viewBox: '0 0 24 24', stroke: 'currentColor', 'stroke-width': '1.5' },
      [
        h('path', {
          'stroke-linecap': 'round',
          'stroke-linejoin': 'round',
          d: 'M21 12a2.25 2.25 0 00-2.25-2.25H15a3 3 0 11-6 0H5.25A2.25 2.25 0 003 12m18 0v6a2.25 2.25 0 01-2.25 2.25H5.25A2.25 2.25 0 013 18v-6m18 0V9M3 12V9m18 0a2.25 2.25 0 00-2.25-2.25H5.25A2.25 2.25 0 003 9m18 0V6a2.25 2.25 0 00-2.25-2.25H5.25A2.25 2.25 0 003 6v3'
        })
      ]
    )
}

const BoltIcon = {
  render: () =>
    h(
      'svg',
      { fill: 'none', viewBox: '0 0 24 24', stroke: 'currentColor', 'stroke-width': '1.5' },
      [
        h('path', {
          'stroke-linecap': 'round',
          'stroke-linejoin': 'round',
          d: 'm3.75 13.5 10.5-11.25L12 10.5h8.25L9.75 21.75 12 13.5H3.75z'
        })
      ]
    )
}

const CalendarIcon = {
  render: () =>
    h(
      'svg',
      { fill: 'none', viewBox: '0 0 24 24', stroke: 'currentColor', 'stroke-width': '1.5' },
      [
        h('path', {
          'stroke-linecap': 'round',
          'stroke-linejoin': 'round',
          d: 'M6.75 3v2.25M17.25 3v2.25M3 18.75V7.5a2.25 2.25 0 012.25-2.25h13.5A2.25 2.25 0 0121 7.5v11.25m-18 0A2.25 2.25 0 005.25 21h13.5A2.25 2.25 0 0021 18.75m-18 0v-7.5A2.25 2.25 0 015.25 9h13.5A2.25 2.25 0 0121 11.25v7.5'
        })
      ]
    )
}

const authStore = useAuthStore()
const appStore = useAppStore()

const user = computed(() => authStore.user)

const passwordForm = ref({
  old_password: '',
  new_password: '',
  confirm_password: ''
})

const profileForm = ref({
  username: '',
  wechat: ''
})

const changingPassword = ref(false)
const updatingProfile = ref(false)
const contactInfo = ref('')

onMounted(async () => {
  try {
    const settings = await authAPI.getPublicSettings()
    contactInfo.value = settings.contact_info || ''

    // Initialize profile form with current user data
    if (user.value) {
      profileForm.value.username = user.value.username || ''
      profileForm.value.wechat = user.value.wechat || ''
    }
  } catch (error) {
    console.error('Failed to load contact info:', error)
  }
})

const formatCurrency = (value: number): string => {
  return `$${value.toFixed(2)}`
}

const handleChangePassword = async () => {
  // Validate password match
  if (passwordForm.value.new_password !== passwordForm.value.confirm_password) {
    appStore.showError(t('profile.passwordsNotMatch'))
    return
  }

  // Validate password length
  if (passwordForm.value.new_password.length < 8) {
    appStore.showError(t('profile.passwordTooShort'))
    return
  }

  changingPassword.value = true
  try {
    await userAPI.changePassword(passwordForm.value.old_password, passwordForm.value.new_password)

    // Clear form
    passwordForm.value = {
      old_password: '',
      new_password: '',
      confirm_password: ''
    }

    appStore.showSuccess(t('profile.passwordChangeSuccess'))
  } catch (error: any) {
    appStore.showError(error.response?.data?.detail || t('profile.passwordChangeFailed'))
  } finally {
    changingPassword.value = false
  }
}

const handleUpdateProfile = async () => {
  updatingProfile.value = true
  try {
    const updatedUser = await userAPI.updateProfile({
      username: profileForm.value.username,
      wechat: profileForm.value.wechat
    })

    // Update auth store with new user data
    authStore.user = updatedUser

    appStore.showSuccess(t('profile.updateSuccess'))
  } catch (error: any) {
    appStore.showError(error.response?.data?.detail || t('profile.updateFailed'))
  } finally {
    updatingProfile.value = false
  }
}
</script>
