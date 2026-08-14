<template>
  <div data-testid="legal-document-view" class="flex min-h-screen flex-col bg-canvas text-ink">
    <header class="border-b border-line">
      <div class="mx-auto flex max-w-5xl items-center justify-between gap-4 px-6 py-3">
        <RouterLink to="/home" class="flex min-w-0 items-center gap-3 rounded">
          <template v-if="settings">
            <img
              :src="siteLogo || '/logo.svg'"
              alt=""
              class="h-7 w-7 shrink-0 rounded object-contain"
            />
            <span class="min-w-0 truncate text-sm font-semibold [overflow-wrap:anywhere]">
              {{ siteName }}
            </span>
          </template>
          <template v-else>
            <span class="skeleton h-7 w-7 shrink-0 rounded" aria-hidden="true"></span>
            <span class="skeleton h-4 w-28" aria-hidden="true"></span>
          </template>
        </RouterLink>

        <Button to="/login" tone="accent" variant="solid" size="md" class="shrink-0">
          {{ t('home.login') }}
        </Button>
      </div>
    </header>

    <main class="flex-1">
      <div class="mx-auto max-w-5xl px-6 py-12 lg:py-16">
        <!--
          A spinner told the reader nothing about what is arriving. Bars on the
          reading measure show the shape of the document instead.
        -->
        <div v-if="loading" data-testid="legal-loading" class="max-w-[68ch] space-y-4">
          <span class="skeleton block h-3 w-24" aria-hidden="true"></span>
          <span class="skeleton block h-8 w-2/3" aria-hidden="true"></span>
          <span class="skeleton block h-4 w-full" aria-hidden="true"></span>
          <span class="skeleton block h-4 w-11/12" aria-hidden="true"></span>
          <span class="skeleton block h-4 w-4/5" aria-hidden="true"></span>
          <span class="sr-only">{{ t('common.loading') }}</span>
        </div>

        <!-- `danger` is a status token, so it may carry a status. The accent may not. -->
        <section
          v-else-if="loadError"
          data-testid="legal-load-error"
          class="max-w-[68ch] border border-line bg-surface p-8"
        >
          <p class="font-mono text-2xs uppercase tracking-[0.04em] text-danger">
            {{ t('common.error') }}
          </p>
          <h1 class="mt-3 text-lg font-semibold text-ink">{{ t('legal.loadFailed') }}</h1>
          <p class="mt-2 text-sm text-ink-secondary">{{ t('legal.retryLater') }}</p>
        </section>

        <section
          v-else-if="!currentDocument"
          data-testid="legal-missing"
          class="max-w-[68ch] border border-line bg-surface p-8"
        >
          <h1 class="text-lg font-semibold text-ink">{{ t('legal.notFound') }}</h1>
          <p class="mt-2 text-sm leading-6 text-ink-secondary">
            {{ t('legal.notFoundDescription') }}
          </p>
        </section>

        <div
          v-else
          data-testid="legal-document"
          :class="showTableOfContents ? 'lg:grid lg:grid-cols-[minmax(0,1fr)_13rem] lg:gap-16' : ''"
        >
          <article class="min-w-0 max-w-[68ch]">
            <header class="border-b border-line pb-8">
              <p class="text-2xs uppercase tracking-[0.04em] text-ink-tertiary">
                {{ documentTypeLabel }}
              </p>
              <h1 class="mt-4 text-3xl font-semibold [overflow-wrap:anywhere]">
                {{ currentDocument.title }}
              </h1>
              <!-- Quantities are mono and tabular, dates included. -->
              <p
                v-if="updatedAt"
                data-testid="legal-updated-at"
                class="mt-5 font-mono text-2xs uppercase tracking-[0.04em] tabular-nums text-ink-tertiary"
              >
                {{ t('legal.updatedAt', { date: updatedAt }) }}
              </p>
            </header>

            <div
              v-if="hasContent"
              data-testid="legal-prose"
              class="long-form-prose mt-10"
              v-html="renderedHtml"
            ></div>
            <p
              v-else
              data-testid="legal-empty"
              class="mt-10 border border-line bg-surface-sunken px-6 py-12 text-center text-sm text-ink-tertiary"
            >
              {{ t('legal.empty') }}
            </p>
          </article>

          <!--
            Earned, not decorative: a document has to reach TOC_MIN_HEADINGS
            sections before this renders at all, so short agreements stay a
            single unbroken column.
          -->
          <nav
            v-if="showTableOfContents"
            data-testid="legal-toc"
            :aria-label="t('legal.tableOfContents')"
            class="hidden lg:block"
          >
            <div class="sticky top-10">
              <p class="text-2xs uppercase tracking-[0.04em] text-ink-tertiary">
                {{ t('legal.tableOfContents') }}
              </p>
              <ul class="mt-4 border-l border-line-subtle">
                <li v-for="item in tableOfContents" :key="item.id">
                  <a
                    :href="`#${item.id}`"
                    class="-ml-px block border-l border-transparent py-1.5 text-xs text-ink-tertiary transition-colors duration-fast hover:border-line-strong hover:text-ink"
                    :class="item.level === 3 ? 'pl-6' : 'pl-4'"
                  >
                    {{ item.text }}
                  </a>
                </li>
              </ul>
            </div>
          </nav>
        </div>
      </div>
    </main>
  </div>
