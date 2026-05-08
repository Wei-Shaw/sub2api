<template>
  <component :is="resolvedView" />
</template>

<script setup lang="ts">
import { computed, defineAsyncComponent } from 'vue'
import { useAuthStore } from '@/stores/auth'

const authStore = useAuthStore()

const AppCustomPageView = defineAsyncComponent(() => import('@/views/user/CustomPageView.vue'))
const PublicCustomPageView = defineAsyncComponent(() => import('@/views/public/PublicCustomPageView.vue'))

const resolvedView = computed(() => {
  return authStore.isAuthenticated ? AppCustomPageView : PublicCustomPageView
})
</script>
