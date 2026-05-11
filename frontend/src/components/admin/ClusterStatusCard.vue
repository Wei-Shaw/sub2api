<template>
  <div class="card">
    <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
      <div class="flex items-center justify-between">
        <div>
          <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
            {{ t('admin.cluster.title') }}
          </h2>
          <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
            {{ t('admin.cluster.description') }}
          </p>
        </div>
        <button
          type="button"
          @click="refreshStatus"
          :disabled="loading"
          class="btn btn-secondary btn-sm"
        >
          <Icon
            name="refresh"
            size="sm"
            :class="{ 'animate-spin': loading }"
          />
        </button>
      </div>
    </div>

    <div class="p-6">
      <!-- Loading State -->
      <div v-if="loading && !status" class="flex items-center gap-2 text-gray-500">
        <div class="h-4 w-4 animate-spin rounded-full border-b-2 border-primary-600"></div>
        {{ t('common.loading') }}
      </div>

      <!-- Cluster Status -->
      <div v-else-if="status" class="space-y-6">
        <!-- Overview Stats -->
        <div class="grid grid-cols-2 gap-4 sm:grid-cols-4">
          <div class="rounded-lg bg-gray-50 p-4 dark:bg-dark-700">
            <p class="text-sm font-medium text-gray-500 dark:text-gray-400">
              {{ t('admin.cluster.totalInstances') }}
            </p>
            <p class="mt-1 text-2xl font-semibold text-gray-900 dark:text-white">
              {{ status.total_instances }}
            </p>
          </div>
          <div class="rounded-lg bg-gray-50 p-4 dark:bg-dark-700">
            <p class="text-sm font-medium text-gray-500 dark:text-gray-400">
              {{ t('admin.cluster.masterSlaveCount') }}
            </p>
            <p class="mt-1 text-2xl font-semibold text-gray-900 dark:text-white">
              {{ status.master_count }} / {{ status.slave_count }}
            </p>
          </div>
          <div class="rounded-lg bg-gray-50 p-4 dark:bg-dark-700">
            <p class="text-sm font-medium text-gray-500 dark:text-gray-400">
              {{ t('admin.cluster.clusterMode') }}
            </p>
            <p class="mt-1 text-lg font-semibold" :class="status.cluster_enabled ? 'text-green-600 dark:text-green-400' : 'text-gray-500'">
              {{ status.cluster_enabled ? t('admin.cluster.enabled') : t('admin.cluster.disabled') }}
            </p>
          </div>
          <div class="rounded-lg bg-gray-50 p-4 dark:bg-dark-700">
            <p class="text-sm font-medium text-gray-500 dark:text-gray-400">
              {{ t('admin.cluster.clusterHealth') }}
            </p>
            <div class="mt-1 flex items-center gap-2">
              <span
                class="h-3 w-3 rounded-full"
                :class="status.cluster_healthy ? 'bg-green-500' : 'bg-red-500'"
              ></span>
              <span class="text-lg font-semibold" :class="status.cluster_healthy ? 'text-green-600 dark:text-green-400' : 'text-red-600 dark:text-red-400'">
                {{ status.cluster_healthy ? t('admin.cluster.healthy') : t('admin.cluster.unhealthy') }}
              </span>
            </div>
          </div>
        </div>

        <!-- Current Instance Info -->
        <div v-if="status.current_instance" class="rounded-lg border border-primary-200 bg-primary-50 p-4 dark:border-primary-800 dark:bg-primary-900/20">
          <div class="flex items-center gap-2">
            <Icon name="server" size="md" class="text-primary-600 dark:text-primary-400" />
            <span class="font-medium text-primary-700 dark:text-primary-300">
              {{ t('admin.cluster.currentInstance') }}
            </span>
            <span
              class="rounded-full px-2 py-0.5 text-xs font-medium"
              :class="roleClass(status.current_instance.role)"
            >
              {{ roleLabel(status.current_instance.role) }}
            </span>
          </div>
          <div class="mt-2 grid grid-cols-2 gap-4 text-sm sm:grid-cols-4">
            <div>
              <span class="text-gray-500 dark:text-gray-400">{{ t('admin.cluster.instanceId') }}:</span>
              <span class="ml-1 font-mono text-gray-900 dark:text-white">{{ truncateId(status.current_instance.id) }}</span>
            </div>
            <div>
              <span class="text-gray-500 dark:text-gray-400">{{ t('admin.cluster.hostname') }}:</span>
              <span class="ml-1 text-gray-900 dark:text-white">{{ status.current_instance.hostname }}</span>
            </div>
            <div v-if="status.current_instance.ip">
              <span class="text-gray-500 dark:text-gray-400">{{ t('admin.cluster.ip') }}:</span>
              <span class="ml-1 text-gray-900 dark:text-white">{{ status.current_instance.ip }}</span>
            </div>
            <div v-if="status.current_instance.version">
              <span class="text-gray-500 dark:text-gray-400">{{ t('admin.cluster.version') }}:</span>
              <span class="ml-1 text-gray-900 dark:text-white">{{ status.current_instance.version }}</span>
            </div>
          </div>
        </div>

        <!-- Instance List -->
        <div v-if="status.instances.length > 0">
          <h3 class="mb-3 text-sm font-medium text-gray-700 dark:text-gray-300">
            {{ t('admin.cluster.allInstances') }}
          </h3>
          <div class="overflow-x-auto">
            <table class="w-full text-sm">
              <thead>
                <tr class="border-b border-gray-200 dark:border-dark-600">
                  <th class="pb-2 text-left font-medium text-gray-500 dark:text-gray-400">
                    {{ t('admin.cluster.instanceId') }}
                  </th>
                  <th class="pb-2 text-left font-medium text-gray-500 dark:text-gray-400">
                    {{ t('admin.cluster.hostname') }}
                  </th>
                  <th class="pb-2 text-left font-medium text-gray-500 dark:text-gray-400">
                    {{ t('admin.cluster.status') }}
                  </th>
                  <th class="pb-2 text-left font-medium text-gray-500 dark:text-gray-400">
                    {{ t('admin.cluster.role') }}
                  </th>
                  <th class="pb-2 text-left font-medium text-gray-500 dark:text-gray-400">
                    {{ t('admin.cluster.lastHeartbeat') }}
                  </th>
                  <th class="pb-2 text-left font-medium text-gray-500 dark:text-gray-400">
                    {{ t('admin.cluster.runningTasks') }}
                  </th>
                </tr>
              </thead>
              <tbody>
                <tr
                  v-for="instance in status.instances"
                  :key="instance.id"
                  class="border-b border-gray-100 dark:border-dark-700"
                  :class="{ 'bg-primary-50/50 dark:bg-primary-900/10': instance.id === status.current_instance?.id }"
                >
                  <td class="py-3 font-mono text-gray-900 dark:text-white">
                    {{ truncateId(instance.id) }}
                  </td>
                  <td class="py-3 text-gray-900 dark:text-white">
                    {{ instance.hostname }}
                  </td>
                  <td class="py-3">
                    <span
                      class="inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-xs font-medium"
                      :class="statusClass(instance.status)"
                    >
                      <span class="h-1.5 w-1.5 rounded-full" :class="statusDotClass(instance.status)"></span>
                      {{ instance.status }}
                    </span>
                  </td>
                  <td class="py-3">
                    <span
                      class="rounded-full px-2 py-0.5 text-xs font-medium"
                      :class="roleClass(instance.role)"
                    >
                      {{ roleLabel(instance.role) }}
                    </span>
                  </td>
                  <td class="py-3 text-gray-500 dark:text-gray-400">
                    {{ formatTime(instance.last_heartbeat) }}
                  </td>
                  <td class="py-3">
                    <div v-if="instance.running_tasks?.length" class="flex flex-wrap gap-1">
                      <span
                        v-for="task in instance.running_tasks.slice(0, 3)"
                        :key="task"
                        class="rounded bg-gray-100 px-1.5 py-0.5 text-xs text-gray-600 dark:bg-dark-600 dark:text-gray-300"
                      >
                        {{ task }}
                      </span>
                      <span
                        v-if="instance.running_tasks.length > 3"
                        class="text-xs text-gray-500"
                      >
                        +{{ instance.running_tasks.length - 3 }}
                      </span>
                    </div>
                    <span v-else class="text-gray-400">-</span>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>

        <!-- No Instances (single mode) -->
        <div v-else class="rounded-lg border border-gray-200 bg-gray-50 p-4 text-center dark:border-dark-600 dark:bg-dark-700">
          <Icon name="server" size="lg" class="mx-auto text-gray-400" />
          <p class="mt-2 text-sm text-gray-500 dark:text-gray-400">
            {{ t('admin.cluster.singleInstanceMode') }}
          </p>
        </div>
      </div>

      <!-- Error State -->
      <div v-else class="text-center text-gray-500">
        {{ t('admin.cluster.loadError') }}
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { clusterAPI, type ClusterStatusResponse } from '@/api/admin/cluster'
import Icon from '@/components/icons/Icon.vue'

