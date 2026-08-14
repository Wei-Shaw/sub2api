<template>
  <Surface :title="t('dashboard.quickActions')" flush data-testid="dashboard-quick-actions">
    <div class="divide-y divide-line-subtle">
      <!--
        A row, not a tile. What is gone: the 48px pastel icon square per action,
        `group-hover:scale-105`, `transition-all`, and the rounded-xl ground.
        Hover moves the background and nothing else.
      -->
      <button
        v-for="action in actions"
        :key="action.key"
        type="button"
        class="flex w-full items-center gap-3 px-4 py-3 text-left transition-colors duration-fast hover:bg-surface-hover"
        @click="router.push(action.to)"
      >
        <Icon :name="action.icon" size="sm" class="shrink-0 text-ink-tertiary" />
        <span class="min-w-0 flex-1">
          <span class="block truncate text-sm font-medium text-ink">{{ action.label }}</span>
          <span class="block truncate text-xs text-ink-tertiary">{{ action.description }}</span>
        </span>
        <Icon name="chevronRight" size="xs" class="shrink-0 text-ink-tertiary" />
      </button>
    </div>
  </Surface>
</template>

<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'

import Surface from '@/components/common/Surface.vue'
import Icon from '@/components/icons/Icon.vue'
import { useBatchImageAccess } from '@/composables/useBatchImageAccess'

const router = useRouter()
const { t } = useI18n()
const { canUseBatchImage, refreshBatchImageAccess } = useBatchImageAccess()

interface QuickAction {
  key: string
  /** Narrow, because `Icon` only accepts names it actually has a path for. */
  icon: 'key' | 'chart' | 'sparkles' | 'gift'
  label: string
  description: string
  to: string
}

const actions = computed<QuickAction[]>(() => {
  const list: QuickAction[] = [
    {
      key: 'keys',
      icon: 'key',
      label: t('dashboard.createApiKey'),
      description: t('dashboard.generateNewKey'),
      to: '/keys',
    },
    {
      key: 'usage',
      icon: 'chart',
      label: t('dashboard.viewUsage'),
      description: t('dashboard.checkDetailedLogs'),
      to: '/usage',
    },
  ]

  if (canUseBatchImage.value) {
    list.push({
      key: 'batch-image',
      icon: 'sparkles',
      label: t('dashboard.batchImageAgent'),
      description: t('dashboard.batchImageAgentDesc'),
      to: '/batch-image',
    })
  }

  list.push({
    key: 'redeem',
    icon: 'gift',
    label: t('dashboard.redeemCode'),
    description: t('dashboard.addBalanceWithCode'),
    to: '/redeem',
  })

  return list
})

onMounted(() => {
  void refreshBatchImageAccess()
})
</script>
