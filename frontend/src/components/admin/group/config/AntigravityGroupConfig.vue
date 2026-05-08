<template>
  <div class="space-y-4">
    <!-- 图片生成计费配置 -->
    <SharedImagePricing :form-data="formData" />

    <!-- 支持的模型系列 -->
    <div class="border-t pt-4">
      <div class="mb-1.5 flex items-center gap-1">
        <label class="text-sm font-medium text-gray-700 dark:text-gray-300">
          {{ t("admin.groups.supportedScopes.title") }}
        </label>
        <!-- Help Tooltip -->
        <div class="group relative inline-flex">
          <Icon
            name="questionCircle"
            size="sm"
            :stroke-width="2"
            class="cursor-help text-gray-400 transition-colors hover:text-primary-500 dark:text-gray-500 dark:hover:text-primary-400"
          />
          <div
            class="pointer-events-none absolute bottom-full left-0 z-50 mb-2 w-72 opacity-0 transition-all duration-200 group-hover:pointer-events-auto group-hover:opacity-100"
          >
            <div
              class="rounded-lg bg-gray-900 p-3 text-white shadow-lg dark:bg-gray-800"
            >
              <p class="text-xs leading-relaxed text-gray-300">
                {{ t("admin.groups.supportedScopes.tooltip") }}
              </p>
              <div
                class="absolute -bottom-1.5 left-3 h-3 w-3 rotate-45 bg-gray-900 dark:bg-gray-800"
              ></div>
            </div>
          </div>
        </div>
      </div>
      <div class="space-y-2">
        <label class="flex items-center gap-2 cursor-pointer">
          <input
            type="checkbox"
            :checked="formData.supported_model_scopes.includes('claude')"
            @change="toggleScope('claude')"
            class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500 dark:border-dark-600 dark:bg-dark-700"
          />
          <span class="text-sm text-gray-700 dark:text-gray-300">{{
            t("admin.groups.supportedScopes.claude")
          }}</span>
        </label>
        <label class="flex items-center gap-2 cursor-pointer">
          <input
            type="checkbox"
            :checked="
              formData.supported_model_scopes.includes('gemini_text')
            "
            @change="toggleScope('gemini_text')"
            class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500 dark:border-dark-600 dark:bg-dark-700"
          />
          <span class="text-sm text-gray-700 dark:text-gray-300">{{
            t("admin.groups.supportedScopes.geminiText")
          }}</span>
        </label>
        <label class="flex items-center gap-2 cursor-pointer">
          <input
            type="checkbox"
            :checked="
              formData.supported_model_scopes.includes('gemini_image')
            "
            @change="toggleScope('gemini_image')"
            class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500 dark:border-dark-600 dark:bg-dark-700"
          />
          <span class="text-sm text-gray-700 dark:text-gray-300">{{
            t("admin.groups.supportedScopes.geminiImage")
          }}</span>
        </label>
      </div>
      <p class="mt-2 text-xs text-gray-500 dark:text-gray-400">
        {{ t("admin.groups.supportedScopes.hint") }}
      </p>
    </div>

    <!-- MCP XML 协议注入 -->
    <div class="border-t pt-4">
      <div class="mb-1.5 flex items-center gap-1">
        <label class="text-sm font-medium text-gray-700 dark:text-gray-300">
          {{ t("admin.groups.mcpXml.title") }}
        </label>
        <div class="group relative inline-flex">
          <Icon
            name="questionCircle"
            size="sm"
            :stroke-width="2"
            class="cursor-help text-gray-400 transition-colors hover:text-primary-500 dark:text-gray-500 dark:hover:text-primary-400"
          />
          <div
            class="pointer-events-none absolute bottom-full left-0 z-50 mb-2 w-72 opacity-0 transition-all duration-200 group-hover:pointer-events-auto group-hover:opacity-100"
          >
            <div
              class="rounded-lg bg-gray-900 p-3 text-white shadow-lg dark:bg-gray-800"
            >
              <p class="text-xs leading-relaxed text-gray-300">
                {{ t("admin.groups.mcpXml.tooltip") }}
              </p>
              <div
                class="absolute -bottom-1.5 left-3 h-3 w-3 rotate-45 bg-gray-900 dark:bg-gray-800"
              ></div>
            </div>
          </div>
        </div>
      </div>
      <div class="flex items-center gap-3">
        <button
          type="button"
          @click="formData.mcp_xml_inject = !formData.mcp_xml_inject"
          :class="[
            'relative inline-flex h-6 w-11 items-center rounded-full transition-colors',
            formData.mcp_xml_inject
              ? 'bg-primary-500'
              : 'bg-gray-300 dark:bg-dark-600',
          ]"
        >
          <span
            :class="[
              'inline-block h-4 w-4 transform rounded-full bg-white shadow transition-transform',
              formData.mcp_xml_inject ? 'translate-x-6' : 'translate-x-1',
            ]"
          />
        </button>
        <span class="text-sm text-gray-500 dark:text-gray-400">
          {{
            formData.mcp_xml_inject
              ? t("admin.groups.mcpXml.enabled")
              : t("admin.groups.mcpXml.disabled")
          }}
        </span>
      </div>
    </div>

    <!-- 账号过滤控制 -->
    <SharedAccountFilters :form-data="formData" />

    <!-- 无效请求兜底（非订阅分组时显示） -->
    <SharedInvalidRequestFallback
      v-if="formData.subscription_type !== 'subscription'"
      :form-data="formData"
      :groups="groups"
      :editing-group-id="editingGroupId"
    />
  </div>
</template>

<script setup lang="ts">
import { useI18n } from "vue-i18n";
import type { AdminGroup } from "@/types";
import Icon from "@/components/icons/Icon.vue";
import SharedImagePricing from "./SharedImagePricing.vue";
import SharedAccountFilters from "./SharedAccountFilters.vue";
import SharedInvalidRequestFallback from "./SharedInvalidRequestFallback.vue";

const props = defineProps<{
  mode: "create" | "edit";
  platform: string;
  formData: Record<string, any>;
  groups: AdminGroup[];
  editingGroupId?: number | null;
}>();

const { t } = useI18n();

const toggleScope = (scope: string) => {
  const scopes = props.formData.supported_model_scopes as string[];
  const idx = scopes.indexOf(scope);
  if (idx === -1) {
    scopes.push(scope);
  } else {
    scopes.splice(idx, 1);
  }
};
</script>