const { t } = useI18n()

const loading = ref(false)
const status = ref<ClusterStatusResponse | null>(null)
let refreshInterval: ReturnType<typeof setInterval> | null = null

const healthyColor = computed(() => {
  if (!status.value) return 'text-gray-500'
  if (status.value.healthy_count === status.value.total_instances) {
    return 'text-green-600 dark:text-green-400'
  } else if (status.value.healthy_count > 0) {
    return 'text-amber-600 dark:text-amber-400'
  }
  return 'text-red-600 dark:text-red-400'
})

function truncateId(id: string): string {
  if (id.length <= 12) return id
  return id.slice(0, 8) + '...'
}

function statusClass(status: string): string {
  switch (status) {
    case 'healthy':
      return 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400'
    case 'unhealthy':
      return 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400'
    default:
      return 'bg-gray-100 text-gray-700 dark:bg-dark-600 dark:text-gray-400'
  }
}

function statusDotClass(status: string): string {
  switch (status) {
    case 'healthy':
      return 'bg-green-500'
    case 'unhealthy':
      return 'bg-red-500'
    default:
      return 'bg-gray-400'
  }
}

function roleClass(role: string): string {
  switch (role) {
    case 'master':
      return 'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-400'
    case 'slave':
      return 'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400'
    default:
      return 'bg-gray-100 text-gray-700 dark:bg-dark-600 dark:text-gray-400'
  }
}

function roleLabel(role: string): string {
  switch (role) {
    case 'master':
      return t('admin.cluster.master')
    case 'slave':
      return t('admin.cluster.slave')
    default:
      return t('admin.cluster.standalone')
  }
}

function formatTime(isoString: string): string {
  if (!isoString) return '-'
  const date = new Date(isoString)
  const now = new Date()
  const diffMs = now.getTime() - date.getTime()
  const diffSec = Math.floor(diffMs / 1000)
  
  if (diffSec < 60) return `${diffSec}s ago`
  if (diffSec < 3600) return `${Math.floor(diffSec / 60)}m ago`
  if (diffSec < 86400) return `${Math.floor(diffSec / 3600)}h ago`
  return date.toLocaleString()
}

async function refreshStatus() {
  loading.value = true
  try {
    status.value = await clusterAPI.getClusterStatus()
  } catch (error) {
    console.error('Failed to fetch cluster status:', error)
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  refreshStatus()
  // Auto-refresh every 10 seconds
  refreshInterval = setInterval(refreshStatus, 10000)
})

onUnmounted(() => {
  if (refreshInterval) {
    clearInterval(refreshInterval)
  }
})
</script>
