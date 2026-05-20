<template>
  <div class="card">
    <div
      class="flex flex-col gap-3 border-b border-gray-100 px-6 py-4 dark:border-dark-700 lg:flex-row lg:items-start lg:justify-between"
    >
      <div>
        <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
          {{ t("admin.settings.emailTemplates.title") }}
        </h2>
        <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
          {{ t("admin.settings.emailTemplates.description") }}
        </p>
      </div>
      <div class="flex flex-wrap gap-2">
        <button
          type="button"
          class="btn btn-secondary btn-sm"
          :disabled="loadingTemplate || previewing || !canPreview"
          @click="refreshPreview"
        >
          {{ previewing ? t("admin.settings.emailTemplates.previewing") : t("admin.settings.emailTemplates.preview") }}
        </button>
        <button
          type="button"
          class="btn btn-secondary btn-sm"
          :disabled="loadingTemplate || restoring || !selectedEvent || !selectedLocale"
          @click="restoreOfficial"
        >
          {{ restoring ? t("admin.settings.emailTemplates.restoring") : t("admin.settings.emailTemplates.restoreOfficial") }}
        </button>
        <button
          type="button"
          class="btn btn-primary btn-sm"
          :disabled="loadingTemplate || saving || !canSave"
          @click="saveTemplate"
        >
          {{ saving ? t("admin.settings.emailTemplates.saving") : t("admin.settings.emailTemplates.save") }}
        </button>
      </div>
    </div>

    <div class="space-y-6 p-6">
      <div
        v-if="loadingList"
        class="flex items-center gap-2 text-sm text-gray-500 dark:text-gray-400"
      >
        <span
          class="h-4 w-4 animate-spin rounded-full border-b-2 border-primary-600"
        ></span>
        {{ t("common.loading") }}
      </div>

      <template v-else>
        <div class="grid grid-cols-1 gap-4 md:grid-cols-2">
          <div>
            <label class="input-label" for="email-template-event">
              {{ t("admin.settings.emailTemplates.event") }}
            </label>
            <select
              id="email-template-event"
              v-model="selectedEvent"
              class="input"
              :disabled="loadingTemplate || eventOptions.length === 0"
            >
              <option
                v-for="option in eventOptions"
                :key="option.value"
                :value="option.value"
              >
                {{ option.label || option.value }}
              </option>
            </select>
            <p v-if="selectedEventDescription" class="input-hint">
              {{ selectedEventDescription }}
            </p>
          </div>
          <div>
            <label class="input-label" for="email-template-locale">
              {{ t("admin.settings.emailTemplates.locale") }}
            </label>
            <select
              id="email-template-locale"
              v-model="selectedLocale"
              class="input"
              :disabled="loadingTemplate || localeOptions.length === 0"
            >
              <option
                v-for="localeOption in localeOptions"
                :key="localeOption"
                :value="localeOption"
              >
                {{ formatLocale(localeOption) }}
              </option>
            </select>
          </div>
        </div>

        <div
          v-if="!eventOptions.length || !localeOptions.length"
          class="rounded-lg border border-amber-200 bg-amber-50 p-4 text-sm text-amber-700 dark:border-amber-800 dark:bg-amber-900/20 dark:text-amber-300"
        >
          {{ t("admin.settings.emailTemplates.empty") }}
        </div>

        <div v-else class="grid grid-cols-1 gap-6 xl:grid-cols-2">
          <div class="space-y-4">
            <div>
              <label class="input-label" for="email-template-subject">
                {{ t("admin.settings.emailTemplates.subject") }}
              </label>
              <input
                id="email-template-subject"
                v-model="subject"
                type="text"
                class="input"
                :disabled="loadingTemplate"
                :placeholder="t('admin.settings.emailTemplates.subjectPlaceholder')"
              />
            </div>

            <div>
              <label class="input-label" for="email-template-html">
                {{ t("admin.settings.emailTemplates.html") }}
              </label>
              <textarea
                id="email-template-html"
                v-model="html"
                rows="18"
                class="input min-h-[28rem] resize-y font-mono text-sm leading-6"
                :disabled="loadingTemplate"
                :placeholder="t('admin.settings.emailTemplates.htmlPlaceholder')"
              ></textarea>
            </div>

            <div
              class="rounded-lg border border-gray-200 bg-gray-50 p-4 dark:border-dark-700 dark:bg-dark-800/60"
            >
              <div class="text-sm font-medium text-gray-900 dark:text-white">
                {{ t("admin.settings.emailTemplates.placeholders") }}
              </div>
              <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                {{ t("admin.settings.emailTemplates.placeholdersHelp") }}
              </p>
              <div class="mt-3 flex flex-wrap gap-2">
                <button
                  v-for="placeholder in placeholderList"
                  :key="placeholder"
                  type="button"
                  class="rounded-full border border-gray-200 bg-white px-3 py-1 font-mono text-xs text-gray-700 transition-colors hover:border-primary-300 hover:text-primary-600 dark:border-dark-600 dark:bg-dark-700 dark:text-gray-200 dark:hover:border-primary-500 dark:hover:text-primary-300"
                  @click="copyPlaceholder(placeholder)"
                >
                  {{ placeholder }}
                </button>
              </div>
            </div>
          </div>

          <div class="space-y-4">
            <div
              class="rounded-lg border border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-800"
            >
              <div
                class="flex items-center justify-between border-b border-gray-100 px-4 py-3 dark:border-dark-700"
              >
                <div>
                  <div class="text-sm font-medium text-gray-900 dark:text-white">
                    {{ t("admin.settings.emailTemplates.livePreview") }}
                  </div>
                  <div class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
                    {{ previewSubject || t("admin.settings.emailTemplates.noPreview") }}
                  </div>
                </div>
                <span
                  v-if="isCustomTemplate"
                  class="rounded-full bg-primary-50 px-2.5 py-1 text-xs font-medium text-primary-700 dark:bg-primary-900/30 dark:text-primary-300"
                >
                  {{ t("admin.settings.emailTemplates.customized") }}
                </span>
              </div>
              <div class="bg-gray-100 p-3 dark:bg-dark-900">
                <iframe
                  class="h-[36rem] w-full rounded-md border border-gray-200 bg-white dark:border-dark-700"
                  sandbox=""
                  :srcdoc="previewHtml"
                  :title="t('admin.settings.emailTemplates.livePreview')"
                ></iframe>
              </div>
            </div>

            <p class="text-xs text-gray-500 dark:text-gray-400">
              {{ t("admin.settings.emailTemplates.previewSecurityHint") }}
            </p>
          </div>
        </div>
      </template>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import { adminAPI } from "@/api";
