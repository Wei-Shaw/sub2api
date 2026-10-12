<template>
  <div class="app-shell min-h-screen">
    <AppHeader />
    <AppSidebar />

    <div
      class="app-main relative min-h-[calc(100vh-48px)] transition-[margin] duration-200"
      :class="[sidebarCollapsed ? 'lg:ml-11' : 'lg:ml-52']"
    >
      <main class="app-content">
        <div class="newapi-page-heading">
          <div class="min-w-0">
            <h1>{{ pageTitle }}</h1>
            <p v-if="pageDescription">{{ pageDescription }}</p>
          </div>
        </div>
        <div class="newapi-page-body">
          <slot />
        </div>
      </main>
    </div>
  </div>
</template>

<script setup lang="ts">
import '@/styles/onboarding.css'
import { computed, onMounted } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores'
import { useAuthStore } from '@/stores/auth'
import { useOnboardingTour } from '@/composables/useOnboardingTour'
import { useOnboardingStore } from '@/stores/onboarding'
import AppSidebar from './AppSidebar.vue'
import AppHeader from './AppHeader.vue'

const appStore = useAppStore()
const authStore = useAuthStore()
const route = useRoute()
const { t } = useI18n()
const sidebarCollapsed = computed(() => appStore.sidebarCollapsed)
const isAdmin = computed(() => authStore.user?.role === 'admin')
const localPreviewMode = import.meta.env.VITE_LOCAL_PREVIEW === 'true'
const pageTitle = computed(() => {
  const key = route.meta.titleKey as string | undefined
  if (key) {
    const translated = t(key)
    if (translated !== key) return translated
  }
  return String(route.meta.title || '')
})
const pageDescription = computed(() => {
  const key = route.meta.descriptionKey as string | undefined
  if (!key) return ''
  const translated = t(key)
  return translated === key ? '' : translated
})

const { replayTour } = useOnboardingTour({
  storageKey: isAdmin.value ? 'admin_guide' : 'user_guide',
  autoStart: !localPreviewMode
})

const onboardingStore = useOnboardingStore()

onMounted(() => {
  onboardingStore.setReplayCallback(replayTour)
})

defineExpose({ replayTour })
</script>
