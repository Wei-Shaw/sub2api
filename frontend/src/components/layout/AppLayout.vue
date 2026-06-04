<template>
  <div class="cosmic-shell min-h-screen">
    <!-- 星空与鼠标黑洞透镜背景 -->
    <div class="cosmic-backdrop cosmic-backdrop--fixed cosmic-backdrop--subtle">
      <div class="starfield starfield--subtle">
        <div class="stars-sm"></div>
        <div class="stars-md"></div>
        <div class="stars-lg"></div>
      </div>
      <div class="meteors">
        <i class="meteor" style="--a: 75deg; --dur: 7s; --delay: 1.6s"></i>
        <i class="meteor" style="--a: 205deg; --dur: 8s; --delay: 4.4s"></i>
      </div>
    </div>

    <!-- Sidebar -->
    <AppSidebar />

    <!-- Main Content Area -->
    <div
      class="cosmic-distortion-surface relative min-h-screen transition-all duration-300"
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
import { computed, onMounted } from 'vue'
import { useAppStore } from '@/stores'
import { useAuthStore } from '@/stores/auth'
import { useOnboardingTour } from '@/composables/useOnboardingTour'
import { useOnboardingStore } from '@/stores/onboarding'
import AppSidebar from './AppSidebar.vue'
import AppHeader from './AppHeader.vue'

const appStore = useAppStore()
const authStore = useAuthStore()
const sidebarCollapsed = computed(() => appStore.sidebarCollapsed)
const isAdmin = computed(() => authStore.user?.role === 'admin')

const { replayTour } = useOnboardingTour({
  storageKey: isAdmin.value ? 'admin_guide' : 'user_guide',
  autoStart: true
})

const onboardingStore = useOnboardingStore()

onMounted(() => {
  onboardingStore.setReplayCallback(replayTour)
})

defineExpose({ replayTour })
</script>
