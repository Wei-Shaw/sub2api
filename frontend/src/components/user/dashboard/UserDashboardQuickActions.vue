<template>
  <section class="card">
    <div class="card-header">
      <h2 class="card-title">{{ t('dashboard.quickActions') }}</h2>
    </div>
    <div class="flex flex-col divide-y divide-border">
      <router-link
        v-for="action in actions"
        :key="action.to"
        :to="action.to"
        class="group flex items-center gap-3 px-5 py-3 text-left transition-colors hover:bg-accent-weak focus:outline-none focus-visible:bg-accent-weak"
      >
        <Icon :name="action.icon" size="sm" class="shrink-0 text-accent" aria-hidden="true" />
        <span class="min-w-0 flex-1">
          <span class="block truncate text-body font-semibold text-fg group-hover:text-accent-strong">{{ t(action.title) }}</span>
          <span class="block truncate text-meta text-fg-muted">{{ t(action.desc) }}</span>
        </span>
        <Icon name="chevronRight" size="sm" class="shrink-0 text-fg-subtle group-hover:text-accent-strong" aria-hidden="true" />
      </router-link>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import { useBatchImageAccess } from '@/composables/useBatchImageAccess'
import { useAuthStore } from '@/stores/auth'

type IconName = InstanceType<typeof Icon>['$props']['name']

const { t } = useI18n()
const authStore = useAuthStore()
const { canUseBatchImage, refreshBatchImageAccess } = useBatchImageAccess()

const actions = computed<{ to: string; icon: IconName; title: string; desc: string }[]>(() => [
  { to: '/keys', icon: 'key', title: 'dashboard.createApiKey', desc: 'dashboard.generateNewKey' },
  { to: '/usage', icon: 'chart', title: 'dashboard.viewUsage', desc: 'dashboard.checkDetailedLogs' },
  ...(canUseBatchImage.value
    ? [{ to: '/batch-image', icon: 'sparkles' as IconName, title: 'dashboard.batchImageAgent', desc: 'dashboard.batchImageAgentDesc' }]
    : []),
  // The router blocks /redeem in simple mode, so do not link to it there.
  ...(authStore.isSimpleMode
    ? []
    : [{ to: '/redeem', icon: 'gift' as IconName, title: 'dashboard.redeemCode', desc: 'dashboard.addBalanceWithCode' }])
])

onMounted(() => {
  void refreshBatchImageAccess()
})
</script>
