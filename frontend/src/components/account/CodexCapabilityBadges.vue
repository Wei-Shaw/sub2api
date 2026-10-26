<template>
  <div
    v-if="applicable"
    class="flex flex-wrap items-center gap-1 pl-0.5 text-[11px] leading-4"
    :title="title"
    data-testid="codex-capability-badges"
  >
    <template v-if="capabilities">
      <span
        v-for="badge in badges"
        :key="badge.key"
        :class="['inline-block rounded px-1.5 py-0.5 font-medium', badge.className]"
        :data-testid="`codex-capability-${badge.key}`"
      >
        {{ badge.label }}
      </span>
      <span
        v-if="badges.length === 0"
        class="text-slate-500 dark:text-slate-400"
        data-testid="codex-capability-none"
      >
        {{ t("admin.accounts.codexCapabilities.none") }}
      </span>
      <span
        v-if="syncedAtLabel"
        class="text-slate-400 dark:text-slate-500"
        data-testid="codex-capability-synced-at"
      >
        {{ t("admin.accounts.codexCapabilities.syncedAt") }} {{ syncedAtLabel }}
      </span>
    </template>
    <span
      v-else
      class="text-slate-500 dark:text-slate-400"
      data-testid="codex-capability-unprobed"
    >
      {{ t("admin.accounts.codexCapabilities.unprobed") }}
    </span>
  </div>
</template>

<script setup lang="ts">
import { computed } from "vue";
import { useI18n } from "vue-i18n";
import type { Account, CodexAccountCapabilities } from "@/types";
import { formatDateTime } from "@/utils/format";

const props = defineProps<{ account: Pick<Account, "platform" | "type" | "extra"> }>();
const { t } = useI18n();

// 只有 ChatGPT/Codex 协议的 OpenAI OAuth-like 账号有探测来源。
const applicable = computed(
  () => props.account.platform === "openai" && (props.account.type === "oauth" || props.account.type === "setup-token"),
);

const capabilities = computed<CodexAccountCapabilities | null>(() => {
  const raw = (props.account.extra as Record<string, unknown> | undefined)?.codex_capabilities;
  if (!raw || typeof raw !== "object") return null;
  const record = raw as CodexAccountCapabilities;
  if (!record.models || Object.keys(record.models).length === 0) return null;
  return record;
});

const badgeClass: Record<string, string> = {
  daybreak_blue: "bg-blue-100 text-blue-700 dark:bg-blue-900/40 dark:text-blue-300",
  daybreak_red: "bg-rose-100 text-rose-700 dark:bg-rose-900/40 dark:text-rose-300",
  priority: "bg-emerald-100 text-emerald-700 dark:bg-emerald-900/40 dark:text-emerald-300",
  ultrafast: "bg-amber-100 text-amber-700 dark:bg-amber-900/40 dark:text-amber-300",
};

const badges = computed(() => {
  const record = capabilities.value;
  if (!record) return [];
  const out: { key: string; label: string; className: string }[] = [];
  for (const program of ["daybreak_blue", "daybreak_red"]) {
    if (record.cyber_programs?.includes(program)) {
      out.push({ key: program, label: t(`admin.accounts.codexCapabilities.badges.${program}`), className: badgeClass[program] });
    }
  }
  for (const tier of ["priority", "ultrafast"]) {
    if (record.service_tiers?.includes(tier)) {
      out.push({ key: tier, label: t(`admin.accounts.codexCapabilities.badges.${tier}`), className: badgeClass[tier] });
    }
  }
  return out;
});

const syncedAtLabel = computed(() => {
  const record = capabilities.value;
  if (!record?.synced_at) return "";
  const date = new Date(record.synced_at);
  return Number.isNaN(date.getTime()) ? "" : formatDateTime(date);
});

const title = computed(() => {
  if (!capabilities.value) return t("admin.accounts.codexCapabilities.unprobedHint");
  return syncedAtLabel.value
    ? `${t("admin.accounts.codexCapabilities.title")} | ${t("admin.accounts.codexCapabilities.syncedAt")}: ${syncedAtLabel.value}`
    : t("admin.accounts.codexCapabilities.title");
});
</script>
