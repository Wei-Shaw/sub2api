<template>
  <div class="min-h-screen bg-gray-50 dark:bg-dark-950">
    <header class="border-b border-gray-200 bg-white dark:border-dark-800 dark:bg-dark-900">
      <div class="mx-auto max-w-4xl px-6 py-5 flex items-center justify-between">
        <RouterLink to="/home" class="flex items-center gap-3">
          <img src="/logo.png" alt="Sub2API" class="h-8 w-8" />
          <span class="font-semibold text-gray-900 dark:text-white">Sub2API</span>
        </RouterLink>
        <RouterLink
          to="/home"
          class="text-sm text-primary-600 hover:underline dark:text-primary-300"
        >
          {{ locale === 'zh' ? '返回首页' : 'Back to Home' }}
        </RouterLink>
      </div>
    </header>

    <main class="mx-auto max-w-3xl px-6 py-12">
      <h1 class="text-3xl font-bold text-gray-950 dark:text-white">
        {{ locale === 'zh' ? '常见问题' : 'Frequently Asked Questions' }}
      </h1>
      <p class="mt-3 text-gray-600 dark:text-dark-300">
        {{ locale === 'zh' ? '关于 Sub2API 的常见问题与解答。' : 'Common questions and answers about Sub2API.' }}
      </p>

      <dl class="mt-10 space-y-8">
        <div v-for="item in items" :key="item.id" class="border-b border-gray-200 pb-6 dark:border-dark-800">
          <dt :id="item.id" class="text-lg font-semibold text-gray-950 dark:text-white">
            <a :href="`#${item.id}`" class="hover:underline">{{ item.q }}</a>
          </dt>
          <dd class="mt-3 text-gray-700 leading-7 dark:text-dark-200">
            {{ item.a }}
          </dd>
          <details v-if="item.details" class="mt-3 text-sm text-gray-600 dark:text-dark-300">
            <summary class="cursor-pointer">{{ locale === 'zh' ? '更多细节' : 'More details' }}</summary>
            <div class="mt-2 prose prose-sm dark:prose-invert max-w-none" v-html="renderMd(item.details)"></div>
          </details>
        </div>
      </dl>
    </main>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { marked } from 'marked'
import DOMPurify from 'dompurify'
import faqZh from '@shared/seo/faq.zh.json'
import faqEn from '@shared/seo/faq.en.json'

interface FaqItem {
  id: string
  q: string
  a: string
  details?: string
}

const { locale } = useI18n()
const items = computed<FaqItem[]>(() =>
  locale.value === 'en' ? (faqEn as FaqItem[]) : (faqZh as FaqItem[])
)

marked.setOptions({ breaks: true, gfm: true })

// FAQ content comes from shared/seo/faq.{zh,en}.json (build artifact, not user input).
// DOMPurify still sanitizes as defense-in-depth.
function renderMd(md: string): string {
  return DOMPurify.sanitize(marked.parse(md, { async: false }) as string)
}
</script>
