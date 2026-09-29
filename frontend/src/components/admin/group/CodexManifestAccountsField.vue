<template>
  <div class="border-t border-border pt-4 mt-4">
    <div class="mb-3 flex items-start justify-between gap-3">
      <div class="min-w-0 flex-1">
        <label class="text-sm font-medium text-fg">
          {{ t("admin.groups.codexModelsManifest.title") }}
        </label>
        <p class="mt-1 text-xs text-fg-muted">
          {{ t("admin.groups.codexModelsManifest.hint") }}
        </p>
      </div>
      <Toggle
        data-testid="codex-manifest-toggle"
        :aria-label="t('admin.groups.codexModelsManifest.enable')"
        :model-value="config.enabled"
        @update:model-value="emitUpdate({ enabled: $event })"
      />
    </div>

    <div v-if="config.enabled">
      <p
        v-if="config.enabled"
        class="mb-2 text-xs text-fg-muted"
      >
        {{ t("admin.groups.codexModelsManifest.enabledHint") }}
      </p>

      <label class="input-label">
        {{ t("admin.groups.codexModelsManifest.accounts") }}
      </label>

      <!-- 已选账号标签 -->
      <div
        v-if="config.account_ids.length > 0"
        class="mb-2 flex flex-wrap gap-1.5"
        data-testid="codex-manifest-selected-tags"
      >
        <span
          v-for="id in config.account_ids"
          :key="id"
          class="badge badge-primary gap-1"
        >
          {{ accountLabel(id) }}
          <button
            type="button"
            class="ml-0.5 text-accent hover:text-accent-strong"
            :aria-label="`remove account ${id}`"
            @click="removeAccount(id)"
          >
            <Icon name="x" size="xs" />
          </button>
        </span>
      </div>

      <!-- 搜索输入 + 下拉 -->
      <div class="relative" ref="searchContainerRef">
        <input
          v-model="searchKeyword"
          type="text"
          class="input text-sm"
          data-testid="codex-manifest-search"
          :placeholder="t('admin.groups.codexModelsManifest.searchPlaceholder')"
          @input="searchAccounts"
          @focus="onSearchFocus"
        />
        <div
          v-if="showDropdown && (searchResults.length > 0 || searchKeyword.trim() !== '')"
          class="absolute z-50 mt-1 max-h-48 w-full overflow-auto rounded-sm border border-border-strong bg-surface-raised shadow-overlay"
          data-testid="codex-manifest-dropdown"
        >
          <p
            v-if="searchResults.length === 0"
            class="px-3 py-2 text-sm text-fg-subtle"
            data-testid="codex-manifest-search-empty"
          >
            {{ t("admin.groups.codexModelsManifest.searchEmpty") }}
          </p>
          <button
            v-for="account in searchResults"
            :key="account.id"
            type="button"
            class="w-full px-3 py-2 text-left text-sm hover:bg-accent-weak"
            :class="{
              'opacity-50': config.account_ids.includes(account.id),
            }"
            :disabled="config.account_ids.includes(account.id)"
            @click="selectAccount(account)"
          >
            <span>{{ account.name }}</span>
            <span class="ml-2 font-mono text-xs text-fg-subtle">#{{ account.id }}</span>
          </button>
        </div>
      </div>

      <!-- 回退子开关 -->
      <div class="mt-3 flex items-start justify-between gap-3">
        <div class="min-w-0 flex-1">
          <label class="text-sm text-fg">
            {{ t("admin.groups.codexModelsManifest.fallback") }}
          </label>
          <p class="mt-1 text-xs text-fg-muted">
            {{ t("admin.groups.codexModelsManifest.fallbackHint") }}
          </p>
        </div>
        <Toggle
          data-testid="codex-manifest-fallback-toggle"
          :aria-label="t('admin.groups.codexModelsManifest.fallback')"
          :model-value="config.fallback_to_scheduler"
          @update:model-value="emitUpdate({ fallback_to_scheduler: $event })"
        />
      </div>

      <p
        v-if="showValidationError"
        class="mt-2 text-xs text-danger"
        role="alert"
        data-testid="codex-manifest-validation-error"
      >
        {{ t("admin.groups.codexModelsManifest.selectAtLeastOne") }}
      </p>
    </div>
    <p v-else class="text-xs text-fg-muted">
      {{ t("admin.groups.codexModelsManifest.disabledHint") }}
    </p>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from "vue";
