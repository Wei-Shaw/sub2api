<template>
  <div class="mx-auto max-w-lg space-y-5">
    <h2 class="text-xl font-semibold text-gray-900 dark:text-white">{{ t('organization.password.title') }}</h2>
    <form class="space-y-4 rounded-md border border-gray-200 bg-white p-5 dark:border-dark-700 dark:bg-dark-800" @submit.prevent="submit">
      <div><label class="input-label" for="new-password">{{ t('organization.password.new') }}</label><input id="new-password" v-model="password" class="input" type="password" minlength="8" required autocomplete="new-password" /></div>
      <div><label class="input-label" for="confirm-password">{{ t('organization.password.confirm') }}</label><input id="confirm-password" v-model="confirmation" class="input" type="password" minlength="8" required autocomplete="new-password" /></div>
      <p v-if="error" class="text-sm text-red-600">{{ error }}</p>
      <button class="btn btn-primary" type="submit" :disabled="loading">{{ t('common.save') }}</button>
    </form>
  </div>
</template>
<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { organizationAPI } from '@/api'
import { useAuthStore } from '@/stores'
const { t } = useI18n(); const router = useRouter(); const auth = useAuthStore()
const password = ref(''); const confirmation = ref(''); const loading = ref(false); const error = ref('')
async function submit() {
  if (password.value !== confirmation.value) { error.value = t('organization.password.mismatch'); return }
  loading.value = true; error.value = ''
	try { const response = await organizationAPI.changePassword(password.value); auth.applyAuthResponse(response); await auth.refreshUser(); await router.replace('/dashboard') }
  catch (cause) { error.value = (cause as { message?: string }).message || t('common.error') }
  finally { loading.value = false }
}
</script>
