<template>
  <div class="min-h-screen bg-gray-50 px-4 py-10 dark:bg-dark-900">
    <div class="mx-auto max-w-2xl">
      <div class="card p-6">
        <h1 class="text-lg font-semibold text-gray-900 dark:text-white">
          {{ t('auth.oauth.callbackTitle') REDACTEDREDACTED
        </h1>
        <p class="mt-2 text-sm text-gray-600 dark:text-gray-400">
          {{ t('auth.oauth.callbackHint') REDACTEDREDACTED
        </p>

        <div class="mt-6 space-y-4">
          <div>
            <label class="input-label">{{ t('auth.oauth.code') REDACTEDREDACTED</label>
            <div class="flex gap-2">
              <input class="input flex-1 font-mono text-sm" :value="code" readonly />
              <button class="btn btn-secondary" type="button" :disabled="!code" @click="copy(code)">
                {{ t('common.copy') REDACTEDREDACTED
              </button>
            </div>
          </div>

          <div>
            <label class="input-label">{{ t('auth.oauth.state') REDACTEDREDACTED</label>
            <div class="flex gap-2">
              <input class="input flex-1 font-mono text-sm" :value="state" readonly />
              <button
                class="btn btn-secondary"
                type="button"
                :disabled="!state"
                @click="copy(state)"
              >
                {{ t('common.copy') REDACTEDREDACTED
              </button>
            </div>
          </div>

          <div>
            <label class="input-label">{{ t('auth.oauth.fullUrl') REDACTEDREDACTED</label>
            <div class="flex gap-2">
              <input class="input flex-1 font-mono text-xs" :value="fullUrl" readonly />
              <button
                class="btn btn-secondary"
                type="button"
                :disabled="!fullUrl"
                @click="copy(fullUrl)"
              >
                {{ t('common.copy') REDACTEDREDACTED
              </button>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, watch REDACTED from 'vue'
import { useI18n REDACTED from 'vue-i18n'
import { useRoute REDACTED from 'vue-router'
import { useClipboard REDACTED from '@/composables/useClipboard'
import { useAppStore REDACTED from '@/stores'

const route = useRoute()
const { t REDACTED = useI18n()
const { copyToClipboard REDACTED = useClipboard()
const appStore = useAppStore()

const code = computed(() => (route.query.code as string) || '')
const state = computed(() => (route.query.state as string) || '')
const error = computed(
  () => (route.query.error as string) || (route.query.error_description as string) || ''
)

const fullUrl = computed(() => {
  if (typeof window === 'undefined') return ''
  return window.location.href
REDACTED)

watch(
  error,
  (message) => {
    if (message) {
      appStore.showError(message)
    REDACTED
  REDACTED,
  { immediate: true REDACTED
)

const copy = (value: string) => {
  if (!value) return
  copyToClipboard(value)
REDACTED
</script>
