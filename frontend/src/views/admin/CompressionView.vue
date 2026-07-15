<template>
  <AppLayout>
  <div class="space-y-6 pb-12">
    <div class="flex flex-wrap items-center justify-between gap-3">
      <div>
        <h1 class="text-2xl font-semibold text-gray-900 dark:text-white">Prompt Compression / RTK</h1>
        <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">Safe tool-output compression with observe, enforce and emergency stop controls.</p>
      </div>
      <div class="flex gap-2">
        <button class="rounded-lg bg-gray-100 px-3 py-2 text-sm dark:bg-dark-800" @click="refresh">Refresh</button>
        <button v-if="status?.runtime.emergency_stopped" class="rounded-lg bg-emerald-600 px-3 py-2 text-sm text-white" @click="resume">Resume</button>
        <button v-else class="rounded-lg bg-red-600 px-3 py-2 text-sm text-white" @click="stop">Emergency stop</button>
      </div>
    </div>

    <div v-if="error" class="rounded-xl bg-red-50 p-4 text-sm text-red-700 dark:bg-red-900/20 dark:text-red-300">{{ error }}</div>
    <div v-if="status" class="grid gap-4 md:grid-cols-4">
      <div v-for="card in cards" :key="card.label" class="rounded-2xl border border-gray-200 bg-white p-5 dark:border-dark-700 dark:bg-dark-900">
        <div class="text-xs uppercase tracking-wide text-gray-500">{{ card.label }}</div>
        <div class="mt-2 text-xl font-semibold text-gray-900 dark:text-white">{{ card.value }}</div>
      </div>
    </div>

    <div v-if="status" class="grid gap-6 lg:grid-cols-2">
      <section class="rounded-2xl border border-gray-200 bg-white p-5 dark:border-dark-700 dark:bg-dark-900">
        <h2 class="text-lg font-medium text-gray-900 dark:text-white">Activation</h2>
        <div class="mt-4 space-y-4">
          <label class="flex items-center justify-between gap-4 text-sm text-gray-700 dark:text-gray-300">
            Deployment enabled
            <input v-model="draft.enabled" type="checkbox" :disabled="!status.deployment_enabled" />
          </label>
          <label class="block text-sm text-gray-700 dark:text-gray-300">Mode
            <select v-model="draft.mode" class="mt-1 w-full rounded-lg border border-gray-300 bg-transparent px-3 py-2 dark:border-dark-600">
              <option value="off">off</option><option value="observe">observe</option><option value="enforce">enforce</option>
            </select>
          </label>
          <label class="block text-sm text-gray-700 dark:text-gray-300">Intensity
            <select v-model="draft.intensity" class="mt-1 w-full rounded-lg border border-gray-300 bg-transparent px-3 py-2 dark:border-dark-600">
              <option value="safe">safe</option><option value="balanced">balanced</option><option value="aggressive">aggressive</option>
            </select>
          </label>
          <label class="block text-sm text-gray-700 dark:text-gray-300">Rollout: {{ draft.rollout_percent }}%
            <input v-model.number="draft.rollout_percent" type="range" min="0" max="100" class="mt-2 w-full" />
          </label>
          <label class="block text-sm text-gray-700 dark:text-gray-300">Minimum savings tokens
            <input v-model.number="draft.min_savings_tokens" type="number" min="0" class="mt-1 w-full rounded-lg border border-gray-300 bg-transparent px-3 py-2 dark:border-dark-600" />
          </label>
          <button class="rounded-lg bg-primary-600 px-4 py-2 text-sm text-white disabled:opacity-50" :disabled="saving" @click="save">{{ saving ? 'Saving…' : 'Save policy' }}</button>
        </div>
      </section>
      <section class="rounded-2xl border border-gray-200 bg-white p-5 dark:border-dark-700 dark:bg-dark-900">
        <h2 class="text-lg font-medium text-gray-900 dark:text-white">Telemetry</h2>
        <dl class="mt-4 grid grid-cols-2 gap-4 text-sm">
          <div><dt class="text-gray-500">Applied</dt><dd class="text-xl font-semibold">{{ status.telemetry.applied }}</dd></div>
          <div><dt class="text-gray-500">Skipped</dt><dd class="text-xl font-semibold">{{ status.telemetry.skipped }}</dd></div>
          <div><dt class="text-gray-500">Failed</dt><dd class="text-xl font-semibold">{{ status.telemetry.failed }}</dd></div>
          <div><dt class="text-gray-500">Dropped</dt><dd class="text-xl font-semibold">{{ status.telemetry.dropped }}</dd></div>
        </dl>
        <p class="mt-5 text-xs text-gray-500">RTK never changes billing usage. It is disabled unless the deployment hard switch is enabled.</p>
      </section>
    </div>
  </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import compressionAPI, { type CompressionStatus } from '@/api/admin/compression'
import AppLayout from '@/components/layout/AppLayout.vue'

const status = ref<CompressionStatus | null>(null)
const error = ref('')
const saving = ref(false)
const draft = reactive({ enabled: false, mode: 'off' as 'off' | 'observe' | 'enforce', intensity: 'balanced' as 'safe' | 'balanced' | 'aggressive', rollout_percent: 0, min_savings_tokens: 64 })
const cards = computed(() => status.value ? [
  { label: 'Deployment', value: status.value.deployment_enabled ? 'Enabled' : 'Disabled' },
  { label: 'Mode', value: status.value.mode },
  { label: 'Engine', value: status.value.engine_available ? 'Ready' : 'Unavailable' },
  { label: 'Revision', value: String(status.value.policy.revision) },
] : [])

async function refresh() {
  error.value = ''
  try {
    status.value = await compressionAPI.status()
    draft.enabled = status.value.policy.enabled
    draft.mode = status.value.policy.mode
    draft.intensity = status.value.policy.intensity
    draft.rollout_percent = status.value.policy.rollout_percent
    draft.min_savings_tokens = status.value.policy.min_savings_tokens
  } catch (e) { error.value = e instanceof Error ? e.message : 'Failed to load RTK status' }
}
async function save() {
  saving.value = true; error.value = ''
  try { await compressionAPI.update(draft); await refresh() } catch (e) { error.value = e instanceof Error ? e.message : 'Failed to save RTK policy' } finally { saving.value = false }
}
async function stop() { try { await compressionAPI.emergencyStop('Stopped from RTK admin console'); await refresh() } catch (e) { error.value = e instanceof Error ? e.message : 'Failed to stop RTK' } }
async function resume() { try { await compressionAPI.resume('Resumed from RTK admin console'); await refresh() } catch (e) { error.value = e instanceof Error ? e.message : 'Failed to resume RTK' } }
onMounted(refresh)
</script>
