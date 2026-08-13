<template>
  <div class="min-h-screen bg-canvas">
    <!--
      The decorative `bg-mesh-gradient` overlay that used to sit here is gone.
      Three radial teal blobs behind every screen in the product was the single
      loudest AI-slop signal in the tree; the canvas is flat now.
    -->
    <AppSidebar />

    <!--
      Gutter is driven from the same custom properties the sidebar sizes itself
      from, so the two can never drift. Previously each side hardcoded its own
      magic number (72px / 16rem).
    -->
    <div
      class="relative min-h-screen transition-[margin] duration-slow ease-out lg:ms-[--gutter]"
      :style="{ '--gutter': gutter }"
    >
      <AppHeader />

      <!--
        The measure lives here, once, instead of every migrated view
        re-declaring `mx-auto max-w-6xl`.
      -->
      <main class="mx-auto w-full max-w-[1440px] px-4 py-4 md:px-6 md:py-6 lg:px-page lg:py-8">
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

// Single source of truth for the sidebar width, shared with `.sidebar` in
// style.css. Only applied from `lg` up; below that the sidebar is off-canvas.
const gutter = computed(() =>
  sidebarCollapsed.value ? 'var(--ds-sidebar-w-collapsed)' : 'var(--ds-sidebar-w)'
)

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
