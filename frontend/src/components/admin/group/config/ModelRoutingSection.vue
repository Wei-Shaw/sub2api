<template>
  <div class="border-t pt-4">
    <div class="mb-1.5 flex items-center gap-1">
      <label class="text-sm font-medium text-gray-700 dark:text-gray-300">
        {{ t("admin.groups.modelRouting.title") }}
      </label>
      <div class="group relative inline-flex">
        <Icon
          name="questionCircle"
          size="sm"
          :stroke-width="2"
          class="cursor-help text-gray-400 transition-colors hover:text-primary-500 dark:text-gray-500 dark:hover:text-primary-400"
        />
        <div
          class="pointer-events-none absolute bottom-full left-0 z-50 mb-2 w-80 opacity-0 transition-all duration-200 group-hover:pointer-events-auto group-hover:opacity-100"
        >
          <div
            class="rounded-lg bg-gray-900 p-3 text-white shadow-lg dark:bg-gray-800"
          >
            <p class="text-xs leading-relaxed text-gray-300">
              {{ t("admin.groups.modelRouting.tooltip") }}
            </p>
            <div
              class="absolute -bottom-1.5 left-3 h-3 w-3 rotate-45 bg-gray-900 dark:bg-gray-800"
            ></div>
          </div>
        </div>
      </div>
    </div>
    <!-- Enable toggle -->
    <div class="flex items-center gap-3 mb-3">
      <button
        type="button"
        @click="
          formData.model_routing_enabled = !formData.model_routing_enabled
        "
        :class="[
          'relative inline-flex h-6 w-11 items-center rounded-full transition-colors',
          formData.model_routing_enabled
            ? 'bg-primary-500'
            : 'bg-gray-300 dark:bg-dark-600',
        ]"
      >
        <span
          :class="[
            'inline-block h-4 w-4 transform rounded-full bg-white shadow transition-transform',
            formData.model_routing_enabled
              ? 'translate-x-6'
              : 'translate-x-1',
          ]"
        />
      </button>
      <span class="text-sm text-gray-500 dark:text-gray-400">
        {{
          formData.model_routing_enabled
            ? t("admin.groups.modelRouting.enabled")
            : t("admin.groups.modelRouting.disabled")
        }}
      </span>
    </div>
    <p
      v-if="!formData.model_routing_enabled"
      class="text-xs text-gray-500 dark:text-gray-400 mb-3"
    >
      {{ t("admin.groups.modelRouting.disabledHint") }}
    </p>
    <p v-else class="text-xs text-gray-500 dark:text-gray-400 mb-3">
      {{ t("admin.groups.modelRouting.noRulesHint") }}
    </p>
    <!-- Routing rules list -->
    <div v-if="formData.model_routing_enabled" class="space-y-3">
      <div
        v-for="rule in routingRules"
        :key="getRuleRenderKey(rule)"
        class="rounded-lg border border-gray-200 p-3 dark:border-dark-600"
      >
        <div class="flex items-start gap-3">
          <div class="flex-1 space-y-2">
            <div>
              <label class="input-label text-xs">{{
                t("admin.groups.modelRouting.modelPattern")
              }}</label>
              <input
                v-model="rule.pattern"
                type="text"
                class="input text-sm"
                :placeholder="
                  t('admin.groups.modelRouting.modelPatternPlaceholder')
                "
              />
            </div>
            <div>
              <label class="input-label text-xs">{{
                t("admin.groups.modelRouting.accounts")
              }}</label>
              <!-- Selected accounts tags -->
              <div
                v-if="rule.accounts.length > 0"
                class="flex flex-wrap gap-1.5 mb-2"
              >
                <span
                  v-for="account in rule.accounts"
                  :key="account.id"
                  class="inline-flex items-center gap-1 rounded-full bg-primary-100 px-2.5 py-1 text-xs font-medium text-primary-700 dark:bg-primary-900/30 dark:text-primary-300"
                >
                  {{ account.name }}
                  <button
                    type="button"
                    @click="removeSelectedAccount(rule, account.id)"
                    class="ml-0.5 text-primary-500 hover:text-primary-700 dark:hover:text-primary-200"
                  >
                    <Icon name="x" size="xs" />
                  </button>
                </span>
              </div>
              <!-- Account search input -->
              <div class="relative account-search-container">
                <input
                  v-model="accountSearchKeyword[getRuleSearchKey(rule)]"
                  type="text"
                  class="input text-sm"
                  :placeholder="
                    t(
                      'admin.groups.modelRouting.searchAccountPlaceholder',
                    )
                  "
                  @input="searchAccountsByRule(rule)"
                  @focus="onAccountSearchFocus(rule)"
                />
                <!-- Search results dropdown -->
                <div
                  v-if="
                    showAccountDropdown[getRuleSearchKey(rule)] &&
                    accountSearchResults[getRuleSearchKey(rule)]
                      ?.length > 0
                  "
                  class="absolute z-50 mt-1 max-h-48 w-full overflow-auto rounded-lg border bg-white shadow-lg dark:border-dark-600 dark:bg-dark-800"
                >
                  <button
                    v-for="account in accountSearchResults[
                      getRuleSearchKey(rule)
                    ]"
                    :key="account.id"
                    type="button"
                    @click="selectAccount(rule, account)"
                    class="w-full px-3 py-2 text-left text-sm hover:bg-gray-100 dark:hover:bg-dark-700"
                    :class="{
                      'opacity-50': rule.accounts.some(
                        (a) => a.id === account.id,
                      ),
                    }"
                    :disabled="
                      rule.accounts.some((a) => a.id === account.id)
                    "
                  >
                    <span>{{ account.name }}</span>
                    <span class="ml-2 text-xs text-gray-400"
                      >#{{ account.id }}</span
                    >
                  </button>
                </div>
              </div>
              <p class="text-xs text-gray-400 mt-1">
                {{ t("admin.groups.modelRouting.accountsHint") }}
              </p>
            </div>
          </div>
          <button
            type="button"
            @click="removeRoutingRule(rule)"
            class="mt-5 p-1.5 text-gray-400 hover:text-red-500 transition-colors"
            :title="t('admin.groups.modelRouting.removeRule')"
          >
            <Icon name="trash" size="sm" />
          </button>
        </div>
      </div>
    </div>
    <!-- Add rule button -->
    <button
      v-if="formData.model_routing_enabled"
      type="button"
      @click="addRoutingRule"
      class="mt-3 flex items-center gap-1.5 text-sm text-primary-600 hover:text-primary-700 dark:text-primary-400 dark:hover:text-primary-300"
    >
      <Icon name="plus" size="sm" />
      {{ t("admin.groups.modelRouting.addRule") }}
    </button>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted } from "vue";
