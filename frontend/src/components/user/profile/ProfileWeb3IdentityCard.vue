<template>
  <section
    data-testid="profile-web3-identity"
    class="card border border-violet-100 bg-gradient-to-br from-violet-50 to-white p-6 dark:border-violet-500/20 dark:from-violet-500/10 dark:to-dark-900/60"
  >
    <div class="flex items-start gap-4">
      <div class="rounded-2xl bg-violet-100 p-3 text-violet-600 dark:bg-violet-500/20 dark:text-violet-300">
        <Icon name="link" size="lg" />
      </div>
      <div class="min-w-0 flex-1">
        <div class="flex flex-wrap items-center gap-2">
          <h3 class="font-semibold text-gray-900 dark:text-white">
            {{ t('profile.web3Identity.title') }}
          </h3>
          <span class="rounded-full bg-emerald-100 px-2.5 py-1 text-xs font-medium text-emerald-700 dark:bg-emerald-500/15 dark:text-emerald-300">
            {{ t('profile.web3Identity.connected') }}
          </span>
        </div>
        <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
          {{ t('profile.web3Identity.description') }}
        </p>
        <div class="mt-4 rounded-2xl border border-violet-100 bg-white/80 px-4 py-3 dark:border-violet-500/20 dark:bg-dark-900/50">
          <p class="text-xs font-medium uppercase tracking-[0.16em] text-gray-400 dark:text-gray-500">
            {{ t('profile.web3Identity.address') }}
          </p>
          <p class="mt-1 break-all font-mono text-sm font-semibold text-gray-900 dark:text-white" :title="address">
            {{ address }}
          </p>
        </div>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { Icon } from '@/components/icons'

defineProps<{
  address: string
}>()

const { t } = useI18n()
</script>
