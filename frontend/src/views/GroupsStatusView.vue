<template>
  <div class="min-h-screen bg-[#f5f7fc] dark:bg-dark-950">
    <PlazaNavBar redirect-path="/groups-status" />
    <main class="mx-auto max-w-[1480px] px-4 py-5 sm:px-6 lg:px-8 lg:py-7">
      <GroupsStatusContent
        :response="data"
        :loading="loading"
        :error="loadFailed"
        :last-updated-at="lastUpdatedAt"
        @retry="loadStatus"
      />
    </main>
  </div>
</template>

<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import PlazaNavBar from '@/components/modelPlaza/PlazaNavBar.vue'
import GroupsStatusContent from '@/components/groupsStatus/GroupsStatusContent.vue'
import { getGroupsStatus, type GroupsStatusResponse } from '@/api/groupsStatus'
import { useAppStore } from '@/stores/app'

const appStore = useAppStore()
const data = ref<GroupsStatusResponse | null>(null)
const loading = ref(true)
const loadFailed = ref(false)
const lastUpdatedAt = ref<Date | null>(null)
let activeController: AbortController | null = null

async function loadStatus(): Promise<void> {
  activeController?.abort()
  const requestController = new AbortController()
  activeController = requestController
  loading.value = true
  loadFailed.value = false
  try {
    const nextData = await getGroupsStatus({ signal: requestController.signal })
    if (activeController !== requestController || requestController.signal.aborted) return
    data.value = nextData
    lastUpdatedAt.value = new Date()
  } catch (error) {
    if (activeController !== requestController || requestController.signal.aborted) return
    console.error('Failed to load public group status:', error)
    loadFailed.value = true
  } finally {
    if (activeController === requestController) {
      loading.value = false
      activeController = null
    }
  }
}

onMounted(() => {
  void appStore.fetchPublicSettings()
  void loadStatus()
})

onBeforeUnmount(() => {
  activeController?.abort()
  activeController = null
})
</script>