import type {
  EmailTemplateEventOption,
  EmailTemplateOption,
} from "@/api/admin/settings";
import { useAppStore } from "@/stores";
import { extractApiErrorMessage } from "@/utils/apiError";

const { t, locale } = useI18n();
const appStore = useAppStore();

const fallbackPlaceholders = [
  "{{site_name}}",
  "{{recipient_name}}",
  "{{recipient_email}}",
  "{{verification_code}}",
  "{{expires_in_minutes}}",
  "{{reset_url}}",
  "{{subscription_group}}",
  "{{subscription_days}}",
  "{{expiry_time}}",
  "{{days_remaining}}",
  "{{current_balance}}",
  "{{threshold}}",
  "{{recharge_url}}",
  "{{recharge_amount}}",
  "{{order_id}}",
  "{{unsubscribe_url}}",
  "{{account_id}}",
  "{{account_name}}",
  "{{platform}}",
  "{{quota_dimension}}",
  "{{quota_used}}",
  "{{quota_limit}}",
  "{{quota_remaining}}",
  "{{quota_threshold}}",
  "{{triggered_at}}",
  "{{group_name}}",
  "{{moderation_category}}",
  "{{moderation_score}}",
  "{{violation_count}}",
  "{{ban_threshold}}",
  "{{rule_name}}",
  "{{severity}}",
  "{{alert_status}}",
  "{{metric_type}}",
  "{{operator}}",
  "{{metric_value}}",
  "{{threshold_value}}",
  "{{alert_description}}",
  "{{report_name}}",
  "{{report_type}}",
  "{{report_start_time}}",
  "{{report_end_time}}",
  "{{report_html}}",
];

const loadingList = ref(true);
const loadingTemplate = ref(false);
const saving = ref(false);
const previewing = ref(false);
const restoring = ref(false);
const eventOptions = ref<EmailTemplateOption[]>([]);
const localeOptions = ref<string[]>([]);
const selectedEvent = ref("");
const selectedLocale = ref("");
const subject = ref("");
const html = ref("");
const isCustomTemplate = ref(false);
const placeholders = ref<string[]>([]);
const previewSubject = ref("");
const previewHtml = ref("");
const initializingSelection = ref(false);

function normalizeEventOption(option: EmailTemplateEventOption): EmailTemplateOption {
  if (typeof option === "string") {
    return { value: option };
  }
  return option;
}

const selectedEventDescription = computed(() => {
  return (
    eventOptions.value.find((option) => option.value === selectedEvent.value)
      ?.description || ""
  );
});

const placeholderList = computed(() => {
  const combined = [...placeholders.value, ...fallbackPlaceholders];
  return Array.from(
    new Set(
      combined
        .map((item) => formatPlaceholder(item))
        .filter((item) => item.length > 0),
    ),
  );
});

function formatPlaceholder(placeholder: string): string {
  const trimmed = placeholder.trim();
  if (!trimmed) return "";
  if (trimmed.startsWith("{{") && trimmed.endsWith("}}")) return trimmed;
  return `{{${trimmed}}}`;
}

