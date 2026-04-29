<template>
  <div class="space-y-4">
    <!-- Plan C·C2: cycles in if-then-else dependencies cannot be resolved
         in a single evaluator pass; surface a banner and fall back to
         showing every field so the admin still has a way out. -->
    <div
      v-if="evaluated.cyclic"
      class="rounded-md border border-amber-300 bg-amber-50 px-3 py-2 text-xs text-amber-700 dark:border-amber-700 dark:bg-amber-900/30 dark:text-amber-200"
    >
      {{ t('admin.pluginSettings.cyclicConditions') }}
    </div>
    <div
      v-if="!visibleDescriptors.length"
      class="text-sm text-gray-500 dark:text-gray-400"
    >
      {{ t('admin.pluginSettings.emptySchema') }}
    </div>
    <div
      v-for="prop in visibleDescriptors"
      :key="prop.key"
      class="rounded-md border border-gray-200 dark:border-gray-700 p-4"
    >
      <DeprecatedBadge v-if="prop.deprecated" :message="prop.deprecated">
        <template #label>
          <span>{{ prop.title || prop.key }}</span>
        </template>
        <p
          v-if="prop.description"
          class="mt-0.5 text-xs text-gray-500 dark:text-gray-400"
        >
          {{ prop.description }}
        </p>
      </DeprecatedBadge>
      <template v-else>
        <label class="block text-sm font-medium text-gray-900 dark:text-gray-100">
          {{ prop.title || prop.key }}
        </label>
        <p
          v-if="prop.description"
          class="mt-0.5 text-xs text-gray-500 dark:text-gray-400"
        >
          {{ prop.description }}
        </p>
      </template>
      <div class="mt-3">
        <component
          :is="resolveWidget(prop)"
          :prop="prop"
          :model-value="localValues[prop.key]"
          @update:model-value="(v: unknown) => onChange(prop, v)"
        />
        <RequiresReloadBadge v-if="prop.requiresReload" />
        <p
          v-if="errors[prop.key]"
          class="mt-1 text-xs text-red-600 dark:text-red-400"
        >
          {{ errors[prop.key] }}
        </p>
      </div>
      <div class="mt-3 flex justify-end">
        <button
          type="button"
          class="inline-flex items-center rounded-md bg-blue-600 px-3 py-1.5 text-xs font-medium text-white hover:bg-blue-500 disabled:opacity-50"
          :disabled="!dirty[prop.key] || saving === prop.key"
          @click="save(prop)"
        >
          {{ saving === prop.key ? t('common.saving') : t('common.save') }}
        </button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
// SETTINGS-V2 plugin settings form. Reads PluginSettingsSchemaInfo from the
// admin API and renders one row per top-level schema property using the
// widget map (resolveWidget) + decorators from
// @/components/admin/plugin-settings-widgets. Backend-only fields are
// filtered out client-side; the host also rejects writes server-side.
//
// DESIGN §5.6 — dirty/save/error orchestration only. Widget rendering rules
// live in plugin-settings-widgets/index.ts. The deprecated wrapper renders
// the label inside its own slot (strikethrough + badge); the plain branch
// renders the same label outside any wrapper. The widget body, error and
// save button are shared between both branches via straight markup.
import { computed, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'

import {
  pluginSettingsApi,
  type PluginSettingsSchemaInfo,
} from '@/api/admin/pluginSettings'
import {
  DeprecatedBadge,
  RequiresReloadBadge,
  buildPropDescriptors,
  evaluateConditions,
  resolveWidget,
  type PropDescriptor,
} from '@/components/admin/plugin-settings-widgets'
import { useAppStore } from '@/stores'
import { extractApiErrorMessage } from '@/utils/apiError'

const props = defineProps<{ info: PluginSettingsSchemaInfo }>()
const emit = defineEmits<{
  (e: 'updated', key: string, value: unknown): void
}>()

const { t } = useI18n()
const appStore = useAppStore()

const schemaProperties = computed<PropDescriptor[]>(() =>
  // DESIGN §5.4: backend-only fields are not rendered. Server-side
  // validation still rejects writes for defence in depth.
  buildPropDescriptors(props.info).filter((p) => p.visibility !== 'backend'),
)

const localValues = reactive<Record<string, unknown>>({ ...(props.info.values ?? {}) })
const dirty = reactive<Record<string, boolean>>({})
const errors = reactive<Record<string, string>>({})
const saving = ref<string | null>(null)

// Plan C·C2 — opt-out flag for the conditional evaluator. When the
// flag is set to 'off', the form behaves like the pre-C2 version
// (every schema field is rendered).
const conditionsEnabled = import.meta.env.VITE_PLUGIN_SETTINGS_CONDITIONS !== 'off'

const evaluated = computed(() => {
  if (!conditionsEnabled || !props.info?.schema) {
    return { hidden: new Set<string>(), requiredOverride: new Set<string>(), cyclic: false }
  }
  return evaluateConditions(
    props.info.schema as Record<string, unknown>,
    localValues,
  )
})

const visibleDescriptors = computed<PropDescriptor[]>(() =>
  schemaProperties.value.filter((p) => !evaluated.value.hidden.has(p.key)),
)

watch(
  () => props.info,
  (next) => {
    Object.keys(localValues).forEach((k) => delete localValues[k])
    Object.assign(localValues, next.values ?? {})
    Object.keys(dirty).forEach((k) => delete dirty[k])
    Object.keys(errors).forEach((k) => delete errors[k])
  },
)

function onChange(prop: PropDescriptor, v: unknown) {
  localValues[prop.key] = v
  dirty[prop.key] = true
  delete errors[prop.key]
}

async function save(prop: PropDescriptor) {
  if (errors[prop.key]) return
  // Plan C·C2: refuse to save fields that the evaluator currently hides.
  // The form already drops hidden fields from the render tree, so this
  // is a defense-in-depth guard against a stale dirty entry firing
  // through some queued widget event.
  if (evaluated.value.hidden.has(prop.key)) {
    dirty[prop.key] = false
    return
  }
  // SETTINGS-V2 secret semantics (DESIGN §5.6): empty input on a NOT-yet
  // configured secret means "user did not type anything" — skip the request.
  // Empty input on a configured secret falls through to the regular PUT,
  // which the backend interprets as "delete row".
  const value = localValues[prop.key]
  if (prop.visibility === 'secret' && value === '' && !prop.isConfigured) {
    dirty[prop.key] = false
    return
  }
  saving.value = prop.key
  try {
    await pluginSettingsApi.update(props.info.plugin, prop.key, value)
    dirty[prop.key] = false
    emit('updated', prop.key, value)
    appStore.showSuccess(t('admin.pluginSettings.saveSuccess'))
  } catch (err: unknown) {
    // Map well-known SETTINGS-V2 errcodes to i18n strings; all other codes
    // fall through to the backend's English `message`.
    appStore.showError(
      extractApiErrorMessage(err, t('common.error'), {
        PLUGIN_SETTINGS_BACKEND_ONLY: t('admin.pluginSettings.backendOnly'),
      }),
    )
  } finally {
    saving.value = null
  }
}
</script>
