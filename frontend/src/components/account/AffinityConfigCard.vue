<script setup lang="ts">
import { ref, watch, computed } from 'vue'
import { useI18n } from 'vue-i18n'

const { t } = useI18n()

const props = defineProps<{
  enabled: boolean
  base: number | null
  buffer: number | null // null = infinite yellow zone
}>()

const emit = defineEmits<{
  'update:enabled': [value: boolean]
  'update:base': [value: number | null]
  'update:buffer': [value: number | null]
}>()

const localEnabled = ref(props.enabled)

watch(() => props.enabled, (val) => {
  localEnabled.value = val
})

watch(localEnabled, (val) => {
  emit('update:enabled', val)
  if (!val) {
    emit('update:base', null)
    emit('update:buffer', null)
  }
})

// Green zone: toggle + input
const baseLimitEnabled = ref(props.base != null && props.base > 0)

watch(() => props.base, (val) => {
  baseLimitEnabled.value = val != null && val > 0
})

const toggleBaseLimit = () => {
  baseLimitEnabled.value = !baseLimitEnabled.value
  if (baseLimitEnabled.value) {
    emit('update:base', 5) // default base
  } else {
    emit('update:base', null)
    emit('update:buffer', null)
  }
}

const onBaseInput = (e: Event) => {
  const raw = (e.target as HTMLInputElement).valueAsNumber
  emit('update:base', Number.isNaN(raw) ? null : Math.max(1, Math.floor(raw)))
}

// Yellow zone: "unlimited" checkbox + input
const bufferIsInfinite = ref(props.buffer === null || props.buffer === undefined)

watch(() => props.buffer, (val) => {
  bufferIsInfinite.value = val === null || val === undefined
})

const toggleBufferInfinite = () => {
  bufferIsInfinite.value = !bufferIsInfinite.value
  if (bufferIsInfinite.value) {
    emit('update:buffer', null)
  } else {
    emit('update:buffer', 3) // default buffer
  }
}

const onBufferInput = (e: Event) => {
  const raw = (e.target as HTMLInputElement).valueAsNumber
  emit('update:buffer', Number.isNaN(raw) ? null : Math.max(0, Math.floor(raw)))
}

// Zone preview
const zonePreview = computed(() => {
  const base = props.base ?? 0
  if (base <= 0) return null
  const buf = props.buffer
  const greenMax = base
  if (buf === null || buf === undefined) {
    return { green: `1~${greenMax}`, yellow: `${greenMax + 1}+`, red: null }
  }
  if (buf === 0) {
    return { green: `1~${greenMax}`, yellow: null, red: `${greenMax + 1}+` }
  }
  const yellowMax = base + buf
  return {
    green: `1~${greenMax}`,
    yellow: `${greenMax + 1}~${yellowMax}`,
    red: `${yellowMax + 1}+`
  }
})
</script>

<template>
  <div class="rounded-lg border border-gray-200 p-4 dark:border-dark-600">
      <div class="flex items-center justify-between" :class="{ 'mb-3': localEnabled }">
        <div>
          <label class="input-label mb-0">{{ t('admin.accounts.affinityToggle') }}</label>
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.accounts.affinityToggleHint') }}
          </p>
        </div>
        <button
          type="button"
          @click="localEnabled = !localEnabled"
          :class="[
            'relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2',
            localEnabled ? 'bg-primary-600' : 'bg-gray-200 dark:bg-dark-600'
          ]"
        >
          <span
            :class="[
              'pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out',
              localEnabled ? 'translate-x-5' : 'translate-x-0'
            ]"
          />
        </button>
      </div>

      <div v-if="localEnabled" class="space-y-3">
        <!-- Green zone toggle + input -->
        <div>
          <div class="flex items-center justify-between mb-1">
            <label class="input-label mb-0">{{ t('admin.accounts.affinityBase') }}</label>
            <button
              type="button"
              @click="toggleBaseLimit"
              :class="[
                'relative inline-flex h-5 w-9 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none',
                baseLimitEnabled ? 'bg-primary-600' : 'bg-gray-200 dark:bg-dark-600'
              ]"
            >
              <span
                :class="[
                  'pointer-events-none inline-block h-4 w-4 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out',
                  baseLimitEnabled ? 'translate-x-4' : 'translate-x-0'
                ]"
              />
            </button>
          </div>
          <input
            v-if="baseLimitEnabled"
            :value="base"
            @input="onBaseInput"
            type="number"
            min="1"
            step="1"
            class="input"
            :placeholder="t('admin.accounts.affinityBasePlaceholder')"
          />
          <p class="input-hint">{{ baseLimitEnabled ? t('admin.accounts.affinityBaseHint') : t('admin.accounts.affinityBaseOffHint') }}</p>
        </div>

        <!-- Buffer (yellow zone) - only shown when base is set -->
        <div v-if="baseLimitEnabled">
          <div class="flex items-center justify-between mb-1">
            <label class="input-label mb-0">{{ t('admin.accounts.affinityBuffer') }}</label>
            <label class="flex items-center gap-1.5 text-xs text-gray-500 dark:text-gray-400 cursor-pointer">
              <input
                type="checkbox"
                :checked="bufferIsInfinite"
                @change="toggleBufferInfinite"
                class="h-3.5 w-3.5 rounded border-gray-300 text-primary-600"
              />
              {{ t('admin.accounts.affinityBufferInfinite') }}
            </label>
          </div>
          <input
            v-if="!bufferIsInfinite"
            :value="buffer"
            @input="onBufferInput"
            type="number"
            min="0"
            step="1"
            class="input"
            :placeholder="t('admin.accounts.affinityBufferPlaceholder')"
          />
          <p class="input-hint">{{ t('admin.accounts.affinityBufferHint') }}</p>
        </div>

        <!-- Zone preview -->
        <div v-if="zonePreview" class="flex items-center gap-2 text-xs">
          <span class="inline-flex items-center gap-1 rounded-full bg-emerald-100 px-2 py-0.5 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-400">
            {{ zonePreview.green }}
          </span>
          <span v-if="zonePreview.yellow" class="inline-flex items-center gap-1 rounded-full bg-yellow-100 px-2 py-0.5 text-yellow-700 dark:bg-yellow-900/30 dark:text-yellow-400">
            {{ zonePreview.yellow }}
          </span>
          <span v-if="zonePreview.red" class="inline-flex items-center gap-1 rounded-full bg-red-100 px-2 py-0.5 text-red-700 dark:bg-red-900/30 dark:text-red-400">
            {{ zonePreview.red }}
          </span>
        </div>
      </div>
  </div>
</template>
