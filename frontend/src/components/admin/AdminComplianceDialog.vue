<template>
  <BaseDialog
    :show="visible"
    :title="t('adminCompliance.title')"
    width="wide"
    :close-on-escape="false"
    :close-on-click-outside="false"
    :show-close-button="false"
    :z-index="80"
    @close="noop"
  >
    <div class="space-y-5">
      <div class="rounded-lg border border-amber-200 bg-amber-50 p-4 text-sm text-amber-900 dark:border-amber-500/30 dark:bg-amber-500/10 dark:text-amber-100">
        <div class="flex gap-3">
          <Icon name="exclamationTriangle" size="md" class="mt-0.5 flex-shrink-0" />
          <div class="space-y-2">
            <p class="font-semibold">{{ t('adminCompliance.blockingNotice') }}</p>
            <p class="leading-6">{{ t('adminCompliance.riskNotice') }}</p>
          </div>
        </div>
      </div>

      <div class="grid gap-4 md:grid-cols-[minmax(0,1fr)_240px]">
        <section class="min-h-[320px] max-h-[46vh] overflow-y-auto rounded-lg border border-gray-200 bg-white p-5 dark:border-dark-700 dark:bg-dark-900">
          <div class="legal-document-content" v-html="renderedDocument"></div>
        </section>

        <aside class="space-y-3 rounded-lg border border-gray-200 bg-gray-50 p-4 text-sm dark:border-dark-700 dark:bg-dark-900/60">
          <div>
            <p class="text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-dark-400">
              {{ t('adminCompliance.version') }}
            </p>
            <p class="mt-1 break-all font-mono text-gray-900 dark:text-white">
              {{ complianceStore.status?.version || 'v2026.06.10' }}
            </p>
          </div>
          <a
            :href="documentUrl"
            target="_blank"
            rel="noopener noreferrer"
            class="inline-flex items-center gap-2 text-primary-600 underline underline-offset-4 hover:text-primary-700 dark:text-primary-300 dark:hover:text-primary-200"
          >
            <Icon name="externalLink" size="sm" />
            {{ t('adminCompliance.openDocument') }}
          </a>
          <p class="leading-6 text-gray-600 dark:text-dark-300">
            {{ t('adminCompliance.documentSource') }}
          </p>
        </aside>
      </div>

      <div class="space-y-3">
        <label for="admin-compliance-phrase" class="block text-sm font-semibold text-gray-900 dark:text-white">
          {{ t('adminCompliance.inputLabel') }}
        </label>
        <div class="rounded-lg bg-gray-100 px-3 py-2 font-mono text-sm text-gray-900 dark:bg-dark-800 dark:text-dark-100">
          {{ expectedPhrase }}
        </div>
        <Input
          id="admin-compliance-phrase"
          v-model="typedPhrase"
          :placeholder="t('adminCompliance.inputPlaceholder')"
          autocomplete="off"
          :disabled="complianceStore.submitting"
          :error="inputError"
          @enter="submit"
        />
      </div>

      <p class="text-xs leading-5 text-gray-500 dark:text-dark-400">
        {{ t('adminCompliance.legalNote') }}
      </p>
    </div>

    <template #footer>
      <div class="flex flex-col gap-3 sm:flex-row sm:justify-end">
        <button
          type="button"
          class="btn btn-secondary"
          :disabled="complianceStore.submitting"
          @click="logout"
        >
          {{ t('adminCompliance.logout') }}
        </button>
        <button
          type="button"
          class="btn btn-primary"
          :disabled="!canSubmit || complianceStore.submitting"
          @click="submit"
        >
          <span v-if="complianceStore.submitting">{{ t('common.submitting') }}</span>
          <span v-else>{{ t('adminCompliance.accept') }}</span>
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { marked } from 'marked'
import DOMPurify from 'dompurify'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Input from '@/components/common/Input.vue'
import Icon from '@/components/icons/Icon.vue'
import { useAdminComplianceStore, useAppStore, useAuthStore } from '@/stores'
import { getLocale } from '@/i18n'
import zhDocument from '../../../../docs/legal/admin-compliance.zh.md?raw'
import enDocument from '../../../../docs/legal/admin-compliance.en.md?raw'

const { t } = useI18n()
const complianceStore = useAdminComplianceStore()
const authStore = useAuthStore()
const appStore = useAppStore()
const typedPhrase = ref('')
const attemptedSubmit = ref(false)

marked.setOptions({
  breaks: true,
  gfm: true,
})

const visible = computed(() => authStore.isAuthenticated && authStore.isAdmin && complianceStore.shouldShow)
const expectedPhrase = computed(() => complianceStore.expectedPhrase)
const canSubmit = computed(() => typedPhrase.value.trim() === expectedPhrase.value)
const currentDocument = computed(() => getLocale() === 'zh' ? zhDocument : enDocument)
const documentUrl = computed(() => {
  if (getLocale() === 'zh') {
    return complianceStore.status?.document_url_zh || 'https://github.com/Wei-Shaw/sub2api/blob/main/docs/legal/admin-compliance.zh.md'
  }
  return complianceStore.status?.document_url_en || 'https://github.com/Wei-Shaw/sub2api/blob/main/docs/legal/admin-compliance.en.md'
})
const inputError = computed(() => {
  if (!attemptedSubmit.value || canSubmit.value) {
    return ''
  }
  return t('adminCompliance.inputMismatch')
})
const renderedDocument = computed(() => {
  const html = marked.parse(currentDocument.value) as string
  return DOMPurify.sanitize(html)
})

watch(expectedPhrase, () => {
  typedPhrase.value = ''
  attemptedSubmit.value = false
})

watch(visible, (isVisible) => {
  if (isVisible) {
    typedPhrase.value = ''
    attemptedSubmit.value = false
  }
})

function noop(): void {
  // 强制确认弹窗不允许通过关闭按钮绕过。
}

async function submit(): Promise<void> {
  attemptedSubmit.value = true
  if (!canSubmit.value) {
    return
  }

  try {
    const status = await complianceStore.accept(typedPhrase.value.trim())
    if (!status.required) {
      appStore.showSuccess(t('adminCompliance.accepted'))
      typedPhrase.value = ''
      attemptedSubmit.value = false
    }
  } catch (error) {
    const message = (error as { message?: string })?.message || t('adminCompliance.acceptFailed')
    appStore.showError(message)
  }
}

async function logout(): Promise<void> {
  await authStore.logout()
  window.location.href = '/login'
}
</script>

<style scoped>
.legal-document-content {
  line-height: 1.75;
  overflow-wrap: anywhere;
  color: inherit;
}

.legal-document-content :deep(h1) {
  @apply mb-4 text-2xl font-bold text-gray-950 dark:text-white;
}

.legal-document-content :deep(h2) {
  @apply mb-3 mt-6 text-xl font-semibold text-gray-900 dark:text-white;
}

.legal-document-content :deep(p) {
  @apply mb-4 text-sm text-gray-700 dark:text-dark-200;
}

.legal-document-content :deep(ul),
.legal-document-content :deep(ol) {
  @apply mb-4 pl-6 text-sm text-gray-700 dark:text-dark-200;
}

.legal-document-content :deep(ul) {
  @apply list-disc;
}

.legal-document-content :deep(ol) {
  @apply list-decimal;
}

.legal-document-content :deep(li) {
  @apply mb-1;
}

.legal-document-content :deep(strong) {
  @apply font-semibold text-gray-950 dark:text-white;
}
</style>
