<template>
  <AppLayout v-if="isEmbedded">
    <VideoModelPlazaContent :groups="groups" :loading="loading" :error="loadFailed" embedded />
  </AppLayout>
  <div v-else class="min-h-screen bg-gray-50 dark:bg-dark-950">
    <PlazaNavBar />
    <main class="mx-auto max-w-7xl px-4 py-6 sm:px-6 lg:px-8 lg:py-8">
      <VideoModelPlazaContent :groups="groups" :loading="loading" :error="loadFailed" />
    </main>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import AppLayout from '@/components/layout/AppLayout.vue'
import PlazaNavBar from '@/components/modelPlaza/PlazaNavBar.vue'
import VideoModelPlazaContent from '@/components/videoPlaza/VideoModelPlazaContent.vue'
import { getVideoModelPlaza, type VideoPlazaGroup } from '@/api/videoModelPlaza'
import { useAuthStore } from '@/stores/auth'

const route = useRoute()
const authStore = useAuthStore()
const isEmbedded = computed(() => route.query.embedded === '1' && authStore.isAuthenticated)
const groups = ref<VideoPlazaGroup[]>([])
const loading = ref(true)
const loadFailed = ref(false)

onMounted(async () => {
  try {
    groups.value = (await getVideoModelPlaza()).groups
  } catch {
    loadFailed.value = true
  } finally {
    loading.value = false
  }
})
</script>
