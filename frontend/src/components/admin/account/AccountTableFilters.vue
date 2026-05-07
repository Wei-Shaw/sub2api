<template>
  <div class="flex flex-wrap items-center gap-3">
    <SearchInput
      :model-value="searchQuery"
      :placeholder="t('admin.accounts.searchAccounts')"
      class="w-full sm:w-64"
      @update:model-value="$emit('update:searchQuery', $event)"
      @search="$emit('change')"
    />
    <Select :model-value="filters.platform" class="w-40" :options="pOpts" @update:model-value="updatePlatform" @change="$emit('change')" />
    <Select :model-value="filters.type" class="w-40" :options="tOpts" @update:model-value="updateType" @change="$emit('change')" />
    <Select :model-value="filters.status" class="w-40" :options="sOpts" @update:model-value="updateStatus" @change="$emit('change')" />
    <Select :model-value="filters.privacy_mode" class="w-40" :options="privacyOpts" @update:model-value="updatePrivacyMode" @change="$emit('change')" />
    <Select :model-value="filters.group" class="w-40" :options="gOpts" @update:model-value="updateGroup" @change="$emit('change')" />
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { Select } from '@sub2api/plugin-sdk'
import SearchInput from '@/components/common/SearchInput.vue'
import { usePlatforms } from '@/composables/usePlatforms'
import type { AdminGroup } from '@/types'

const props = defineProps<{ searchQuery: string; filters: Record<string, any>; groups?: AdminGroup[] }>()
const emit = defineEmits(['update:searchQuery', 'update:filters', 'change'])
const { t } = useI18n()
const { platforms, fetchPlatforms } = usePlatforms()

onMounted(() => { fetchPlatforms() })

const updatePlatform = (value: string | number | boolean | null) => { emit('update:filters', { ...props.filters, platform: value }) }
const updateType = (value: string | number | boolean | null) => { emit('update:filters', { ...props.filters, type: value }) }
const updateStatus = (value: string | number | boolean | null) => { emit('update:filters', { ...props.filters, status: value }) }
const updatePrivacyMode = (value: string | number | boolean | null) => { emit('update:filters', { ...props.filters, privacy_mode: value }) }
const updateGroup = (value: string | number | boolean | null) => { emit('update:filters', { ...props.filters, group: value }) }

const BUILTIN_PLATFORMS = [
  { value: 'anthropic', label: 'Anthropic' },
  { value: 'openai', label: 'OpenAI' },
  { value: 'gemini', label: 'Gemini' },
  { value: 'antigravity', label: 'Antigravity' },
]

const pOpts = computed(() => {
  const builtinIds = new Set(BUILTIN_PLATFORMS.map(p => p.value))
  const pluginPlatforms = platforms.value
    .filter(p => !builtinIds.has(p.platform))
    .map(p => ({ value: p.platform, label: p.display_name }))
  return [
    { value: '', label: t('admin.accounts.allPlatforms') },
    ...BUILTIN_PLATFORMS,
    ...pluginPlatforms,
  ]
})

const BUILTIN_TYPES = [
  { value: 'oauth', label: 'admin.accounts.oauthType' },
  { value: 'setup-token', label: 'admin.accounts.setupToken' },
  { value: 'apikey', label: 'admin.accounts.apiKey' },
  { value: 'bedrock', label: 'AWS Bedrock' },
]

const tOpts = computed(() => {
  const builtinIds = new Set(BUILTIN_TYPES.map(t => t.value))
  const pluginTypes = platforms.value
    .flatMap(p => p.account_types)
    .filter(at => !builtinIds.has(at.type))
    .map(at => ({ value: at.type, label: at.display_name }))
  const seen = new Set<string>()
  const deduped = pluginTypes.filter(t => {
    if (seen.has(t.value)) return false
    seen.add(t.value)
    return true
  })
  return [
    { value: '', label: t('admin.accounts.allTypes') },
    ...BUILTIN_TYPES.map(bt => ({ value: bt.value, label: bt.label.startsWith('admin.') ? t(bt.label) : bt.label })),
    ...deduped,
  ]
})

const sOpts = computed(() => [
  { value: '', label: t('admin.accounts.allStatus') },
  { value: 'active', label: t('admin.accounts.status.active') },
  { value: 'inactive', label: t('admin.accounts.status.inactive') },
  { value: 'error', label: t('admin.accounts.status.error') },
  { value: 'rate_limited', label: t('admin.accounts.status.rateLimited') },
  { value: 'temp_unschedulable', label: t('admin.accounts.status.tempUnschedulable') },
  { value: 'unschedulable', label: t('admin.accounts.status.unschedulable') },
])

const privacyOpts = computed(() => {
  const builtinPrivacy = [
    { value: '', label: t('admin.accounts.allPrivacyModes') },
    { value: '__unset__', label: t('admin.accounts.privacyUnset') },
    { value: 'training_off', label: 'Privacy' },
    { value: 'training_set_cf_blocked', label: 'CF' },
    { value: 'training_set_failed', label: 'Fail' },
  ]
  const pluginPrivacy = platforms.value
    .flatMap(p => p.privacy_states || [])
    .map(ps => ({ value: ps.value, label: ps.display_name }))
  const seen = new Set(builtinPrivacy.map(p => p.value))
  const deduped = pluginPrivacy.filter(p => {
    if (seen.has(p.value)) return false
    seen.add(p.value)
    return true
  })
  return [...builtinPrivacy, ...deduped]
})

const gOpts = computed(() => [
  { value: '', label: t('admin.accounts.allGroups') },
  { value: 'ungrouped', label: t('admin.accounts.ungroupedGroup') },
  ...(props.groups || []).map(g => ({ value: String(g.id), label: g.name })),
])
</script>
