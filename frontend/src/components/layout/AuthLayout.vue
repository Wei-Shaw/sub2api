<template>
  <div class="flex min-h-screen items-center justify-center bg-surface-sunken p-4 sm:p-8">
    <div class="w-full max-w-[440px]">
      <!-- Logo/Brand -->
      <div class="border border-border bg-surface" style="border-top: 4px solid rgb(var(--accent))">
        <!-- 票头：运营方品牌 -->
        <div class="flex min-h-[4.5rem] items-center gap-3 border-b border-border px-6 py-4 sm:px-8">
          <template v-if="settingsLoaded">
            <div class="flex h-10 w-10 shrink-0 items-center justify-center overflow-hidden">
              <img :src="siteLogo || '/logo.svg'" alt="" class="h-full w-full object-contain" />
            </div>
            <div class="min-w-0">
              <h1 class="truncate text-h2 font-bold text-accent-strong">
                {{ siteName }}
              </h1>
              <p class="truncate text-meta text-fg-muted">
                {{ siteSubtitle }}
              </p>
            </div>
          </template>
          <div class="ml-auto shrink-0">
            <LocaleSwitcher />
          </div>
        </div>

        <div class="p-6 sm:p-8">
          <slot />
        </div>
      </div>

      <!-- Footer Links -->
      <div class="mt-6 text-center text-body">
        <slot name="footer" />
      </div>

      <!-- Copyright -->
      <div class="mt-8 text-center text-meta text-fg-subtle">
        &copy; {{ currentYear }} {{ siteName }}. {{ t('home.footer.allRightsReserved') }}
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores'
import LocaleSwitcher from '@/components/common/LocaleSwitcher.vue'
import { sanitizeUrl } from '@/utils/url'

const { t } = useI18n()
const appStore = useAppStore()

const siteName = computed(() => appStore.siteName || 'Sub2API')
const siteLogo = computed(() => sanitizeUrl(appStore.siteLogo || '', { allowRelative: true, allowDataUrl: true }))
const siteSubtitle = computed(() => appStore.cachedPublicSettings?.site_subtitle || 'Subscription to API Conversion Platform')
const settingsLoaded = computed(() => appStore.publicSettingsLoaded)

const currentYear = computed(() => new Date().getFullYear())

onMounted(() => {
  appStore.fetchPublicSettings()
})
</script>
