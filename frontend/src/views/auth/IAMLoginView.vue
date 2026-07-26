<template>
  <AuthLayout>
    <div class="space-y-6">
      <div class="grid grid-cols-2 rounded-md bg-gray-100 p-1 dark:bg-dark-800">
        <router-link to="/login" class="rounded px-3 py-2 text-center text-sm text-gray-600 dark:text-dark-300">
          {{ t('organization.login.personal') }}
        </router-link>
        <span class="rounded bg-white px-3 py-2 text-center text-sm font-medium text-gray-900 shadow-sm dark:bg-dark-700 dark:text-white">
          {{ t('organization.login.iam') }}
        </span>
      </div>

      <div>
        <h2 class="text-xl font-semibold text-gray-900 dark:text-white">{{ t('organization.login.title') }}</h2>
        <p class="mt-1 text-sm text-gray-500 dark:text-dark-400">{{ t('organization.login.subtitle') }}</p>
      </div>

      <form class="space-y-4" @submit.prevent="submit">
        <div>
          <label class="input-label" for="iam-login-name">{{ t('organization.login.loginName') }}</label>
          <input id="iam-login-name" v-model.trim="form.login_name" class="input" required autocomplete="username" />
        </div>
        <div>
          <label class="input-label" for="iam-account-id">{{ t('organization.accountId') }}</label>
          <input id="iam-account-id" v-model.trim="form.account_id" class="input font-mono" required inputmode="numeric" maxlength="16" pattern="[1-9][0-9]{15}" />
        </div>
        <div>
          <label class="input-label" for="iam-password">{{ t('auth.passwordLabel') }}</label>
          <input id="iam-password" v-model="form.password" class="input" required type="password" autocomplete="current-password" />
        </div>
        <p v-if="error" class="text-sm text-red-600 dark:text-red-400">{{ t('organization.login.genericError') }}</p>
        <button class="btn btn-primary w-full" type="submit" :disabled="loading">
          {{ loading ? t('auth.signingIn') : t('auth.signIn') }}
        </button>
      </form>
    </div>
  </AuthLayout>
</template>

<script setup lang="ts">
import { reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import AuthLayout from '@/components/layout/AuthLayout.vue'
import { useAuthStore } from '@/stores'

const { t } = useI18n()
const router = useRouter()
const auth = useAuthStore()
const loading = ref(false)
const error = ref(false)
const form = reactive({ login_name: '', account_id: '', password: '' })

async function submit() {
  loading.value = true
  error.value = false
  try {
    const response = await auth.loginIAM(form)
    await router.replace(response.user.must_change_password ? '/organization/change-password' : '/dashboard')
  } catch {
    error.value = true
  } finally {
    loading.value = false
  }
}
</script>
