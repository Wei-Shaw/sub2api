<template>
  <section class="card p-6">
    <h3 class="text-base font-semibold text-gray-900 dark:text-white">
      {{ t('organization.recovery.title') }}
    </h3>

    <form class="mt-4 space-y-4" @submit.prevent="verificationSent ? verify() : sendCode()">
      <div>
        <label class="input-label" for="iam-recovery-email">
          {{ t('organization.members.recoveryEmail') }}
        </label>
        <input
          id="iam-recovery-email"
          v-model.trim="email"
          class="input"
          type="email"
          autocomplete="email"
          required
          :disabled="verificationSent"
        >
      </div>

      <div v-if="verificationSent">
        <label class="input-label" for="iam-recovery-code">
          {{ t('organization.recovery.code') }}
        </label>
        <input
          id="iam-recovery-code"
          v-model.trim="code"
          class="input"
          inputmode="numeric"
          autocomplete="one-time-code"
          required
        >
      </div>

      <p v-if="message" class="text-sm" :class="verified ? 'text-green-600' : 'text-gray-500'">
        {{ message }}
      </p>
      <p v-if="error" class="text-sm text-red-600">{{ error }}</p>

      <div class="flex flex-wrap gap-2">
        <button class="btn btn-primary" type="submit" :disabled="loading || verified">
          {{ verificationSent ? t('organization.recovery.verify') : t('organization.recovery.send') }}
        </button>
        <button
          v-if="verificationSent && !verified"
          class="btn btn-secondary"
          type="button"
          :disabled="loading"
          @click="reset"
        >
          {{ t('organization.recovery.change') }}
        </button>
      </div>
    </form>
  </section>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { organizationAPI } from '@/api'

const { t } = useI18n()
const email = ref('')
const code = ref('')
const loading = ref(false)
const verificationSent = ref(false)
const verified = ref(false)
const message = ref('')
const error = ref('')

async function sendCode() {
  loading.value = true
  error.value = ''
  try {
    await organizationAPI.sendRecoveryEmailCode(email.value)
    verificationSent.value = true
    message.value = t('organization.recovery.sent')
  } catch (cause) {
    error.value = (cause as { message?: string }).message || t('common.error')
  } finally {
    loading.value = false
  }
}

async function verify() {
  loading.value = true
  error.value = ''
  try {
    await organizationAPI.verifyRecoveryEmail(email.value, code.value)
    verified.value = true
    message.value = t('organization.recovery.verified')
  } catch (cause) {
    error.value = (cause as { message?: string }).message || t('common.error')
  } finally {
    loading.value = false
  }
}

function reset() {
  code.value = ''
  verificationSent.value = false
  message.value = ''
  error.value = ''
}
</script>
