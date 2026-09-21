<template>
  <AppLayout>
    <div class="mx-auto max-w-6xl space-y-5">
      <section class="card overflow-hidden">
        <div class="border-b border-gray-100 px-6 pt-6 dark:border-dark-700">
          <div class="flex flex-wrap items-start justify-between gap-4">
            <div>
              <h1 class="text-2xl font-bold text-gray-900 dark:text-white">{{ t('tutorials.title') }}</h1>
              <p class="mt-1 text-sm text-gray-500 dark:text-dark-400">{{ t('tutorials.description') }}</p>
            </div>
            <Icon name="book" size="lg" class="text-primary-500" />
          </div>
          <div class="mt-5 flex gap-2 overflow-x-auto" role="tablist" :aria-label="t('tutorials.title')">
            <button
              v-for="tab in tabs"
              :key="tab.id"
              type="button"
              role="tab"
              :aria-selected="activeTab === tab.id"
              :class="[
                'whitespace-nowrap rounded-t-lg border-b-2 px-4 py-3 text-sm font-medium transition-colors',
                activeTab === tab.id
                  ? 'border-primary-500 text-primary-600 dark:text-primary-400'
                  : 'border-transparent text-gray-500 hover:border-gray-300 hover:text-gray-700 dark:text-dark-400 dark:hover:border-dark-500 dark:hover:text-dark-200'
              ]"
              @click="activeTab = tab.id"
            >
              {{ tab.label }}
            </button>
          </div>
        </div>
        <div class="px-6 py-7 md:px-10">
          <R6MarkdownContent :markdown="activeMarkdown" />
        </div>
      </section>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import R6MarkdownContent from '@/components/user/R6MarkdownContent.vue'
import workBuddyMarkdown from '@/content/r6/work-buddy.md?raw'
import codexWindowsMarkdown from '@/content/r6/codex-windows.md?raw'

type TutorialTab = 'work-buddy' | 'codex-windows'

const { t } = useI18n()
const activeTab = ref<TutorialTab>('work-buddy')
const tabs = computed(() => [
  { id: 'work-buddy' as const, label: t('tutorials.workBuddy') },
  { id: 'codex-windows' as const, label: t('tutorials.codexWindows') }
])
const activeMarkdown = computed(() => activeTab.value === 'work-buddy' ? workBuddyMarkdown : codexWindowsMarkdown)
</script>
