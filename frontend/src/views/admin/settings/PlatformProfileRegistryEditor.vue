<template>
  <section class="rounded-xl border border-gray-200 bg-gray-50/80 p-4 dark:border-dark-700 dark:bg-dark-800/60">
    <div class="flex flex-col gap-3 lg:flex-row lg:items-start lg:justify-between">
      <div>
        <div class="flex flex-wrap items-center gap-2">
          <label
            for="platform-profile-registry"
            class="text-sm font-semibold text-gray-900 dark:text-white"
          >
            Platform profile registry / guide metadata
          </label>
          <span
            class="rounded-full px-2.5 py-1 text-xs font-medium"
            :class="statusBadgeClass"
            data-test="registry-status"
          >
            {{ statusLabel }}
          </span>
        </div>
        <p class="mt-1 text-xs leading-5 text-gray-500 dark:text-gray-400">
          JSON này được lưu trong bảng settings của DB. Đây là chỗ insert/sửa hướng dẫn delivery theo platform/provider;
          để trống rồi lưu sẽ tự ghi mặc định OpenAI, Anthropic, Gemini vào DB.
        </p>
      </div>

      <div class="flex flex-wrap gap-2">
        <button
          type="button"
          class="btn btn-secondary btn-sm"
          :disabled="disabled || !canFormat"
          @click="formatJSON"
        >
          <Icon name="document" size="xs" />
          Format JSON
        </button>
        <button
          type="button"
          class="btn btn-secondary btn-sm"
          :disabled="disabled || !canInsertProfile"
          @click="insertPlatformTemplate"
        >
          <Icon name="plus" size="xs" />
          Insert platform mẫu
        </button>
        <button
          type="button"
          class="btn btn-secondary btn-sm"
          :disabled="disabled"
          @click="resetToDefaultOnSave"
        >
          <Icon name="refresh" size="xs" />
          Reset mặc định 3 platform khi lưu
        </button>
      </div>
    </div>

    <div class="mt-4 grid grid-cols-1 gap-4 xl:grid-cols-[minmax(0,1.15fr)_minmax(20rem,0.85fr)]">
      <div>
        <textarea
          id="platform-profile-registry"
          :value="modelValue"
          rows="16"
          class="input w-full resize-y font-mono text-xs leading-5"
          :class="validation.ok ? '' : 'input-error ring-2 ring-red-500/20'"
          spellcheck="false"
          :disabled="disabled"
          placeholder='{"version":1,"profiles":[...]}'
          @input="handleInput"
        />

        <div
          v-if="validation.errors.length"
          class="mt-3 rounded-lg border border-red-200 bg-red-50 p-3 text-xs text-red-700 dark:border-red-900/50 dark:bg-red-950/20 dark:text-red-300"
          data-test="registry-errors"
        >
          <div class="font-semibold">Không thể lưu registry hiện tại:</div>
          <ul class="mt-1 list-disc space-y-1 pl-5">
            <li v-for="error in validation.errors" :key="error">{{ error }}</li>
          </ul>
        </div>

        <div
          v-if="validation.warnings.length"
          class="mt-3 rounded-lg border border-amber-200 bg-amber-50 p-3 text-xs text-amber-800 dark:border-amber-900/50 dark:bg-amber-950/20 dark:text-amber-300"
          data-test="registry-warnings"
        >
          <div class="font-semibold">Lưu ý:</div>
          <ul class="mt-1 list-disc space-y-1 pl-5">
            <li v-for="warning in validation.warnings" :key="warning">{{ warning }}</li>
          </ul>
        </div>

        <div class="mt-3 rounded-lg border border-gray-200 bg-white p-3 dark:border-dark-700 dark:bg-dark-900/60">
          <div class="text-xs font-semibold uppercase tracking-wide text-gray-500 dark:text-gray-400">
            Placeholder render ở user modal
          </div>
          <div class="mt-2 flex flex-wrap gap-2">
            <code
              v-for="placeholder in placeholders"
              :key="placeholder"
              class="rounded-md bg-gray-100 px-2 py-1 text-xs text-gray-700 dark:bg-dark-800 dark:text-gray-200"
            >
              {{ placeholder }}
            </code>
          </div>
        </div>
      </div>

      <aside class="space-y-4">
        <div class="rounded-lg border border-gray-200 bg-white p-4 dark:border-dark-700 dark:bg-dark-900/60">
          <div class="flex items-center justify-between gap-3">
            <div>
              <div class="text-sm font-semibold text-gray-900 dark:text-white">
                Live schema preview
              </div>
              <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                Preview này đọc trực tiếp JSON trước khi lưu DB.
              </p>
            </div>
            <span class="rounded-full bg-primary-50 px-2.5 py-1 text-xs font-medium text-primary-700 dark:bg-primary-900/30 dark:text-primary-300">
              v{{ registryVersion }}
            </span>
          </div>

          <dl class="mt-4 grid grid-cols-3 gap-2 text-center">
            <div class="rounded-lg bg-gray-50 p-2 dark:bg-dark-800">
              <dt class="text-[11px] text-gray-500 dark:text-gray-400">Platforms</dt>
              <dd class="text-base font-semibold text-gray-900 dark:text-white">{{ profileCount }}</dd>
            </div>
            <div class="rounded-lg bg-gray-50 p-2 dark:bg-dark-800">
              <dt class="text-[11px] text-gray-500 dark:text-gray-400">Clients</dt>
              <dd class="text-base font-semibold text-gray-900 dark:text-white">{{ clientCount }}</dd>
            </div>
            <div class="rounded-lg bg-gray-50 p-2 dark:bg-dark-800">
              <dt class="text-[11px] text-gray-500 dark:text-gray-400">Blocks</dt>
              <dd class="text-base font-semibold text-gray-900 dark:text-white">{{ copyBlockCount }}</dd>
            </div>
          </dl>
        </div>

        <div
          v-if="validation.isEmpty"
          class="rounded-lg border border-dashed border-gray-300 bg-white p-4 text-sm text-gray-600 dark:border-dark-600 dark:bg-dark-900/60 dark:text-gray-300"
        >
          Registry đang để trống. Khi bấm Lưu, backend sẽ normalize và insert default registry vào settings table.
        </div>

        <div
          v-else-if="validation.ok && profiles.length"
          class="space-y-3"
          data-test="registry-preview"
        >
          <article
            v-for="profile in profiles"
            :key="profile.platform || profile.guide?.profile_id"
            class="rounded-lg border border-gray-200 bg-white p-4 dark:border-dark-700 dark:bg-dark-900/60"
          >
            <div class="flex flex-wrap items-start justify-between gap-2">
              <div>
                <h3 class="font-mono text-sm font-semibold text-gray-900 dark:text-white">
                  {{ profile.platform }}
                </h3>
                <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                  {{ profile.provider_name || profile.provider_id || 'Provider chưa đặt tên' }}
                  <span v-if="profile.api_style">· {{ profile.api_style }}</span>
                </p>
              </div>
              <a
                v-if="profile.guide?.docs_url"
                class="text-xs font-medium text-primary-600 hover:text-primary-700 dark:text-primary-400"
                :href="profile.guide.docs_url"
                target="_blank"
                rel="noopener noreferrer"
              >
                Docs
              </a>
            </div>

            <div class="mt-3">
              <div class="text-sm font-medium text-gray-900 dark:text-white">
                {{ profile.guide?.title || 'Guide chưa có title' }}
              </div>
              <p class="mt-1 text-xs leading-5 text-gray-600 dark:text-gray-300">
                {{ profile.guide?.description || 'Guide chưa có description' }}
              </p>
              <p
                v-if="profile.guide?.note"
                class="mt-2 rounded-md bg-primary-50 px-3 py-2 text-xs text-primary-800 dark:bg-primary-950/30 dark:text-primary-200"
              >
                {{ profile.guide.note }}
              </p>
            </div>

            <div class="mt-3 flex flex-wrap gap-2">
              <span
                v-for="client in profileClients(profile)"
                :key="client.id"
                class="rounded-full bg-gray-100 px-2.5 py-1 text-xs text-gray-700 dark:bg-dark-800 dark:text-gray-200"
              >
                {{ client.label }}
                <span v-if="client.os?.length" class="text-gray-400">({{ client.os.join(', ') }})</span>
              </span>
            </div>

            <div class="mt-3 space-y-2">
              <div
                v-for="block in profileBlocks(profile).slice(0, 3)"
                :key="block.id || `${block.client_id}-${block.path}`"
                class="rounded-md border border-gray-100 bg-gray-50 p-2 dark:border-dark-700 dark:bg-dark-800/70"
              >
                <div class="flex flex-wrap items-center gap-2 text-xs">
                  <code class="font-semibold text-gray-900 dark:text-white">{{ block.path }}</code>
                  <span class="text-gray-400">{{ block.client_id }}</span>
                  <span v-if="block.os" class="text-gray-400">{{ block.os }}</span>
                </div>
                <pre class="mt-2 max-h-24 overflow-hidden whitespace-pre-wrap text-[11px] leading-4 text-gray-500 dark:text-gray-400">{{ previewTemplate(block.content_template || '') }}</pre>
              </div>
              <div
                v-if="profileBlocks(profile).length > 3"
                class="text-xs text-gray-500 dark:text-gray-400"
              >
                +{{ profileBlocks(profile).length - 3 }} copy block khác
              </div>
            </div>
          </article>
        </div>
      </aside>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed } from "vue";
