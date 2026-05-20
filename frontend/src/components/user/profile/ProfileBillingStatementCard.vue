<template>
  <div class="card">
    <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
      <h2 class="text-lg font-medium text-gray-900 dark:text-white">
        {{ t('profile.billingStatement.title') }}
      </h2>
      <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
        {{ t('profile.billingStatement.description') }}
      </p>
    </div>
    <div class="px-6 py-6 space-y-4">
      <div class="flex items-center justify-between">
        <label class="input-label mb-0" :class="dailyAvailable ? '' : 'text-gray-400 dark:text-gray-500'">{{ t('profile.billingStatement.dailyEnabled') }}</label>
        <label class="relative inline-flex items-center cursor-pointer">
          <input type="checkbox" v-model="dailyEnabled" :disabled="!dailyAvailable" @change="handleSave" class="sr-only peer" />
          <div class="w-11 h-6 rounded-full peer after:absolute after:top-[2px] after:left-[2px] after:h-5 after:w-5 after:rounded-full after:border after:border-gray-300 after:bg-white after:transition-all after:content-[''] dark:after:border-gray-600" :class="dailyAvailable ? 'bg-gray-200 peer-focus:outline-none peer-focus:ring-4 peer-focus:ring-primary-300 dark:peer-focus:ring-primary-800 peer-checked:after:translate-x-full peer-checked:after:border-white peer-checked:bg-primary-600 dark:bg-gray-700' : 'cursor-not-allowed bg-gray-100 dark:bg-gray-800'"></div>
        </label>
      </div>
      <div class="flex items-center justify-between border-t border-gray-100 pt-4 dark:border-dark-700">
        <label class="input-label mb-0" :class="weeklyAvailable ? '' : 'text-gray-400 dark:text-gray-500'">{{ t('profile.billingStatement.weeklyEnabled') }}</label>
        <label class="relative inline-flex items-center cursor-pointer">
          <input type="checkbox" v-model="weeklyEnabled" :disabled="!weeklyAvailable" @change="handleSave" class="sr-only peer" />
          <div class="w-11 h-6 rounded-full peer after:absolute after:top-[2px] after:left-[2px] after:h-5 after:w-5 after:rounded-full after:border after:border-gray-300 after:bg-white after:transition-all after:content-[''] dark:after:border-gray-600" :class="weeklyAvailable ? 'bg-gray-200 peer-focus:outline-none peer-focus:ring-4 peer-focus:ring-primary-300 dark:peer-focus:ring-primary-800 peer-checked:after:translate-x-full peer-checked:after:border-white peer-checked:bg-primary-600 dark:bg-gray-700' : 'cursor-not-allowed bg-gray-100 dark:bg-gray-800'"></div>
        </label>
      </div>
      <div class="flex items-center justify-between border-t border-gray-100 pt-4 dark:border-dark-700">
        <label class="input-label mb-0" :class="monthlyAvailable ? '' : 'text-gray-400 dark:text-gray-500'">{{ t('profile.billingStatement.monthlyEnabled') }}</label>
        <label class="relative inline-flex items-center cursor-pointer">
          <input type="checkbox" v-model="monthlyEnabled" :disabled="!monthlyAvailable" @change="handleSave" class="sr-only peer" />
          <div class="w-11 h-6 rounded-full peer after:absolute after:top-[2px] after:left-[2px] after:h-5 after:w-5 after:rounded-full after:border after:border-gray-300 after:bg-white after:transition-all after:content-[''] dark:after:border-gray-600" :class="monthlyAvailable ? 'bg-gray-200 peer-focus:outline-none peer-focus:ring-4 peer-focus:ring-primary-300 dark:peer-focus:ring-primary-800 peer-checked:after:translate-x-full peer-checked:after:border-white peer-checked:bg-primary-600 dark:bg-gray-700' : 'cursor-not-allowed bg-gray-100 dark:bg-gray-800'"></div>
        </label>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAuthStore } from '@/stores/auth'
import { useAppStore } from '@/stores/app'
import { userAPI } from '@/api'

const props = defineProps<{
  dailyEnabledInit: boolean
  weeklyEnabledInit: boolean
  monthlyEnabledInit: boolean
  dailyAvailable: boolean
  weeklyAvailable: boolean
  monthlyAvailable: boolean
}>()

const { t } = useI18n()
const authStore = useAuthStore()
const appStore = useAppStore()

const dailyEnabled = ref(props.dailyEnabledInit)
const weeklyEnabled = ref(props.weeklyEnabledInit)
const monthlyEnabled = ref(props.monthlyEnabledInit)

watch(() => props.dailyEnabledInit, (v) => { dailyEnabled.value = v })
watch(() => props.weeklyEnabledInit, (v) => { weeklyEnabled.value = v })
watch(() => props.monthlyEnabledInit, (v) => { monthlyEnabled.value = v })

const handleSave = async () => {
  try {
    const updatedUser = await userAPI.updateProfile({
      ...(props.dailyAvailable ? { billing_statement_daily_enabled: dailyEnabled.value } : {}),
      ...(props.weeklyAvailable ? { billing_statement_weekly_enabled: weeklyEnabled.value } : {}),
      ...(props.monthlyAvailable ? { billing_statement_monthly_enabled: monthlyEnabled.value } : {}),
    })
    authStore.user = updatedUser
    appStore.showSuccess(t('profile.billingStatement.saved'))
  } catch (error: any) {
    appStore.showError(error.response?.data?.detail || t('profile.billingStatement.saveFailed'))
  }
}
</script>