import { useI18n } from "vue-i18n";
import Icon from "@/components/icons/Icon.vue";
import Toggle from "@/components/common/Toggle.vue";
import { useKeyedDebouncedSearch } from "@/composables/useKeyedDebouncedSearch";
import { adminAPI } from "@/api/admin";
import type { CodexModelsManifestConfig } from "@/types";

interface SimpleAccount {
  id: number;
  name: string;
}

const props = defineProps<{
  groupId: number;
  modelValue: CodexModelsManifestConfig;
  /** 已选账号 ID → 名称映射；无法解析的 ID 以 #<id> 展示 */
  accountNames?: Record<number, string>;
}>();

const emit = defineEmits<{
  (event: "update:modelValue", value: CodexModelsManifestConfig): void;
}>();

const { t } = useI18n();

// 本次会话内新选账号的名称缓存：props.accountNames 只覆盖打开对话框时已存的
// ID，搜索新选的账号名称由组件自己记住，避免标签闪现 #<id>。
const localNames = ref<Record<number, string>>({});

const config = computed(() => props.modelValue);

const emitUpdate = (patch: Partial<CodexModelsManifestConfig>) => {
  emit("update:modelValue", { ...props.modelValue, ...patch });
};

const accountLabel = (id: number) =>
  props.accountNames?.[id] ?? localNames.value[id] ?? `#${id}`;

const searchKeyword = ref("");
const searchResults = ref<SimpleAccount[]>([]);
const showDropdown = ref(false);
const showValidationError = ref(false);
const searchContainerRef = ref<HTMLElement | null>(null);

// 点击下拉容器外部时收起（与 GroupsView 模型路由下拉同一模式）。
const handleDocumentClick = (event: MouseEvent) => {
  const target = event.target as HTMLElement;
  if (searchContainerRef.value && !searchContainerRef.value.contains(target)) {
    showDropdown.value = false;
  }
};
onMounted(() => {
  // 捕获阶段监听：BaseDialog 面板的 @click.stop 会拦住冒泡，否则对话框内点击其他位置关不掉下拉。
  document.addEventListener("click", handleDocumentClick, true);
});
onUnmounted(() => {
  document.removeEventListener("click", handleDocumentClick, true);
});

const searchRunner = useKeyedDebouncedSearch<SimpleAccount[]>({
  delay: 300,
  search: async (keyword, { signal }) => {
    const res = await adminAPI.accounts.list(
      1,
      20,
      {
        search: keyword,
        platform: "openai",
        group: String(props.groupId),
      },
      { signal },
    );
    return res.items.map((account) => ({
      id: account.id,
      name: account.name,
    }));
  },
  onSuccess: (_key, result) => {
    searchResults.value = result;
  },
  onError: () => {
    searchResults.value = [];
  },
});

const searchAccounts = () => {
  searchRunner.trigger("codex-manifest", searchKeyword.value);
};

const onSearchFocus = () => {
  showDropdown.value = true;
  if (searchResults.value.length === 0) {
    searchAccounts();
  }
};

const selectAccount = (account: SimpleAccount) => {
  if (props.modelValue.account_ids.includes(account.id)) return;
  localNames.value[account.id] = account.name;
  emitUpdate({
    account_ids: [...props.modelValue.account_ids, account.id],
  });
  searchKeyword.value = "";
  showDropdown.value = false;
  showValidationError.value = false;
};

const removeAccount = (id: number) => {
  emitUpdate({
    account_ids: props.modelValue.account_ids.filter(
      (accountId) => accountId !== id,
    ),
  });
};

/** 校验：开启后至少一个账号。返回是否通过；不通过时展示错误提示。 */
const validate = (): boolean => {
  if (props.modelValue.enabled && props.modelValue.account_ids.length === 0) {
    showValidationError.value = true;
    return false;
  }
  showValidationError.value = false;
  return true;
};

const resetValidation = () => {
  showValidationError.value = false;
};

defineExpose({ validate, resetValidation });
</script>
