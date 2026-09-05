<template>
  <AuthLayout>
    <div class="space-y-6">
      <div class="text-center">
        <h2 class="text-2xl font-bold text-gray-900 dark:text-white">{{ t('auth.welcomeBack') }}</h2>
        <p class="mt-2 text-sm text-gray-500 dark:text-dark-400">{{ t('auth.web3.loginSubtitle') }}</p>
      </div>
      <AuthMethodTabs active="web3" context="login" />
      <Web3WalletPanel intent="login" />
      <p class="text-center text-sm text-gray-500 dark:text-dark-400">
        {{ t('auth.dontHaveAccount') }}
        <RouterLink to="/register/web3" class="font-medium text-primary-600 hover:text-primary-500 dark:text-primary-400">
          {{ t('auth.signUp') }}
        </RouterLink>
      </p>
    </div>
  </AuthLayout>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { AuthLayout } from '@/components/layout'
import AuthMethodTabs from '@/components/auth/AuthMethodTabs.vue'
import Web3WalletPanel from '@/components/auth/Web3WalletPanel.vue'

const { t } = useI18n()
</script>