import Icon from "@/components/icons/Icon.vue";
import {
  appendPlatformProfileTemplate,
  formatPlatformProfileRegistryInput,
  platformProfileRegistryPlaceholders,
  validatePlatformProfileRegistryInput,
} from "./platformProfileRegistry";
import type {
  PlatformGuideClient,
  PlatformGuideCopyBlock,
  PlatformProfile,
} from "./platformProfileRegistry";

interface Props {
  modelValue: string;
  disabled?: boolean;
}

const props = withDefaults(defineProps<Props>(), {
  disabled: false,
});

const emit = defineEmits<{
  (event: "update:modelValue", value: string): void;
}>();

const validation = computed(() =>
  validatePlatformProfileRegistryInput(props.modelValue || ""),
);

const profiles = computed<PlatformProfile[]>(() =>
  validation.value.registry?.profiles || [],
);

const placeholders = platformProfileRegistryPlaceholders;

const profileCount = computed(() => profiles.value.length);
const clientCount = computed(() =>
  profiles.value.reduce((total, profile) => total + profileClients(profile).length, 0),
);
const copyBlockCount = computed(() =>
  profiles.value.reduce((total, profile) => total + profileBlocks(profile).length, 0),
);
const registryVersion = computed(() => validation.value.registry?.version || 1);

