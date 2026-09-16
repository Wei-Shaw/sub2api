<template>
  <div ref="containerRef" class="relative">
    <div v-if="selectedUserIds.length > 0" class="mb-2 flex flex-wrap gap-2">
      <span
        v-for="userId in selectedUserIds"
        :key="userId"
        class="inline-flex max-w-full items-center gap-1.5 rounded-md bg-gray-100 px-2.5 py-1.5 text-xs text-gray-700 dark:bg-dark-600 dark:text-gray-200"
      >
        <span class="max-w-64 truncate font-medium" :title="selectedUserLabel(userId)">
          {{ selectedUserLabel(userId) }}
        </span>
        <span class="shrink-0 text-gray-400">#{{ userId }}</span>
        <span
          v-if="selectedUsers[userId]?.deleted"
          class="shrink-0 text-gray-400"
        >
          {{ t("admin.settings.openaiFastPolicy.userDeleted") }}
        </span>
        <button
          type="button"
          class="shrink-0 rounded text-gray-400 hover:text-red-600 dark:hover:text-red-400"
          :aria-label="t('admin.settings.openaiFastPolicy.removeUser')"
          :title="t('admin.settings.openaiFastPolicy.removeUser')"
          @click="removeUser(userId)"
        >
          <Icon name="x" size="xs" :stroke-width="2" />
        </button>
      </span>
    </div>

    <div class="relative">
      <Icon
        name="search"
        size="sm"
        class="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-gray-400"
      />
      <input
        v-model="searchQuery"
        type="text"
        autocomplete="off"
        class="input input-sm w-full pl-9"
        :placeholder="t('admin.settings.openaiFastPolicy.userSearchPlaceholder')"
        @input="debounceSearch"
        @focus="showDropdown = true"
      />
    </div>

    <div
      v-if="showDropdown && searchQuery.trim()"
      class="absolute z-50 mt-1 max-h-60 w-full overflow-auto rounded-lg border border-gray-200 bg-white shadow-lg dark:border-dark-600 dark:bg-dark-700"
    >
      <div v-if="searchLoading" class="px-4 py-3 text-sm text-gray-500 dark:text-gray-400">
        {{ t("common.loading") }}
      </div>
      <div
        v-else-if="availableResults.length === 0"
        class="px-4 py-3 text-sm text-gray-500 dark:text-gray-400"
      >
        {{ t("admin.settings.openaiFastPolicy.userSearchEmpty") }}
      </div>
      <template v-else>
        <button
          v-for="user in availableResults"
          :key="user.id"
          type="button"
          class="flex w-full items-center justify-between gap-3 px-4 py-2 text-left text-sm hover:bg-gray-100 dark:hover:bg-dark-600"
          @click="selectUser(user)"
        >
          <span class="min-w-0 truncate font-medium text-gray-900 dark:text-white">
            {{ user.email }}
            <span v-if="user.deleted" class="ml-1 text-xs font-normal text-gray-400">
              {{ t("admin.settings.openaiFastPolicy.userDeleted") }}
            </span>
          </span>
          <span class="shrink-0 text-xs text-gray-400">#{{ user.id }}</span>
        </button>
      </template>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import { adminAPI } from "@/api/admin";
import type { SimpleUser } from "@/api/admin/usage";
import Icon from "@/components/icons/Icon.vue";

const props = defineProps<{
  modelValue: number[];
}>();

const emit = defineEmits<{
  "update:modelValue": [value: number[]];
}>();

const { t } = useI18n();
const containerRef = ref<HTMLElement | null>(null);
const searchQuery = ref("");
const searchResults = ref<SimpleUser[]>([]);
const searchLoading = ref(false);
const showDropdown = ref(false);
const selectedUsers = ref<Record<number, SimpleUser>>({});
let searchTimer: ReturnType<typeof setTimeout> | null = null;
let searchSequence = 0;

const selectedUserIds = computed(() =>
  Array.from(new Set(props.modelValue.filter((id) => Number.isInteger(id) && id > 0))),
);

const availableResults = computed(() => {
  const selected = new Set(selectedUserIds.value);
  return searchResults.value
    .filter((user) => !selected.has(user.id))
    .sort((a, b) => Number(a.deleted) - Number(b.deleted));
});

function selectedUserLabel(userId: number): string {
  return selectedUsers.value[userId]?.email ||
    t("admin.settings.openaiFastPolicy.userIdFallback", { id: userId });
}

function clearPendingSearch(): void {
  if (searchTimer) {
    clearTimeout(searchTimer);
    searchTimer = null;
  }
  searchSequence += 1;
}

function debounceSearch(): void {
  clearPendingSearch();
  const query = searchQuery.value.trim();
  showDropdown.value = true;
  if (!query) {
    searchResults.value = [];
    searchLoading.value = false;
    return;
  }

  const sequence = searchSequence;
  searchTimer = setTimeout(async () => {
    searchLoading.value = true;
    try {
      const results = await adminAPI.usage.searchUsers(query);
      if (sequence === searchSequence) {
        searchResults.value = results;
      }
    } catch {
      if (sequence === searchSequence) {
        searchResults.value = [];
      }
    } finally {
      if (sequence === searchSequence) {
        searchLoading.value = false;
      }
    }
  }, 300);
}

function selectUser(user: SimpleUser): void {
  selectedUsers.value = { ...selectedUsers.value, [user.id]: user };
  emit("update:modelValue", [...selectedUserIds.value, user.id]);
  clearPendingSearch();
  searchQuery.value = "";
  searchResults.value = [];
  searchLoading.value = false;
  showDropdown.value = false;
}

function removeUser(userId: number): void {
  emit(
    "update:modelValue",
    selectedUserIds.value.filter((id) => id !== userId),
  );
}

async function hydrateSelectedUsers(userIds: number[]): Promise<void> {
  const missing = userIds.filter((id) => !selectedUsers.value[id]);
  if (missing.length === 0) return;

  const users = await Promise.all(
    missing.map(async (id) => {
      try {
        const user = await adminAPI.users.getById(id, true);
        return {
          id: user.id,
          email: user.email,
          deleted: Boolean(user.deleted_at),
        } satisfies SimpleUser;
      } catch {
        return null;
      }
    }),
  );

  const next = { ...selectedUsers.value };
  for (const user of users) {
    if (user && props.modelValue.includes(user.id)) {
      next[user.id] = user;
    }
  }
  selectedUsers.value = next;
}

function handleDocumentClick(event: MouseEvent): void {
  const target = event.target as Node | null;
  if (target && !containerRef.value?.contains(target)) {
    showDropdown.value = false;
  }
}

watch(
  selectedUserIds,
  (userIds) => {
    void hydrateSelectedUsers(userIds);
  },
  { immediate: true },
);

onMounted(() => {
  document.addEventListener("click", handleDocumentClick);
});

onUnmounted(() => {
  clearPendingSearch();
  document.removeEventListener("click", handleDocumentClick);
});
</script>
