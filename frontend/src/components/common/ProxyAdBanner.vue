<template>
  <a
    class="inline-flex max-w-full shrink-0 items-center gap-1 truncate text-xs font-normal text-primary-600 transition-colors hover:underline focus:outline-none focus:ring-2 focus:ring-primary-400 focus:ring-offset-2 dark:text-primary-400 dark:focus:ring-offset-dark-800"
    href="https://bestproxy.com/?keyword=a2e8iuol"
    target="_blank"
    rel="noopener noreferrer"
  >
    <span class="truncate">{{ t('admin.proxies.ad.inline') }}</span>
    <Icon name="externalLink" size="xs" />
  </a>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'

const { t } = useI18n()
</script>