import { useI18n } from "vue-i18n";
import Icon from "@/components/icons/Icon.vue";
import { adminAPI } from "@/api/admin";
import { createStableObjectKeyResolver } from "@/utils/stableObjectKey";
import { useKeyedDebouncedSearch } from "@/composables/useKeyedDebouncedSearch";

interface SimpleAccount {
  id: number;
  name: string;
}

interface ModelRoutingRule {
  pattern: string;
  accounts: SimpleAccount[];
}

const props = defineProps<{
  mode: "create" | "edit";
  formData: Record<string, any>;
}>();

const { t } = useI18n();

// --- Model Routing State ---

const routingRules = ref<ModelRoutingRule[]>([]);

const resolveRuleKey = createStableObjectKeyResolver<ModelRoutingRule>(
  `${props.mode}-rule`,
);
const getRuleRenderKey = (rule: ModelRoutingRule) => resolveRuleKey(rule);
const getRuleSearchKey = (rule: ModelRoutingRule) =>
  `${props.mode}-${resolveRuleKey(rule)}`;

const accountSearchKeyword = ref<Record<string, string>>({});
const accountSearchResults = ref<Record<string, SimpleAccount[]>>({});
const showAccountDropdown = ref<Record<string, boolean>>({});

const accountSearchRunner = useKeyedDebouncedSearch<SimpleAccount[]>({
  delay: 300,
  search: async (keyword, { signal }) => {
    const res = await adminAPI.accounts.list(
      1,
      20,
      { search: keyword, platform: props.formData.platform || '' },
      { signal },
    );
    return res.items.map((a) => ({ id: a.id, name: a.name }));
  },
  onSuccess: (key, result) => {
    accountSearchResults.value[key] = result;
  },
  onError: (key) => {
    accountSearchResults.value[key] = [];
  },
});

