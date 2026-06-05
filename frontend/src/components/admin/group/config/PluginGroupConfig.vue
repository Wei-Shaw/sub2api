<template>
  <div v-if="hasSchema" class="space-y-4 border-t border-gray-200 pt-4 dark:border-dark-600">
    <h3 class="text-sm font-medium text-gray-700 dark:text-gray-300">
      {{ t('admin.groups.pluginConfig') }}
    </h3>
    <JsonSchemaForm
      :schema="schema"
      :model-value="formData.group_extra || {}"
      @update:model-value="formData.group_extra = $event"
    />
  </div>
  <div v-else class="border-t border-gray-200 pt-4 dark:border-dark-600">
    <p class="text-sm text-gray-500 dark:text-gray-400">
      {{ t('admin.groups.noPluginConfig') }}
    </p>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { usePlatforms } from '@/composables/usePlatforms'
import JsonSchemaForm from '@/components/common/JsonSchemaForm.vue'

const formData = defineModel<Record<string, any>>('formData', { required: true })

const props = defineProps<{
  mode: 'create' | 'edit'
  platform: string
  groups: any[]
  editingGroupId?: number | null
}>()



const { t } = useI18n()
const { getPlatformDecl } = usePlatforms()

const schema = computed(() => {
  const decl = getPlatformDecl(props.platform)
  return decl?.group_config?.group_extra_schema || null
})
const hasSchema = computed(() => schema.value != null && Object.keys(schema.value).length > 0)
</script>
