<template>
  <div data-testid="company-upgrade-content" class="w-full space-y-6">
    <div>
      <h2 class="text-xl font-semibold text-gray-900 dark:text-white">{{ t('organization.upgrade.title') }}</h2>
    </div>

    <div
      data-testid="upgrade-fee-notice"
      class="border-l-4 border-primary-500 bg-primary-50 px-5 py-4 dark:border-primary-400 dark:bg-primary-950/30"
    >
      <p class="text-sm font-medium text-primary-700 dark:text-primary-300">
        {{ t('organization.upgrade.feeLabel') }}
      </p>
      <p data-testid="upgrade-fee-amount" class="mt-1 text-3xl font-semibold text-gray-900 dark:text-white">
        {{ displayedUpgradeFee }}
      </p>
      <p class="mt-2 text-sm leading-6 text-gray-600 dark:text-dark-300">
        {{ t('organization.upgrade.feeNotice') }}
      </p>
    </div>

    <div v-if="application" data-testid="company-upgrade-application" class="rounded-md border border-gray-200 bg-white p-5 dark:border-dark-700 dark:bg-dark-800">
      <dl class="grid gap-4 sm:grid-cols-2">
        <div><dt class="text-xs text-gray-500">{{ t('organization.companyName') }}</dt><dd class="mt-1 font-medium">{{ application.requested_name }}</dd></div>
        <div v-if="application.requested_english_name"><dt class="text-xs text-gray-500">{{ t('organization.companyEnglishName') }}</dt><dd class="mt-1 font-medium">{{ application.requested_english_name }}</dd></div>
        <div v-if="application.company_size"><dt class="text-xs text-gray-500">{{ t('organization.companySize') }}</dt><dd class="mt-1 font-medium">{{ application.company_size }}</dd></div>
        <div><dt class="text-xs text-gray-500">{{ t('common.status') }}</dt><dd class="mt-1 font-medium">{{ t(`organization.status.${application.status}`) }}</dd></div>
        <div><dt class="text-xs text-gray-500">{{ t('organization.upgrade.chargedFee') }}</dt><dd class="mt-1 font-mono">{{ formatUpgradeFee(application.fee_amount, application.fee_currency) }}</dd></div>
        <div v-if="application.review_reason"><dt class="text-xs text-gray-500">{{ t('organization.reviewReason') }}</dt><dd class="mt-1">{{ application.review_reason }}</dd></div>
      </dl>
      <button v-if="application.status === 'pending'" class="btn btn-secondary mt-5" :disabled="loading" @click="withdraw">
        {{ t('organization.upgrade.withdraw') }}
      </button>
    </div>

    <form v-else-if="eligibility?.eligible" data-testid="company-upgrade-form" class="space-y-4 rounded-md border border-gray-200 bg-white p-5 dark:border-dark-700 dark:bg-dark-800" @submit.prevent="submit">
      <div>
        <label for="company-name" class="input-label">{{ t('organization.companyName') }}</label>
        <input id="company-name" v-model.trim="companyName" class="input" maxlength="255" required />
      </div>
      <div>
        <label for="company-english-name" class="input-label">{{ t('organization.companyEnglishName') }}</label>
        <input
          id="company-english-name"
          v-model.trim="englishName"
          class="input"
          maxlength="255"
          pattern="[A-Za-z0-9 &.,'()\-]+"
          :title="t('organization.upgrade.englishNameHint')"
          required
        />
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('organization.upgrade.englishNameHint') }}</p>
      </div>
      <div>
        <label for="company-size" class="input-label">{{ t('organization.companySize') }}</label>
        <Select
          id="company-size"
          v-model="companySize"
          :options="companySizeOptions"
          :placeholder="t('organization.upgrade.companySizePlaceholder')"
        />
      </div>
      <p v-if="error" class="text-sm text-red-600 dark:text-red-400">{{ error }}</p>
      <button type="submit" class="btn btn-primary" :disabled="loading">{{ t('organization.upgrade.submit') }}</button>
    </form>
    <p v-else class="text-sm text-gray-500">{{ t(`organization.upgrade.ineligible.${eligibility?.reason || 'unknown'}`) }}</p>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { organizationAPI } from '@/api'
import { useAuthStore } from '@/stores'
import Select from '@/components/common/Select.vue'
import type { CompanyApplication, CompanyUpgradeEligibility } from '@/types'

const { t } = useI18n()
const authStore = useAuthStore()
const application = ref<CompanyApplication | null>(null)
const eligibility = ref<CompanyUpgradeEligibility>()
const companyName = ref('')
const englishName = ref('')
const companySize = ref('')
const companySizeOptions = ['1-20', '20-100', '100-300', '300-1000', '1000+'].map((size) => ({ value: size, label: size }))
const loading = ref(false)
const error = ref('')
const displayedUpgradeFee = computed(() => formatUpgradeFee(
  application.value?.fee_amount || eligibility.value?.fee_amount || '20',
  application.value?.fee_currency || eligibility.value?.fee_currency || 'USD',
))

function formatUpgradeFee(amount: string | number | undefined, currency: string): string {
  const parsed = Number(amount)
  if (Number.isFinite(parsed) && parsed === 0) {
    return t('organization.upgrade.free')
  }
  const formattedAmount = Number.isFinite(parsed) ? parsed.toFixed(2) : '0.00'
  const normalizedCurrency = currency.trim().toUpperCase()
  return normalizedCurrency === 'USD' ? `$${formattedAmount}` : `${formattedAmount} ${normalizedCurrency}`
}

async function load() {
  eligibility.value = await organizationAPI.getUpgradeEligibility()
  const currentApplication = eligibility.value.application || await organizationAPI.getCurrentApplication()
  application.value = currentApplication?.status === 'withdrawn' ? null : currentApplication
}

async function refreshCurrentUser() {
  try {
    await authStore.refreshUser()
  } catch {
    return
  }
}

async function submit() {
  error.value = ''
  if (!companySize.value) {
    error.value = t('organization.upgrade.companySizeInvalid')
    return
  }
  loading.value = true
  try {
    application.value = await organizationAPI.submitApplication(companyName.value, englishName.value, companySize.value, crypto.randomUUID())
    await refreshCurrentUser()
  } catch (cause) {
    const apiError = cause as { code?: string; message?: string }
    if (apiError.code === 'INSUFFICIENT_BALANCE') {
      error.value = t('organization.upgrade.insufficientBalance')
    } else if (apiError.code === 'COMPANY_ENGLISH_NAME_TAKEN') {
      error.value = t('organization.upgrade.englishNameTaken')
    } else if (apiError.code === 'COMPANY_ENGLISH_NAME_INVALID') {
      error.value = t('organization.upgrade.englishNameInvalid')
    } else if (apiError.code === 'COMPANY_SIZE_INVALID') {
      error.value = t('organization.upgrade.companySizeInvalid')
    } else {
      error.value = apiError.message || t('common.error')
    }
  } finally { loading.value = false }
}
async function withdraw() {
  if (!application.value) return
  loading.value = true
  try {
    await organizationAPI.withdrawApplication(application.value.id)
    companyName.value = ''
    englishName.value = ''
    companySize.value = ''
    await Promise.all([load(), refreshCurrentUser()])
  }
  finally { loading.value = false }
}
onMounted(load)
</script>