const searchAccountsByRule = (rule: ModelRoutingRule) => {
  const key = getRuleSearchKey(rule);
  accountSearchRunner.trigger(key, accountSearchKeyword.value[key] || "");
};

const onAccountSearchFocus = (rule: ModelRoutingRule) => {
  const key = getRuleSearchKey(rule);
  showAccountDropdown.value[key] = true;
  if (!accountSearchResults.value[key]?.length) {
    accountSearchRunner.trigger(key, accountSearchKeyword.value[key] || "");
  }
};

const selectAccount = (rule: ModelRoutingRule, account: SimpleAccount) => {
  if (!rule.accounts.some((a) => a.id === account.id)) {
    rule.accounts.push(account);
  }
  const key = getRuleSearchKey(rule);
  accountSearchKeyword.value[key] = "";
  showAccountDropdown.value[key] = false;
};

const removeSelectedAccount = (rule: ModelRoutingRule, accountId: number) => {
  rule.accounts = rule.accounts.filter((a) => a.id !== accountId);
};

const addRoutingRule = () => {
  routingRules.value.push({ pattern: "", accounts: [] });
};

const removeRoutingRule = (rule: ModelRoutingRule) => {
  const index = routingRules.value.indexOf(rule);
  if (index === -1) return;
  const key = getRuleSearchKey(rule);
  accountSearchRunner.clearKey(key);
  delete accountSearchKeyword.value[key];
  delete accountSearchResults.value[key];
  delete showAccountDropdown.value[key];
  routingRules.value.splice(index, 1);
};

// --- API Format Conversion ---

const convertRoutingRulesToApiFormat = (
  rules: ModelRoutingRule[],
): Record<string, number[]> | null => {
  const result: Record<string, number[]> = {};
  let hasValid = false;
  for (const rule of rules) {
    const pattern = rule.pattern.trim();
    if (!pattern) continue;
    const ids = rule.accounts.map((a) => a.id).filter((id) => id > 0);
    if (ids.length > 0) {
      result[pattern] = ids;
      hasValid = true;
    }
  }
  return hasValid ? result : null;
};

const convertApiFormatToRoutingRules = async (
  apiFormat: Record<string, number[]> | null,
): Promise<ModelRoutingRule[]> => {
  if (!apiFormat) return [];
  const rules: ModelRoutingRule[] = [];
  for (const [pattern, accountIds] of Object.entries(apiFormat)) {
    const accounts: SimpleAccount[] = [];
    for (const id of accountIds) {
      try {
        const account = await adminAPI.accounts.getById(id);
        accounts.push({ id: account.id, name: account.name });
      } catch {
        accounts.push({ id, name: `#${id}` });
      }
    }
    rules.push({ pattern, accounts });
  }
  return rules;
};

// --- Public API ---

const getRoutingRulesApiFormat = () =>
  convertRoutingRulesToApiFormat(routingRules.value);

const loadRoutingRules = async (
  apiFormat: Record<string, number[]> | null,
) => {
  routingRules.value = await convertApiFormatToRoutingRules(apiFormat);
};

const resetRoutingRules = () => {
  routingRules.value.forEach((rule) => {
    accountSearchRunner.clearKey(getRuleSearchKey(rule));
  });
  accountSearchKeyword.value = {};
  accountSearchResults.value = {};
  showAccountDropdown.value = {};
  routingRules.value = [];
};

// Click outside handler to close dropdowns
const handleClickOutside = (event: MouseEvent) => {
  const target = event.target as HTMLElement;
  if (!target.closest(".account-search-container")) {
    Object.keys(showAccountDropdown.value).forEach((key) => {
      showAccountDropdown.value[key] = false;
    });
  }
};

onMounted(() => {
  document.addEventListener("click", handleClickOutside);
});

onUnmounted(() => {
  document.removeEventListener("click", handleClickOutside);
  accountSearchRunner.clearAll();
});

defineExpose({
  routingRules,
  getRoutingRulesApiFormat,
  loadRoutingRules,
  resetRoutingRules,
});
</script>