</template>

<script setup lang="ts">
import DOMPurify from 'dompurify'
import { marked } from 'marked'
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute } from 'vue-router'

import Button from '@/components/common/Button.vue'
import { getLocale } from '@/i18n'
import { useAppStore } from '@/stores/app'
import type { LoginAgreementDocument } from '@/types'
import { sanitizeUrl } from '@/utils/url'

import '@/styles/long-form-prose.css'
import zhAdminCompliance from '../../../../docs/legal/admin-compliance.zh.md?raw'
import enAdminCompliance from '../../../../docs/legal/admin-compliance.en.md?raw'

interface TocEntry {
  id: string
  text: string
  level: 2 | 3
}

/** Below this, a table of contents is chrome rather than navigation. */
const TOC_MIN_HEADINGS = 4

const route = useRoute()
const { t } = useI18n()
const appStore = useAppStore()
const settings = computed(() => appStore.cachedPublicSettings)
const loading = ref(!settings.value)
const loadError = ref(false)

marked.setOptions({
  breaks: true,
  gfm: true,
})

const documentId = computed(() => String(route.params.documentId || ''))
const isAdminComplianceDocument = computed(() => documentId.value === 'admin-compliance')
const documents = computed(() => settings.value?.login_agreement_documents ?? [])
const siteName = computed(() => settings.value?.site_name || 'Sub2API')
const siteLogo = computed(() =>
  sanitizeUrl(settings.value?.site_logo || '', {
    allowRelative: true,
    allowDataUrl: true,
  })
)
const updatedAt = computed(() =>
  isAdminComplianceDocument.value ? '' : settings.value?.login_agreement_updated_at || ''
)
const documentTypeLabel = computed(() =>
  isAdminComplianceDocument.value ? t('legal.adminCompliance') : t('legal.loginAgreement')
)

const currentDocument = computed<LoginAgreementDocument | null>(() => {
  if (isAdminComplianceDocument.value) {
    return {
      id: 'admin-compliance',
      title: t('adminCompliance.title'),
      content_md: getLocale() === 'zh' ? zhAdminCompliance : enAdminCompliance,
    }
  }
  const id = documentId.value
  if (!id) {
    return null
  }
  return documents.value.find((doc) => doc.id === id) ?? null
})

const hasContent = computed(() => Boolean(currentDocument.value?.content_md?.trim()))

/**
 * Render, sanitize, then annotate — in that order. The annotation pass only
 * ever writes an `id` onto a heading DOMPurify has already cleared, so the
 * markdown pipeline itself is untouched and nothing re-enters the document
 * that sanitization did not approve.
 */
const renderedDocument = computed<{ html: string; toc: TocEntry[] }>(() => {
  const content = currentDocument.value?.content_md?.trim() || ''
  if (!content) {
    return { html: '', toc: [] }
  }

  const safe = DOMPurify.sanitize(marked.parse(content) as string)
  const host = document.createElement('div')
  host.innerHTML = safe

  const toc: TocEntry[] = []
  host.querySelectorAll('h2, h3').forEach((heading, index) => {
    const text = heading.textContent?.trim() ?? ''
    if (!text) {
      return
    }
    // Positional ids rather than slugs: these documents also ship in Chinese,
    // and a CJK slug has to be percent-encoded at every `href` pointing at it.
    const id = `legal-section-${index + 1}`
    heading.id = id
    toc.push({ id, text, level: heading.tagName === 'H2' ? 2 : 3 })
  })

  return { html: host.innerHTML, toc }
})

const renderedHtml = computed(() => renderedDocument.value.html)
const tableOfContents = computed(() => renderedDocument.value.toc)
const showTableOfContents = computed(() => tableOfContents.value.length >= TOC_MIN_HEADINGS)

onMounted(async () => {
  loadError.value = false
  const loadedSettings = await appStore.fetchPublicSettings()
  if (!loadedSettings) {
    loadError.value = true
  }
  loading.value = false
})
</script>
