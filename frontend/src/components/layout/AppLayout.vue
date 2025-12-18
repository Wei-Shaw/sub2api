<template>
  <div class="min-h-screen bg-gray-50 dark:bg-dark-950">
    <!-- Background Decoration -->
    <div class="fixed inset-0 bg-mesh-gradient pointer-events-none"></div>

    <!-- Sidebar -->
    <AppSidebar />

    <!-- Main Content Area -->
    <div
      class="relative min-h-screen transition-all duration-300"
      :class="[
        sidebarCollapsed ? 'lg:ml-[72px]' : 'lg:ml-64',
      ]"
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
import { computed REDACTED from 'vue';
import { useAppStore REDACTED from '@/stores';
import AppSidebar from './AppSidebar.vue';
import AppHeader from './AppHeader.vue';

const appStore = useAppStore();
const sidebarCollapsed = computed(() => appStore.sidebarCollapsed);
</script>

