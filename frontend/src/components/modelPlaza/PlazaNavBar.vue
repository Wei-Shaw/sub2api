<template>
  <!--
    Standalone-mode header. Same lockup as the landing page and the key-usage
    page: logo, site name, one action, and a single hairline under the row.
    The CTA was a gradient-filled, glow-shadowed pill; it is the Button
    primitive now, which is the only accent-filled control on the page.
  -->
  <header class="sticky top-0 z-30 border-b border-line bg-surface">
    <div class="mx-auto flex max-w-6xl items-center justify-between gap-4 px-6 py-3">
      <div class="flex min-w-0 items-center gap-3">
        <template v-if="settings">
          <img
            :src="siteLogo || '/logo.svg'"
            alt=""
            class="h-7 w-7 shrink-0 rounded object-contain"
          />
          <span class="min-w-0 truncate text-sm font-semibold text-ink [overflow-wrap:anywhere]">
            {{ siteName }}
          </span>
        </template>
        <template v-else>
          <span class="skeleton h-7 w-7 shrink-0 rounded" aria-hidden="true"></span>
          <span class="skeleton h-4 w-28 rounded" aria-hidden="true"></span>
        </template>
      </div>

      <Button
        v-if="isAuthenticated"
        :to="backTarget"
        tone="accent"
        variant="solid"
        size="md"
        class="shrink-0"
        data-testid="plaza-nav-dashboard"
      >
        {{ t('modelPlaza.nav.backToDashboard') }}
      </Button>
      <Button
        v-else
        :to="{ path: '/login', query: { redirect: '/model-plaza' } }"
        tone="accent"
        variant="solid"
        size="md"
        class="shrink-0"
        data-testid="plaza-nav-login"
      >
        {{ t('modelPlaza.nav.login') }}
      </Button>
    </div>
  </header>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Button from '@/components/common/Button.vue'
import { sanitizeUrl } from '@/utils/url'
import { useAppStore } from '@/stores/app'
import { useAuthStore } from '@/stores/auth'

const { t } = useI18n()
const appStore = useAppStore()
const authStore = useAuthStore()

const settings = computed(() => appStore.cachedPublicSettings)
const siteName = computed(() => settings.value?.site_name || 'Sub2API')
const siteLogo = computed(() =>
  sanitizeUrl(settings.value?.site_logo || '', { allowRelative: true, allowDataUrl: true })
)
const isAuthenticated = computed(() => authStore.isAuthenticated)
const backTarget = computed(() => (authStore.isAdmin ? '/admin/dashboard' : '/dashboard'))
</script>
