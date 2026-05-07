<template>
  <div class="space-y-5">
    <!-- Credential Fields from plugin JSON Schema -->
    <div v-if="credentialSchema" class="border-t border-gray-200 pt-4 dark:border-dark-600">
      <h3 class="mb-3 text-sm font-medium text-gray-700 dark:text-gray-300">
        {{ t("admin.accounts.credentials") }}
      </h3>
      <JsonSchemaForm
        :schema="credentialSchema"
        :model-value="credentials"
        @update:model-value="credentials = $event"
      />
    </div>
    <!-- Extra Fields from plugin JSON Schema -->
    <div v-if="extraSchema" class="border-t border-gray-200 pt-4 dark:border-dark-600">
      <h3 class="mb-3 text-sm font-medium text-gray-700 dark:text-gray-300">
        {{ t("admin.accounts.extra") }}
      </h3>
      <JsonSchemaForm
        :schema="extraSchema"
        :model-value="extra"
        @update:model-value="extra = $event"
      />
    </div>
    <p v-if="!credentialSchema && !extraSchema" class="text-sm text-gray-500 dark:text-gray-400">
      {{ t("admin.accounts.noFieldsDeclared") }}
    </p>
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from "vue"
import { useI18n } from "vue-i18n"
import JsonSchemaForm from "@/components/common/JsonSchemaForm.vue"
import { usePlatforms } from "@/composables/usePlatforms"
import type { PlatformFormContext, PlatformFormPayload, PlatformFormValidation, EditFormPayload } from "./types"
import type { Account } from '@/types'

const props = defineProps<{
  context: PlatformFormContext
  platform: string
}>()
const { t } = useI18n()
const { getAccountTypeDecl } = usePlatforms()

const platform = computed(() => props.platform)
const credentials = ref<Record<string, unknown>>({})
const extra = ref<Record<string, unknown>>({})

const accountTypeDecl = computed(() =>
  getAccountTypeDecl(platform.value, props.context.accountTypeId)
)
const credentialSchema = computed(() =>
  (accountTypeDecl.value?.credential_schema as Record<string, unknown>) ?? null
)
const extraSchema = computed(() =>
  (accountTypeDecl.value?.extra_schema as Record<string, unknown>) ?? null
)

function validate(): PlatformFormValidation {
  return { valid: true }
}

function getPayload(): PlatformFormPayload {
  return {
    credentials: { ...credentials.value },
    extra: Object.keys(extra.value).length > 0 ? { ...extra.value } : undefined,
  }
}

function isOAuthFlow(): boolean { return false }
function reset() { credentials.value = {}; extra.value = {} }

function initFromAccount(account: Account): void {
  credentials.value = { ...((account.credentials as Record<string, unknown>) || {}) }
  extra.value = { ...((account.extra as Record<string, unknown>) || {}) }
}

function getEditPayload(_account: Account): EditFormPayload {
  return {
    credentials: { ...credentials.value },
    extra: Object.keys(extra.value).length > 0 ? { ...extra.value } : undefined,
  }
}

defineExpose({ validate, getPayload, isOAuthFlow, reset, initFromAccount, getEditPayload })
</script>