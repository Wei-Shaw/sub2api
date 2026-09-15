<template>
  <section class="card" aria-labelledby="key-protection-title">
    <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
      <h2 id="key-protection-title" class="text-lg font-semibold text-gray-900 dark:text-white">
        {{ t('admin.settings.keyProtection.title') }}
      </h2>
      <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('admin.settings.keyProtection.description') }}</p>
    </div>
    <div class="space-y-4 p-6">
      <p v-if="loading" role="status">{{ t('common.loading') }}</p>
      <div v-else-if="!config" class="space-y-2">
        <p role="alert" class="text-sm text-red-600">{{ t('admin.settings.keyProtection.loadFailed') }}</p>
        <button type="button" class="btn btn-secondary" @click="load">{{ t('admin.settings.keyProtection.retry') }}</button>
      </div>
      <template v-else>
        <div class="flex items-center justify-between gap-4">
          <label for="key-protection-enabled" class="font-medium">{{ t('admin.settings.keyProtection.enabled') }}</label>
          <Toggle id="key-protection-enabled" v-model="config.enabled" :aria-label="t('admin.settings.keyProtection.enabled')" />
        </div>
        <div class="space-y-2">
          <p class="text-sm font-medium">{{ t('admin.settings.keyProtection.users') }}</p>
          <OpenAIFastPolicyUserSelector v-model="config.user_ids" />
        </div>
        <label class="block space-y-1 text-sm">
          <span>{{ t('admin.settings.keyProtection.groups') }}</span>
          <select v-model="config.group_ids" multiple class="input min-h-24 w-full" data-testid="protection-groups">
            <option v-for="group in groupOptions" :key="group.id" :value="group.id">{{ group.name }} (#{{ group.id }})</option>
          </select>
        </label>
        <p class="text-sm text-gray-500 dark:text-gray-400">{{ t('admin.settings.keyProtection.targetHint') }}</p>
        <p class="rounded-lg bg-amber-50 p-3 text-sm text-amber-900 dark:bg-amber-900/20 dark:text-amber-200">
          {{ t('admin.settings.keyProtection.boundary') }}
        </p>
        <details class="space-y-3 rounded-lg border border-gray-200 p-3 dark:border-dark-600">
          <summary class="cursor-pointer text-sm font-medium">{{ t('admin.settings.keyProtection.advanced') }}</summary>
          <label class="block space-y-1 text-sm">
            <span>{{ t('admin.settings.keyProtection.rules') }}</span>
            <input v-model="ruleNames" type="text" class="input w-full" autocomplete="off" data-testid="protection-rules" />
          </label>
          <label class="block space-y-1 text-sm">
            <span>{{ t('admin.settings.keyProtection.customRules') }}</span>
            <textarea v-model="customRulesJSON" rows="4" class="input w-full font-mono text-xs" spellcheck="false" data-testid="protection-custom-rules" />
          </label>
          <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.settings.keyProtection.rulesHint') }}</p>
        </details>
        <p v-if="errorMessage" role="alert" class="text-sm text-red-600">{{ errorMessage }}</p>
        <button type="button" class="btn btn-primary" :disabled="saving" data-testid="protection-save" @click="save">
          {{ saving ? t('common.saving') : t('admin.settings.keyProtection.save') }}
        </button>
      </template>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { getAllIncludingInactive } from '@/api/admin/groups'
import { getKeyProtectionConfig, updateKeyProtectionConfig, type KeyProtectionConfig } from '@/api/admin/keyProtection'
import Toggle from '@/components/common/Toggle.vue'
import { useAppStore } from '@/stores'
import OpenAIFastPolicyUserSelector from './OpenAIFastPolicyUserSelector.vue'

const { t } = useI18n()
const appStore = useAppStore()
const config = ref<KeyProtectionConfig | null>(null)
const groups = ref<{ id: number; name: string }[]>([])
const loading = ref(true)
const saving = ref(false)
const errorMessage = ref('')
const ruleNames = ref('')
const customRulesJSON = ref('[]')

// Preserve saved targeting even when a group has since been removed.
const groupOptions = computed(() => {
  const known = new Set(groups.value.map(group => group.id))
  return [...groups.value, ...(config.value?.group_ids ?? []).filter(id => !known.has(id)).map(id => ({ id, name: `#${id}` }))]
})

function applyConfig(value: KeyProtectionConfig) {
  config.value = { ...value, user_ids: value.user_ids ?? [], group_ids: value.group_ids ?? [], rules: value.rules ?? [], custom_rules: value.custom_rules ?? [] }
  ruleNames.value = config.value.rules.join(', ')
  customRulesJSON.value = JSON.stringify(config.value.custom_rules, null, 2)
}

async function load() {
  loading.value = true
  errorMessage.value = ''
  try {
    const [policy, options] = await Promise.all([getKeyProtectionConfig(), getAllIncludingInactive()])
    applyConfig(policy)
    groups.value = options
  } catch {
    config.value = null
  } finally {
    loading.value = false
  }
}

async function save() {
  if (!config.value || saving.value) return
  errorMessage.value = ''
  let customRules: KeyProtectionConfig['custom_rules']
  try {
    const parsed: unknown = JSON.parse(customRulesJSON.value)
    if (!Array.isArray(parsed) || parsed.some(rule => !rule || typeof rule !== 'object' || typeof rule.name !== 'string' || typeof rule.pattern !== 'string' || Object.keys(rule).some(key => key !== 'name' && key !== 'pattern'))) throw new Error('invalid rules')
    customRules = parsed
  } catch {
    errorMessage.value = t('admin.settings.keyProtection.invalidRules')
    return
  }
  saving.value = true
  try {
    applyConfig(await updateKeyProtectionConfig({
      ...config.value,
      rules: ruleNames.value.split(/[\s,]+/).filter(Boolean),
      custom_rules: customRules,
    }))
    appStore.showSuccess(t('admin.settings.keyProtection.saved'))
  } catch {
    errorMessage.value = t('admin.settings.keyProtection.saveFailed')
  } finally {
    saving.value = false
  }
}

onMounted(load)
</script>