const canFormat = computed(() => validation.value.ok && !validation.value.isEmpty);
const canInsertProfile = computed(() => validation.value.ok);

const statusLabel = computed(() => {
  if (validation.value.isEmpty) return "Default sẽ ghi vào DB";
  return validation.value.ok ? "Schema hợp lệ" : "Schema lỗi";
});

const statusBadgeClass = computed(() => {
  if (!validation.value.ok) {
    return "bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-300";
  }
  if (validation.value.isEmpty) {
    return "bg-amber-100 text-amber-800 dark:bg-amber-900/30 dark:text-amber-300";
  }
  return "bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300";
});

function update(value: string): void {
  emit("update:modelValue", value);
}

function handleInput(event: Event): void {
  const target = event.target as HTMLTextAreaElement | null;
  update(target?.value ?? "");
}

function resetToDefaultOnSave(): void {
  update("");
}

function formatJSON(): void {
  const formatted = formatPlatformProfileRegistryInput(props.modelValue || "");
  if (formatted) {
    update(formatted);
  }
}

function insertPlatformTemplate(): void {
  const result = appendPlatformProfileTemplate(props.modelValue || "");
  if (result) {
    update(result.value);
  }
}

function profileClients(profile: PlatformProfile): PlatformGuideClient[] {
  return Array.isArray(profile.guide?.clients) ? profile.guide.clients : [];
}

function profileBlocks(profile: PlatformProfile): PlatformGuideCopyBlock[] {
  return Array.isArray(profile.guide?.copy_blocks) ? profile.guide.copy_blocks : [];
}

function previewTemplate(template: string): string {
  const maxLength = 220;
  if (template.length <= maxLength) return template;
  return `${template.slice(0, maxLength)}…`;
}
</script>
