<template>
  <AppLayout>
    <div class="mx-auto flex min-h-[50vh] max-w-3xl items-center justify-center px-4">
      <div class="w-full border-y border-gray-200 py-12 text-center dark:border-dark-700">
        <Icon name="clock" size="xl" class="mx-auto text-gray-400 dark:text-dark-500" />
        <h2 class="mt-4 text-lg font-semibold text-gray-900 dark:text-white">
          {{ pageTitle }}
        </h2>
        <p class="mt-2 text-sm text-gray-500 dark:text-dark-400">
          {{ t('bestloong.pendingModule') }}
        </p>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute } from 'vue-router'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'

const route = useRoute()
const { t } = useI18n()

const pageTitle = computed(() => {
  const titleKey = route.meta.titleKey
  return titleKey ? t(titleKey) : String(route.meta.title ?? '')
})
</script>
