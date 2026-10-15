<template>
  <AppLayout>
    <div class="store-page">
      <div class="card store-card">
        <div class="store-toolbar">
          <div class="store-service-notice" role="note">
            <Icon name="clock" size="sm" class="flex-shrink-0 text-amber-600 dark:text-amber-400" />
            <p>
              <strong>{{ t('store.serviceHoursTitle') }}</strong>
              <span>{{ t('store.serviceHoursDescription') }}</span>
            </p>
          </div>
          <a
            :href="storeUrl"
            target="_blank"
            rel="noopener noreferrer"
            class="btn btn-secondary btn-sm"
          >
            <Icon name="externalLink" size="sm" class="mr-1.5" />
            {{ t('store.openInNewTab') }}
          </a>
        </div>

        <div class="store-frame-shell">
          <div v-if="loading" class="store-loading">
            <div class="h-8 w-8 animate-spin rounded-full border-2 border-primary-500 border-t-transparent"></div>
            <span>{{ t('store.loading') }}</span>
          </div>
          <iframe
            ref="storeFrame"
            :src="storeUrl"
            :title="t('store.title')"
            class="store-frame"
            referrerpolicy="no-referrer"
            @load="handleLoad"
          ></iframe>
        </div>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import { useAuthStore } from '@/stores/auth'
import { buildStoreUrl } from '@/utils/storeUrl'

const { t } = useI18n()
const authStore = useAuthStore()
const loading = ref(true)
const storeFrame = ref<HTMLIFrameElement | null>(null)
const baseStoreUrl = import.meta.env.VITE_CODEX_STORE_URL
  || 'https://www.gptplusch.store/products?category=other&filter=codex-token&embed=1'
const storeUrl = computed(() => buildStoreUrl(baseStoreUrl, authStore.user?.email))
const handleLoad = () => {
  loading.value = false

  const email = authStore.user?.email?.trim()
  if (!email || !storeFrame.value?.contentWindow) return

  storeFrame.value.contentWindow.postMessage(
    { type: 'sub2api:checkout-email', email },
    new URL(baseStoreUrl).origin
  )
}
</script>

<style scoped>
.store-page {
  height: calc(100vh - 64px - 4rem);
}

.store-card {
  @apply flex h-full min-h-0 flex-col overflow-hidden;
}

.store-toolbar {
  @apply flex flex-shrink-0 items-center justify-between gap-4 border-b border-gray-100 px-4 py-3 dark:border-dark-700;
}

.store-service-notice {
  @apply flex min-w-0 items-center gap-2 text-sm text-amber-800 dark:text-amber-300;
}

.store-service-notice p {
  @apply m-0 leading-6;
}

.store-service-notice strong {
  @apply mr-1 font-semibold;
}

.store-toolbar .btn {
  @apply flex-shrink-0;
}

.store-frame-shell {
  @apply relative min-h-0 flex-1 overflow-hidden bg-[#fbfaff];
}

.store-loading {
  @apply absolute inset-0 z-10 flex items-center justify-center gap-3 bg-white text-sm text-gray-500 dark:bg-dark-900 dark:text-dark-400;
}

.store-frame {
  @apply block h-full w-full border-0;
}

@media (max-width: 640px) {
  .store-toolbar {
    @apply items-start;
  }

  .store-service-notice {
    @apply items-start;
  }
}
</style>
