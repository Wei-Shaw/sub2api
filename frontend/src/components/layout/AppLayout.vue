<template>
  <div class="min-h-screen bg-gray-50 dark:bg-dark-950">
    <!-- Background Decoration -->
    <div class="pointer-events-none fixed inset-0 bg-mesh-gradient"></div>

    <!-- Sidebar -->
    <AppSidebar />

    <!-- Main Content Area -->
    <div
      class="relative min-h-screen transition-all duration-300"
      :class="[sidebarCollapsed ? 'lg:ml-[72px]' : 'lg:ml-64']"
    >
      <!-- Header -->
      <AppHeader />

      <!-- Main Content -->
      <main class="p-4 md:p-6 lg:p-8">
        <slot />
      </main>
    </div>
  </div>
</template>

<script setup lang="ts">
import '@/styles/onboarding.css'
import { computed, onMounted REDACTED from 'vue'
import { useI18n REDACTED from 'vue-i18n'
import { useAppStore REDACTED from '@/stores'
import { useAuthStore REDACTED from '@/stores/auth'
import { useOnboardingTour REDACTED from '@/composables/useOnboardingTour'
import { getAdminSteps, getUserSteps REDACTED from '@/components/Guide/steps'
import { useOnboardingStore REDACTED from '@/stores/onboarding'
import AppSidebar from './AppSidebar.vue'
import AppHeader from './AppHeader.vue'

const appStore = useAppStore()
const authStore = useAuthStore()
const { t REDACTED = useI18n()
const sidebarCollapsed = computed(() => appStore.sidebarCollapsed)
const isAdmin = computed(() => authStore.user?.role === 'admin')

const { replayTour REDACTED = useOnboardingTour({
  steps: isAdmin.value ? getAdminSteps(t) : getUserSteps(t),
  storageKey: isAdmin.value ? 'admin_guide' : 'user_guide',
  autoStart: true
REDACTED)

const onboardingStore = useOnboardingStore()

onMounted(() => {
  onboardingStore.setReplayCallback(replayTour)
REDACTED)

defineExpose({ replayTour REDACTED)
</script>
