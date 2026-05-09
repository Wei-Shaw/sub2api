<template>
  <component :is="resolvedView" />
</template>

<script setup lang="ts">
import { computed, defineAsyncComponent } from 'vue'
import { useAuthStore } from '@/stores/auth'

const authStore = useAuthStore()
const AppTutorialDocumentView = defineAsyncComponent(() => import('@/views/user/TutorialDocumentView.vue'))
const PublicTutorialDocumentView = defineAsyncComponent(() => import('@/views/public/TutorialDocumentView.vue'))

const resolvedView = computed(() => {
  return authStore.isAuthenticated ? AppTutorialDocumentView : PublicTutorialDocumentView
})
</script>
