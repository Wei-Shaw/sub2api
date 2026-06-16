<template>
  <div class="rounded-lg border border-gray-200 p-3 dark:border-dark-600">
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
            :placeholder="t('admin.groups.modelRouting.modelPatternPlaceholder')"
          />
        </div>
        <div>
          <label class="input-label text-xs">{{
            t("admin.groups.modelRouting.accounts")
          }}</label>
          <!-- Selected accounts tags -->
          <div v-if="rule.accounts.length > 0" class="flex flex-wrap gap-1.5 mb-2">
            <span
              v-for="account in rule.accounts"
              :key="account.id"
              class="inline-flex items-center gap-1 rounded-full bg-primary-100 px-2.5 py-1 text-xs font-medium text-primary-700 dark:bg-primary-900/30 dark:text-primary-300"
            >
              {{ account.name }}
              <button
                type="button"
                @click="$emit('remove-account', account.id)"
                class="ml-0.5 text-primary-500 hover:text-primary-700 dark:hover:text-primary-200"
              >
                <Icon name="x" size="xs" />
              </button>
            </span>
          </div>
          <!-- Account search input -->
          <div class="relative account-search-container">
            <input
              :value="searchKeyword"
              type="text"
              class="input text-sm"
              :placeholder="t('admin.groups.modelRouting.searchAccountPlaceholder')"
              @input="$emit('update:keyword', ($event.target as HTMLInputElement).value); $emit('search')"
              @focus="$emit('focus')"
            />
            <!-- Search results dropdown -->
            <div
              v-if="showDropdown && searchResults?.length"
              class="absolute z-50 mt-1 max-h-48 w-full overflow-auto rounded-lg border bg-white shadow-lg dark:border-dark-600 dark:bg-dark-800"
            >
              <button
                v-for="account in searchResults"
                :key="account.id"
                type="button"
                @click="$emit('select', account)"
                class="w-full px-3 py-2 text-left text-sm hover:bg-gray-100 dark:hover:bg-dark-700"
                :class="{ 'opacity-50': rule.accounts.some((a) => a.id === account.id) }"
                :disabled="rule.accounts.some((a) => a.id === account.id)"
              >
                <span>{{ account.name }}</span>
                <span class="ml-2 text-xs text-gray-400">#{{ account.id }}</span>
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
        @click="$emit('remove-rule')"
        class="btn-icon-danger mt-5 p-1.5"
        :title="t('admin.groups.modelRouting.removeRule')"
      >
        <Icon name="trash" size="sm" />
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from "vue-i18n";
import { Icon } from "@sub2api/plugin-sdk";
import type { SimpleAccount, ModelRoutingRule } from "./modelRoutingTypes";

defineProps<{
  rule: ModelRoutingRule;
  searchKey: string;
  searchKeyword?: string;
  searchResults?: SimpleAccount[];
  showDropdown?: boolean;
}>();

defineEmits<{
  search: [];
  focus: [];
  "update:keyword": [value: string];
  select: [account: SimpleAccount];
  "remove-account": [accountId: number];
  "remove-rule": [];
}>();

const { t } = useI18n();
</script>
