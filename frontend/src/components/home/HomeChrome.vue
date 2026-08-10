<template>
  <header :class="headerClass">
    <nav :class="navClass">
      <div class="flex min-w-0 items-center gap-3">
        <img :src="context.siteLogo || '/logo.svg'" :alt="showName ? '' : context.siteName" class="h-9 w-9 shrink-0 rounded-lg object-contain" />
        <span v-if="showName" class="truncate font-semibold">{{ context.siteName }}</span>
      </div>
      <div class="flex max-w-full shrink-0 flex-wrap items-center justify-end gap-2">
        <LocaleSwitcher />
        <a
          v-if="context.docUrl"
          :href="context.docUrl"
          target="_blank"
          rel="noopener noreferrer"
          :class="actionClass"
          :title="t('home.viewDocs')"
          :aria-label="t('home.viewDocs')"
        >
          <Icon name="book" size="md" aria-hidden="true" />
        </a>
        <button
          :class="actionClass"
          :title="context.isDark ? t('home.switchToLight') : t('home.switchToDark')"
          :aria-label="context.isDark ? t('home.switchToLight') : t('home.switchToDark')"
          type="button"
          @click="context.toggleTheme"
        >
          <Icon :name="context.isDark ? 'sun' : 'moon'" size="md" aria-hidden="true" />
        </button>
        <router-link :to="destination" :class="ctaClass">
          {{ context.isAuthenticated ? t('home.dashboard') : t('home.login') }}
        </router-link>
      </div>
    </nav>
  </header>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import LocaleSwitcher from '@/components/common/LocaleSwitcher.vue'
import Icon from '@/components/icons/Icon.vue'
import type { HomeStyleContext } from './types'

const props = withDefaults(defineProps<{
  context: HomeStyleContext
  headerClass?: string
  navClass?: string
  actionClass?: string
  ctaClass?: string
  showName?: boolean
}>(), {
  headerClass: 'px-4 py-4 sm:px-6',
  navClass: 'mx-auto flex max-w-6xl flex-wrap items-center justify-between gap-3',
  actionClass: 'flex h-10 w-10 items-center justify-center rounded-lg transition-colors hover:bg-black/5 dark:hover:bg-white/10',
  ctaClass: 'inline-flex min-h-10 items-center justify-center rounded-lg bg-gray-900 px-4 py-2 text-sm font-medium text-white dark:bg-white dark:text-gray-900',
  showName: true,
})

const { t } = useI18n()
const destination = computed(() => props.context.isAuthenticated ? props.context.dashboardPath : '/login')
</script>
