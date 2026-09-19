<script setup lang="ts">
export interface APIKeyExtraDraft {
  id: string
  label: string
  weight: number
  enabled: boolean
  key: string
}

const strategy = defineModel<'round_robin' | 'weighted'>('strategy', { required: true })
const primaryWeight = defineModel<number>('primaryWeight', { required: true })
const extras = defineModel<APIKeyExtraDraft[]>('extras', { required: true })

defineProps<{
  mode: 'create' | 'edit'
}>()

function addExtra() {
  extras.value = [
    ...extras.value,
    {
      id: `k${Date.now().toString(36)}${Math.random().toString(36).slice(2, 6)}`,
      label: '',
      weight: 1,
      enabled: true,
      key: ''
    }
  ]
}

function removeExtra(id: string) {
  extras.value = extras.value.filter((item) => item.id !== id)
}
</script>

<template>
  <div class="space-y-3 rounded-lg border border-gray-200 p-3 dark:border-dark-600" data-testid="api-key-pool">
    <div class="flex items-center justify-between gap-3">
      <div>
        <p class="text-sm font-medium text-gray-800 dark:text-gray-200">{{ $t('admin.accounts.apiKeyPool.title') }}</p>
        <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">{{ $t('admin.accounts.apiKeyPool.hint') }}</p>
      </div>
      <button type="button" class="btn btn-secondary btn-sm" data-testid="api-key-pool-add" @click="addExtra">
        {{ $t('admin.accounts.apiKeyPool.add') }}
      </button>
    </div>

    <div v-if="extras.length > 0" class="grid gap-3 sm:grid-cols-2">
      <div>
        <label class="input-label">{{ $t('admin.accounts.apiKeyPool.strategy') }}</label>
        <select v-model="strategy" class="input" data-testid="api-key-pool-strategy">
          <option value="round_robin">{{ $t('admin.accounts.apiKeyPool.roundRobin') }}</option>
          <option value="weighted">{{ $t('admin.accounts.apiKeyPool.weighted') }}</option>
        </select>
      </div>
      <div v-if="strategy === 'weighted'">
        <label class="input-label">{{ $t('admin.accounts.apiKeyPool.primaryWeight') }}</label>
        <input v-model.number="primaryWeight" type="number" min="1" max="10000" class="input" />
      </div>
    </div>

    <div v-for="item in extras" :key="item.id" class="grid gap-2 rounded-md bg-gray-50 p-3 dark:bg-dark-800 sm:grid-cols-12">
      <div class="sm:col-span-4">
        <label class="input-label">{{ $t('admin.accounts.apiKeyPool.key') }}</label>
        <input
          v-model="item.key"
          type="password"
          autocomplete="new-password"
          class="input font-mono"
          :placeholder="mode === 'edit' ? $t('admin.accounts.leaveEmptyToKeep') : 'sk-...'"
        />
      </div>
      <div class="sm:col-span-3">
        <label class="input-label">{{ $t('admin.accounts.apiKeyPool.label') }}</label>
        <input v-model="item.label" type="text" class="input" maxlength="40" />
      </div>
      <div v-if="strategy === 'weighted'" class="sm:col-span-2">
        <label class="input-label">{{ $t('admin.accounts.apiKeyPool.weight') }}</label>
        <input v-model.number="item.weight" type="number" min="1" max="10000" class="input" />
      </div>
      <div class="flex items-end gap-3 sm:col-span-3">
        <label class="mb-2 flex items-center gap-2 text-sm text-gray-600 dark:text-gray-300">
          <input v-model="item.enabled" type="checkbox" class="rounded" />
          {{ $t('admin.accounts.apiKeyPool.enabled') }}
        </label>
        <button type="button" class="btn btn-secondary btn-sm mb-1" @click="removeExtra(item.id)">
          {{ $t('admin.accounts.apiKeyPool.remove') }}
        </button>
      </div>
    </div>
  </div>
</template>
