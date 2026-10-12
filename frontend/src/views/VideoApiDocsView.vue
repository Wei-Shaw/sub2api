<template>
  <AppLayout v-if="isEmbedded">
    <main class="mx-auto max-w-5xl px-4 py-6 sm:px-6 lg:px-8 lg:py-10">
      <VideoApiDocsContent />
    </main>
  </AppLayout>
  <div v-else class="min-h-screen bg-gray-50 dark:bg-dark-950">
    <PlazaNavBar />
    <main class="mx-auto max-w-5xl px-4 py-6 sm:px-6 lg:px-8 lg:py-10">
      <VideoApiDocsContent />
    </main>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import AppLayout from '@/components/layout/AppLayout.vue'
import { useAuthStore } from '@/stores/auth'
import PlazaNavBar from '@/components/modelPlaza/PlazaNavBar.vue'
import VideoApiDocsContent from '@/components/videoPlaza/VideoApiDocsContent.vue'

const route = useRoute()
const authStore = useAuthStore()
const isEmbedded = computed(() => route.query.embedded === '1' && authStore.isAuthenticated)
</script>
