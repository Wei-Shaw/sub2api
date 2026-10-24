<template>
  <footer class="relative z-10 border-t border-gray-200/60 bg-white/50 px-6 py-10 backdrop-blur-sm dark:border-dark-800/60 dark:bg-dark-950/40">
    <div class="mx-auto max-w-6xl">
      <div class="flex flex-col items-center gap-6">
        <!-- Navigation links -->
        <div class="flex flex-wrap items-center justify-center gap-x-6 gap-y-2">
          <a
            v-if="docUrl"
            :href="docUrl"
            target="_blank"
            rel="noopener noreferrer"
            class="text-sm text-gray-500 transition-colors hover:text-primary-600 dark:text-dark-400 dark:hover:text-primary-400"
          >
            {{ t('home.docs') }}
          </a>
          <router-link
            v-for="doc in legalDocuments"
            :key="doc.id"
            :to="`/legal/${doc.id}`"
            class="text-sm text-gray-500 transition-colors hover:text-primary-600 dark:text-dark-400 dark:hover:text-primary-400"
          >
            {{ doc.title }}
          </router-link>
          <span
            v-if="contactInfo"
            class="text-sm text-gray-500 dark:text-dark-400"
          >
            {{ contactInfo }}
          </span>
        </div>

        <!-- Copyright -->
        <p class="text-center text-sm text-gray-500 dark:text-dark-400">
          &copy; {{ currentYear }} {{ siteName }}. {{ t('home.footer.allRightsReserved') }}
        </p>

        <!-- Legal filings (ICP / PSB) -->
        <div
          v-if="hasLegalFilings"
          class="flex flex-wrap items-center justify-center gap-x-5 gap-y-2"
        >
          <a
            v-if="footerIcpNumber"
            :href="footerIcpLink"
            target="_blank"
            rel="noopener noreferrer"
            class="inline-flex items-center gap-1.5 text-xs text-gray-400 transition-colors hover:text-gray-600 dark:text-dark-500 dark:hover:text-dark-300"
          >
            <span class="inline-block h-3.5 w-3.5 rounded-sm bg-gray-300 dark:bg-dark-600" aria-hidden="true"></span>
            {{ footerIcpNumber }}
          </a>
          <a
            v-if="footerPsbNumber"
            :href="footerPsbLink"
            target="_blank"
            rel="noopener noreferrer"
            class="inline-flex items-center gap-1.5 text-xs text-gray-400 transition-colors hover:text-gray-600 dark:text-dark-500 dark:hover:text-dark-300"
          >
            <svg class="h-3.5 w-3.5" viewBox="0 0 20 20" fill="currentColor" aria-hidden="true">
              <path
                fill-rule="evenodd"
                d="M10 18a8 8 0 100-16 8 8 0 000 16zM9.555 7.168A1 1 0 008 8v4a1 1 0 001.555.832l3-2a1 1 0 000-1.664l-3-2z"
                clip-rule="evenodd"
              />
            </svg>
            {{ footerPsbNumber }}
          </a>
        </div>
      </div>
    </div>
  </footer>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores'
import { sanitizeUrl } from '@/utils/url'

const { t } = useI18n()
const appStore = useAppStore()

const defaultIcpUrl = 'https://beian.miit.gov.cn/'

const siteName = computed(
  () => appStore.cachedPublicSettings?.site_name || appStore.siteName || 'Sub2API'
)
const docUrl = computed(() =>
  sanitizeUrl(appStore.cachedPublicSettings?.doc_url || appStore.docUrl || '')
)
const contactInfo = computed(
  () => (appStore.cachedPublicSettings?.contact_info || '').trim()
)
const footerIcpNumber = computed(
  () => (appStore.cachedPublicSettings?.footer_icp_number || '').trim()
)
const footerIcpUrl = computed(
  () => (appStore.cachedPublicSettings?.footer_icp_url || '').trim()
)
const footerPsbNumber = computed(
  () => (appStore.cachedPublicSettings?.footer_psb_number || '').trim()
)
const footerPsbUrl = computed(
  () => (appStore.cachedPublicSettings?.footer_psb_url || '').trim()
)
const legalDocuments = computed(() => {
  const docs = appStore.cachedPublicSettings?.login_agreement_documents
  if (!Array.isArray(docs)) return []
  return docs.filter((doc) => doc?.id && doc?.title)
})

const currentYear = computed(() => new Date().getFullYear())

const footerIcpLink = computed(() => {
  const sanitized = sanitizeUrl(footerIcpUrl.value)
  return sanitized || defaultIcpUrl
})

const footerPsbLink = computed(() => {
  const sanitized = sanitizeUrl(footerPsbUrl.value)
  return sanitized || '#'
})

const hasLegalFilings = computed(
  () => Boolean(footerIcpNumber.value || footerPsbNumber.value)
)
</script>
