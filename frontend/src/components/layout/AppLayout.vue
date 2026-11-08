<template>
  <div class="min-h-screen bg-gray-50 dark:bg-dark-950">
    <!-- Background Decoration -->
    <div class="pointer-events-none fixed inset-0 bg-mesh-gradient"></div>

    <!-- Sidebar：只有「工具型页面」才关掉（例如电商改图）。
         这类页面要的是整屏画布，套着平台导航会白白吃掉两侧空间。 -->
    <AppSidebar v-if="showSidebar" />

    <!-- Main Content Area -->
    <div
      class="relative min-h-screen transition-all duration-300"
      :class="showSidebar ? [sidebarCollapsed ? 'lg:ml-[72px]' : 'lg:ml-64'] : ''"
    >
      <!-- Header -->
      <AppHeader v-if="showHeader" />

      <!-- Main Content -->
      <main :class="showSidebar && showHeader ? 'p-4 md:p-6 lg:p-8' : 'p-0'">
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

// 工具型页面（电商改图）用这两个开关把平台导航收起来，换成整屏画布。
// 默认都为 true，其它页面行为完全不变。
withDefaults(defineProps<{ showSidebar?: boolean; showHeader?: boolean }>(), {
  showSidebar: true,
  showHeader: true,
})

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
