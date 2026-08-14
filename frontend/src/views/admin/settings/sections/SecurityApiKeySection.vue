<template>
  <div v-show="activeTab === 'security'" class="space-y-6">
    <!-- Admin API Key Settings -->
    <div class="card">
      <div
        class="border-b border-line-subtle px-6 py-4"
      >
        <h2 class="text-lg font-semibold text-ink">
          {{ t("admin.settings.adminApiKey.title") }}
        </h2>
        <p class="mt-1 text-sm text-ink-secondary">
          {{ t("admin.settings.adminApiKey.description") }}
        </p>
      </div>
      <div class="space-y-4 p-6">
        <!-- Security Warning -->
        <div
          class="rounded-sm border border border-warn/40 bg-warn-tint p-4"
        >
          <div class="flex items-start">
            <Icon
              name="exclamationTriangle"
              size="md"
              class="mt-0.5 flex-shrink-0 text-warn"
            />
            <p class="ml-3 text-sm text-warn">
              {{ t("admin.settings.adminApiKey.securityWarning") }}
            </p>
          </div>
        </div>

        <!-- Loading State -->
        <div
          v-if="adminApiKeyLoading"
          class="flex items-center gap-2 text-ink-secondary"
        >
          <div
            class="h-4 w-4 animate-spin rounded-full border-b-2 border-accent-line"
          ></div>
          {{ t("common.loading") }}
        </div>

        <!-- No Key Configured -->
        <div
          v-else-if="!adminApiKeyExists"
          class="flex items-center justify-between"
        >
          <span class="text-ink-secondary">
            {{ t("admin.settings.adminApiKey.notConfigured") }}
          </span>
          <button
            type="button"
            @click="createAdminApiKey"
            :disabled="adminApiKeyOperating"
            class="btn btn-primary btn-sm"
          >
            <svg
              v-if="adminApiKeyOperating"
              class="mr-1 h-4 w-4 animate-spin"
              fill="none"
              viewBox="0 0 24 24"
            >
              <circle
                class="opacity-25"
                cx="12"
                cy="12"
                r="10"
                stroke="currentColor"
                stroke-width="4"
              ></circle>
              <path
                class="opacity-75"
                fill="currentColor"
                d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"
              ></path>
            </svg>
            {{
              adminApiKeyOperating
                ? t("admin.settings.adminApiKey.creating")
                : t("admin.settings.adminApiKey.create")
            }}
          </button>
        </div>

        <!-- Key Exists -->
        <div v-else class="space-y-4">
          <div class="flex items-center justify-between">
            <div>
              <label
                class="mb-1 block text-sm font-medium text-ink-secondary"
              >
                {{ t("admin.settings.adminApiKey.currentKey") }}
              </label>
              <code
                class="rounded bg-surface-sunken px-2 py-1 font-mono text-sm text-ink"
              >
                {{ adminApiKeyMasked }}
              </code>
            </div>
            <div class="flex gap-2">
              <button
                type="button"
                @click="regenerateAdminApiKey"
                :disabled="adminApiKeyOperating"
                class="btn btn-secondary btn-sm"
              >
                {{
                  adminApiKeyOperating
                    ? t("admin.settings.adminApiKey.regenerating")
                    : t("admin.settings.adminApiKey.regenerate")
                }}
              </button>
              <button
                type="button"
                @click="deleteAdminApiKey"
                :disabled="adminApiKeyOperating"
                class="btn btn-secondary btn-sm text-danger hover:text-danger"
              >
                {{ t("admin.settings.adminApiKey.delete") }}
              </button>
            </div>
          </div>

          <!-- Newly Generated Key Display -->
          <div
            v-if="newAdminApiKey"
            class="space-y-3 rounded-sm border border border-success/40 bg-success-tint p-4"
          >
            <p
              class="text-sm font-medium text-success"
            >
              {{ t("admin.settings.adminApiKey.keyWarning") }}
            </p>
            <div class="flex items-center gap-2">
              <code
                class="flex-1 select-all break-all rounded border border-success/40 bg-surface px-3 py-2 font-mono text-sm dark:border-green-700"
              >
                {{ newAdminApiKey }}
              </code>
              <button
                type="button"
                @click="copyNewKey"
                class="btn btn-primary btn-sm flex-shrink-0"
              >
                {{ t("admin.settings.adminApiKey.copyKey") }}
              </button>
            </div>
            <p class="text-xs text-success">
              {{ t("admin.settings.adminApiKey.usage") }}
            </p>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import Icon from "@/components/icons/Icon.vue";
import { useSettingsFormContext } from "../context";

const {
  activeTab,
  adminApiKeyExists,
  adminApiKeyLoading,
  adminApiKeyMasked,
  adminApiKeyOperating,
  copyNewKey,
  createAdminApiKey,
  deleteAdminApiKey,
  newAdminApiKey,
  regenerateAdminApiKey,
  t,
} = useSettingsFormContext();
</script>
