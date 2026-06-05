<template>
  <div
    class="border-t border-gray-200 dark:border-dark-400 pt-4 mt-4 space-y-4"
  >
    <h4 class="text-sm font-medium text-gray-700 dark:text-gray-300 mb-3">
      {{ t("admin.groups.accountFilters.title") }}
    </h4>

    <!-- require_oauth_only toggle -->
    <div class="flex items-center justify-between">
      <div>
        <label class="text-sm text-gray-600 dark:text-gray-400">{{
          t("admin.groups.accountFilters.oauthOnly")
        }}</label>
        <p class="text-xs text-gray-500 dark:text-gray-400 mt-0.5">
          {{
            formData.require_oauth_only
              ? t("admin.groups.accountFilters.oauthOnlyEnabled")
              : t("admin.groups.accountFilters.disabled")
          }}
        </p>
      </div>
      <button
        type="button"
        @click="formData.require_oauth_only = !formData.require_oauth_only"
        class="relative inline-flex h-6 w-12 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none"
        :class="
          formData.require_oauth_only
            ? 'bg-primary-500'
            : 'bg-gray-300 dark:bg-dark-600'
        "
      >
        <span
          class="pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out"
          :class="
            formData.require_oauth_only
              ? 'translate-x-6'
              : 'translate-x-1'
          "
        />
      </button>
    </div>

    <!-- require_privacy_set toggle -->
    <div class="flex items-center justify-between">
      <div>
        <label class="text-sm text-gray-600 dark:text-gray-400">{{
          t("admin.groups.accountFilters.privacySet")
        }}</label>
        <p class="text-xs text-gray-500 dark:text-gray-400 mt-0.5">
          {{
            formData.require_privacy_set
              ? t("admin.groups.accountFilters.privacySetEnabled")
              : t("admin.groups.accountFilters.disabled")
          }}
        </p>
      </div>
      <button
        type="button"
        @click="
          formData.require_privacy_set = !formData.require_privacy_set
        "
        class="relative inline-flex h-6 w-12 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none"
        :class="
          formData.require_privacy_set
            ? 'bg-primary-500'
            : 'bg-gray-300 dark:bg-dark-600'
        "
      >
        <span
          class="pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out"
          :class="
            formData.require_privacy_set
              ? 'translate-x-6'
              : 'translate-x-1'
          "
        />
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from "vue-i18n";

const formData = defineModel<Record<string, any>>('formData', { required: true });

const { t } = useI18n();
</script>
