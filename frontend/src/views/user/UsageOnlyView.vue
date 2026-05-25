<template>
  <main class="min-h-screen bg-gray-50 p-4 dark:bg-dark-950 md:p-6 lg:p-8">
    <div class="mx-auto flex max-w-[1600px] flex-col gap-6">
      <header class="flex justify-end">
        <button type="button" class="btn btn-secondary" :disabled="loggingOut" @click="handleLogout">
          <Icon name="login" size="sm" class="rotate-180" />
          <span>{{ t('nav.logout') }}</span>
        </button>
      </header>

      <UsageOnlyAccountsPanel />
      <UsageContent without-header />
    </div>
  </main>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useAuthStore } from '@/stores/auth'
import Icon from '@/components/icons/Icon.vue'
import UsageContent from './UsageContent.vue'
import UsageOnlyAccountsPanel from './UsageOnlyAccountsPanel.vue'

const router = useRouter()
const { t } = useI18n()
const authStore = useAuthStore()
const loggingOut = ref(false)

const handleLogout = async () => {
  if (loggingOut.value) return
  loggingOut.value = true
  try {
    await authStore.logout()
  } finally {
    loggingOut.value = false
    router.push('/login')
  }
}
</script>