const canSave = computed(
  () =>
    Boolean(selectedEvent.value && selectedLocale.value) &&
    subject.value.trim().length > 0 &&
    html.value.trim().length > 0,
);

const canPreview = computed(
  () => Boolean(selectedEvent.value && selectedLocale.value) && html.value.trim().length > 0,
);

function formatLocale(locale: string): string {
  const lower = locale.toLowerCase();
  if (lower === "zh" || lower.startsWith("zh-")) {
    return t("admin.settings.emailTemplates.localeZh");
  }
  if (lower === "en" || lower.startsWith("en-")) {
    return t("admin.settings.emailTemplates.localeEn");
  }
  return locale;
}

function selectInitialLocale(locales: string[]): string {
  const currentLocale = locale.value.toLowerCase();
  const exactMatch = locales.find(
    (availableLocale) => availableLocale.toLowerCase() === currentLocale,
  );
  if (exactMatch) return exactMatch;

  const currentLanguage = currentLocale.split("-")[0];
  const languageMatch = locales.find(
    (availableLocale) => availableLocale.toLowerCase().split("-")[0] === currentLanguage,
  );
  if (languageMatch) return languageMatch;

  return locales[0] || "";
}

function applyTemplate(template: {
  subject: string;
  html: string;
  is_custom?: boolean;
  placeholders?: string[];
}) {
  subject.value = template.subject;
  html.value = template.html;
  isCustomTemplate.value = template.is_custom === true;
  placeholders.value = template.placeholders || [];
}

async function loadTemplate() {
  if (!selectedEvent.value || !selectedLocale.value) return;
  loadingTemplate.value = true;
  try {
    const template = await adminAPI.settings.getEmailTemplate(
      selectedEvent.value,
      selectedLocale.value,
    );
    applyTemplate(template);
    await refreshPreview();
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t("common.error")));
  } finally {
    loadingTemplate.value = false;
  }
}

async function loadTemplateList() {
  loadingList.value = true;
  try {
    const response = await adminAPI.settings.getEmailTemplates();
    eventOptions.value = response.events.map(normalizeEventOption);
    localeOptions.value = response.locales;
    placeholders.value = response.placeholders || [];
    initializingSelection.value = true;
    selectedEvent.value = eventOptions.value[0]?.value || "";
    selectedLocale.value = selectInitialLocale(response.locales);
    await loadTemplate();
    initializingSelection.value = false;
  } catch (err: unknown) {
    initializingSelection.value = false;
    appStore.showError(extractApiErrorMessage(err, t("common.error")));
  } finally {
    loadingList.value = false;
  }
}

async function saveTemplate() {
  if (!canSave.value) {
    appStore.showError(t("admin.settings.emailTemplates.validationRequired"));
    return;
  }
  saving.value = true;
  try {
    const template = await adminAPI.settings.updateEmailTemplate(
      selectedEvent.value,
      selectedLocale.value,
      {
        subject: subject.value,
        html: html.value,
      },
    );
    applyTemplate(template);
    await refreshPreview();
    appStore.showSuccess(t("admin.settings.emailTemplates.saveSuccess"));
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t("common.error")));
  } finally {
    saving.value = false;
  }
}

async function refreshPreview() {
  if (!canPreview.value) {
    previewSubject.value = "";
    previewHtml.value = "";
    return;
  }
  previewing.value = true;
  try {
    const preview = await adminAPI.settings.previewEmailTemplate({
      event: selectedEvent.value,
      locale: selectedLocale.value,
      subject: subject.value,
      html: html.value,
    });
    previewSubject.value = preview.subject;
    previewHtml.value = preview.html;
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t("common.error")));
  } finally {
    previewing.value = false;
  }
}

async function restoreOfficial() {
  if (!selectedEvent.value || !selectedLocale.value) return;
  if (!window.confirm(t("admin.settings.emailTemplates.restoreConfirm"))) return;

  restoring.value = true;
  try {
    const template = await adminAPI.settings.restoreOfficialEmailTemplate(
      selectedEvent.value,
      selectedLocale.value,
    );
    applyTemplate(template);
    await refreshPreview();
    appStore.showSuccess(t("admin.settings.emailTemplates.restoreSuccess"));
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t("common.error")));
  } finally {
    restoring.value = false;
  }
}

async function copyPlaceholder(placeholder: string) {
  try {
    await navigator.clipboard.writeText(placeholder);
    appStore.showSuccess(t("admin.settings.emailTemplates.placeholderCopied"));
  } catch {
    appStore.showError(t("common.error"));
  }
}

watch([selectedEvent, selectedLocale], ([eventValue, localeValue], [oldEvent, oldLocale]) => {
  if (initializingSelection.value) return;
  if (!eventValue || !localeValue) return;
  if (eventValue === oldEvent && localeValue === oldLocale) return;
  void loadTemplate();
});

onMounted(() => {
  void loadTemplateList();
});
</script>
