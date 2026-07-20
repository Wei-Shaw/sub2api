<template>
  <div class="card">
    <div
      class="flex flex-col gap-3 border-b border-gray-100 px-6 py-4 dark:border-dark-700 lg:flex-row lg:items-start lg:justify-between"
    >
      <div>
        <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
          {{ t("admin.settings.emailTemplates.title") REDACTEDREDACTED
        </h2>
        <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
          {{ t("admin.settings.emailTemplates.description") REDACTEDREDACTED
        </p>
      </div>
      <div class="flex flex-wrap gap-2">
        <button
          type="button"
          class="btn btn-secondary btn-sm"
          :disabled="loadingTemplate || previewing || !canPreview"
          @click="refreshPreview"
        >
          {{ previewing ? t("admin.settings.emailTemplates.previewing") : t("admin.settings.emailTemplates.preview") REDACTEDREDACTED
        </button>
        <button
          type="button"
          class="btn btn-secondary btn-sm"
          :disabled="loadingTemplate || restoring || !selectedEvent || !selectedLocale"
          @click="restoreOfficial"
        >
          {{ restoring ? t("admin.settings.emailTemplates.restoring") : t("admin.settings.emailTemplates.restoreOfficial") REDACTEDREDACTED
        </button>
        <button
          type="button"
          class="btn btn-primary btn-sm"
          :disabled="loadingTemplate || saving || !canSave"
          @click="saveTemplate"
        >
          {{ saving ? t("admin.settings.emailTemplates.saving") : t("admin.settings.emailTemplates.save") REDACTEDREDACTED
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
        {{ t("common.loading") REDACTEDREDACTED
      </div>

      <template v-else>
        <div class="grid grid-cols-1 gap-4 md:grid-cols-2">
          <div>
            <label class="input-label" for="email-template-event">
              {{ t("admin.settings.emailTemplates.event") REDACTEDREDACTED
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
                {{ formatEventOptionLabel(option) REDACTEDREDACTED
              </option>
            </select>
          </div>
          <div>
            <label class="input-label" for="email-template-locale">
              {{ t("admin.settings.emailTemplates.locale") REDACTEDREDACTED
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
                {{ formatLocale(localeOption) REDACTEDREDACTED
              </option>
            </select>
          </div>
        </div>

        <div
          v-if="selectedEventMeta"
          class="rounded-lg border border-primary-100 bg-primary-50/70 p-4 dark:border-primary-900/50 dark:bg-primary-950/20"
        >
          <div class="flex flex-wrap items-center gap-2">
            <div class="text-sm font-semibold text-gray-900 dark:text-white">
              {{ selectedEventMeta.label REDACTEDREDACTED
            </div>
            <span
              class="rounded-full bg-white px-2.5 py-1 text-xs font-medium text-gray-600 shadow-sm ring-1 ring-gray-200 dark:bg-dark-800 dark:text-gray-300 dark:ring-dark-600"
            >
              {{ selectedEventMeta.categoryLabel REDACTEDREDACTED
            </span>
            <span
              class="rounded-full px-2.5 py-1 text-xs font-medium"
              :class="
                selectedEventMeta.optional
                  ? 'bg-amber-100 text-amber-800 dark:bg-amber-900/30 dark:text-amber-300'
                  : 'bg-emerald-100 text-emerald-800 dark:bg-emerald-900/30 dark:text-emerald-300'
              "
            >
              {{ selectedEventMeta.optional ? localText("可退订通知", "Optional") : localText("事务邮件", "Transactional") REDACTEDREDACTED
            </span>
          </div>
          <p class="mt-2 text-sm leading-6 text-gray-600 dark:text-gray-300">
            {{ selectedEventMeta.timing REDACTEDREDACTED
          </p>
          <p
            v-if="selectedEventDescription"
            class="mt-1 text-xs text-gray-500 dark:text-gray-400"
          >
            {{ selectedEventDescription REDACTEDREDACTED
          </p>
        </div>

        <div
          v-if="!eventOptions.length || !localeOptions.length"
          class="rounded-lg border border-amber-200 bg-amber-50 p-4 text-sm text-amber-700 dark:border-amber-800 dark:bg-amber-900/20 dark:text-amber-300"
        >
          {{ t("admin.settings.emailTemplates.empty") REDACTEDREDACTED
        </div>

        <div v-else class="grid grid-cols-1 gap-6 xl:grid-cols-2">
          <div class="space-y-4">
            <div>
              <label class="input-label" for="email-template-subject">
                {{ t("admin.settings.emailTemplates.subject") REDACTEDREDACTED
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
                {{ t("admin.settings.emailTemplates.html") REDACTEDREDACTED
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
                {{ t("admin.settings.emailTemplates.placeholders") REDACTEDREDACTED
              </div>
              <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                {{ t("admin.settings.emailTemplates.placeholdersHelp") REDACTEDREDACTED
              </p>
              <div class="mt-3 flex flex-wrap gap-2">
                <button
                  v-for="placeholder in placeholderList"
                  :key="placeholder"
                  type="button"
                  class="rounded-full border border-gray-200 bg-white px-3 py-1 font-mono text-xs text-gray-700 transition-colors hover:border-primary-300 hover:text-primary-600 dark:border-dark-600 dark:bg-dark-700 dark:text-gray-200 dark:hover:border-primary-500 dark:hover:text-primary-300"
                  @click="copyPlaceholder(placeholder)"
                >
                  {{ placeholder REDACTEDREDACTED
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
                    {{ t("admin.settings.emailTemplates.livePreview") REDACTEDREDACTED
                  </div>
                  <div class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
                    {{ previewSubject || t("admin.settings.emailTemplates.noPreview") REDACTEDREDACTED
                  </div>
                </div>
                <span
                  v-if="isCustomTemplate"
                  class="rounded-full bg-primary-50 px-2.5 py-1 text-xs font-medium text-primary-700 dark:bg-primary-900/30 dark:text-primary-300"
                >
                  {{ t("admin.settings.emailTemplates.customized") REDACTEDREDACTED
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
              {{ t("admin.settings.emailTemplates.previewSecurityHint") REDACTEDREDACTED
            </p>
          </div>
        </div>
      </template>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch REDACTED from "vue";
import { useI18n REDACTED from "vue-i18n";
import { adminAPI REDACTED from "@/api";
import type {
  EmailTemplateEventOption,
  EmailTemplateOption,
REDACTED from "@/api/admin/settings";
import { useAppStore REDACTED from "@/stores";
import { extractApiErrorMessage REDACTED from "@/utils/apiError";

const { t, locale REDACTED = useI18n();
const appStore = useAppStore();

const fallbackPlaceholders = [
  "{{site_nameREDACTEDREDACTED",
  "{{recipient_nameREDACTEDREDACTED",
  "{{recipient_emailREDACTEDREDACTED",
  "{{verification_codeREDACTEDREDACTED",
  "{{expires_in_minutesREDACTEDREDACTED",
  "{{reset_urlREDACTEDREDACTED",
  "{{subscription_groupREDACTEDREDACTED",
  "{{subscription_daysREDACTEDREDACTED",
  "{{expiry_timeREDACTEDREDACTED",
  "{{days_remainingREDACTEDREDACTED",
  "{{current_balanceREDACTEDREDACTED",
  "{{thresholdREDACTEDREDACTED",
  "{{recharge_urlREDACTEDREDACTED",
  "{{recharge_amountREDACTEDREDACTED",
  "{{order_idREDACTEDREDACTED",
  "{{unsubscribe_urlREDACTEDREDACTED",
  "{{account_idREDACTEDREDACTED",
  "{{account_nameREDACTEDREDACTED",
  "{{platformREDACTEDREDACTED",
  "{{quota_dimensionREDACTEDREDACTED",
  "{{quota_usedREDACTEDREDACTED",
  "{{quota_limitREDACTEDREDACTED",
  "{{quota_remainingREDACTEDREDACTED",
  "{{quota_thresholdREDACTEDREDACTED",
  "{{triggered_atREDACTEDREDACTED",
  "{{group_nameREDACTEDREDACTED",
  "{{moderation_categoryREDACTEDREDACTED",
  "{{moderation_scoreREDACTEDREDACTED",
  "{{violation_countREDACTEDREDACTED",
  "{{ban_thresholdREDACTEDREDACTED",
  "{{rule_nameREDACTEDREDACTED",
  "{{severityREDACTEDREDACTED",
  "{{alert_statusREDACTEDREDACTED",
  "{{metric_typeREDACTEDREDACTED",
  "{{operatorREDACTEDREDACTED",
  "{{metric_valueREDACTEDREDACTED",
  "{{threshold_valueREDACTEDREDACTED",
  "{{alert_descriptionREDACTEDREDACTED",
  "{{report_nameREDACTEDREDACTED",
  "{{report_typeREDACTEDREDACTED",
  "{{report_start_timeREDACTEDREDACTED",
  "{{report_end_timeREDACTEDREDACTED",
  "{{report_summary_displayREDACTEDREDACTED",
  "{{report_detail_displayREDACTEDREDACTED",
  "{{report_total_requestsREDACTEDREDACTED",
  "{{report_success_countREDACTEDREDACTED",
  "{{report_sla_error_countREDACTEDREDACTED",
  "{{report_business_limited_countREDACTEDREDACTED",
  "{{report_slaREDACTEDREDACTED",
  "{{report_error_rateREDACTEDREDACTED",
  "{{report_upstream_error_rateREDACTEDREDACTED",
  "{{report_upstream_error_count_excl_429_529REDACTEDREDACTED",
  "{{report_upstream_429_countREDACTEDREDACTED",
  "{{report_upstream_529_countREDACTEDREDACTED",
  "{{report_latency_p50REDACTEDREDACTED",
  "{{report_latency_p99REDACTEDREDACTED",
  "{{report_ttft_p50REDACTEDREDACTED",
  "{{report_ttft_p99REDACTEDREDACTED",
  "{{report_tokensREDACTEDREDACTED",
  "{{report_qps_currentREDACTEDREDACTED",
  "{{report_qps_peakREDACTEDREDACTED",
  "{{report_qps_avgREDACTEDREDACTED",
  "{{report_tps_currentREDACTEDREDACTED",
  "{{report_tps_peakREDACTEDREDACTED",
  "{{report_tps_avgREDACTEDREDACTED",
  "{{report_htmlREDACTEDREDACTED",
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

interface EventDisplayMeta {
  label: string;
  timing: string;
  categoryLabel: string;
REDACTED

function localText(zh: string, en: string): string {
  return locale.value.toLowerCase().startsWith("zh") ? zh : en;
REDACTED

const eventDisplayMeta: Record<string, EventDisplayMeta> = {
  "auth.verify_code": {
    label: "邮箱验证码",
    timing: "注册、绑定邮箱、OAuth 补全邮箱或 TOTP 邮箱校验时发送。",
    categoryLabel: "认证安全",
  REDACTED,
  "auth.password_reset": {
    label: "密码重置",
    timing: "用户请求密码重置链接时发送。",
    categoryLabel: "认证安全",
  REDACTED,
  "notification_email.verify_code": {
    label: "通知邮箱验证码",
    timing: "用户添加并验证额外通知邮箱时发送。",
    categoryLabel: "认证安全",
  REDACTED,
  "subscription.purchase_success": {
    label: "订阅开通成功",
    timing: "订阅订单完成支付并成功开通或续期后发送。",
    categoryLabel: "订阅",
  REDACTED,
  "subscription.expiry_reminder": {
    label: "订阅到期提醒",
    timing: "后台任务在订阅仍有效且距离到期剩余 7 天、3 天、1 天时各发送一次，可通过邮件设置中的开关关闭。",
    categoryLabel: "订阅",
  REDACTED,
  "balance.low": {
    label: "余额不足提醒",
    timing: "用户余额低于全局或个人配置的提醒阈值时发送。",
    categoryLabel: "计费",
  REDACTED,
  "balance.recharge_success": {
    label: "余额充值成功",
    timing: "余额充值订单支付完成并入账后发送。",
    categoryLabel: "计费",
  REDACTED,
  "account.quota_alert": {
    label: "账号限额告警",
    timing: "上游账号的用量达到配置的额度告警阈值时发送给管理员通知邮箱。",
    categoryLabel: "管理告警",
  REDACTED,
  "content_moderation.violation_notice": {
    label: "内容审计违规提醒",
    timing: "用户请求命中内容审计或风控规则、但尚未被禁用时发送。",
    categoryLabel: "风控",
  REDACTED,
  "content_moderation.account_disabled": {
    label: "内容审计禁用账号",
    timing: "内容审计违规次数达到封禁阈值并自动禁用用户账号时发送。",
    categoryLabel: "风控",
  REDACTED,
  "ops.alert": {
    label: "运维告警",
    timing: "运维监控规则触发告警并满足邮件通知配置时发送给运维收件人。",
    categoryLabel: "运维",
  REDACTED,
  "ops.scheduled_report": {
    label: "运维定时报表",
    timing: "运维日报、周报、错误摘要或账号健康报表到达配置的发送时间时发送；日报和周报的完整指标均可在模板中编辑。",
    categoryLabel: "运维",
  REDACTED,
REDACTED;

const eventDisplayMetaEn: Record<string, EventDisplayMeta> = {
  "auth.verify_code": {
    label: "Email Verification Code",
    timing: "Sent for registration, email binding, OAuth pending email completion, or TOTP email verification.",
    categoryLabel: "Auth",
  REDACTED,
  "auth.password_reset": {
    label: "Password Reset",
    timing: "Sent when a user requests a password reset link.",
    categoryLabel: "Auth",
  REDACTED,
  "notification_email.verify_code": {
    label: "Notification Email Verification",
    timing: "Sent when a user adds and verifies an extra notification email address.",
    categoryLabel: "Auth",
  REDACTED,
  "subscription.purchase_success": {
    label: "Subscription Activated",
    timing: "Sent after a subscription order is paid and the subscription is activated or extended.",
    categoryLabel: "Subscription",
  REDACTED,
  "subscription.expiry_reminder": {
    label: "Subscription Expiry Reminder",
    timing: "Sent by the background job when an active subscription has 7, 3, or 1 day remaining. It can be disabled in Email settings.",
    categoryLabel: "Subscription",
  REDACTED,
  "balance.low": {
    label: "Low Balance Alert",
    timing: "Sent when a user's balance drops below the global or personal reminder threshold.",
    categoryLabel: "Billing",
  REDACTED,
  "balance.recharge_success": {
    label: "Balance Recharge Success",
    timing: "Sent after a balance recharge order is paid and credited.",
    categoryLabel: "Billing",
  REDACTED,
  "account.quota_alert": {
    label: "Account Quota Alert",
    timing: "Sent to admin notification emails when an upstream account reaches the configured quota alert threshold.",
    categoryLabel: "Admin",
  REDACTED,
  "content_moderation.violation_notice": {
    label: "Risk Control Violation Notice",
    timing: "Sent when a user request triggers content moderation or risk-control rules but the account is not disabled yet.",
    categoryLabel: "Risk Control",
  REDACTED,
  "content_moderation.account_disabled": {
    label: "Risk Control Account Disabled",
    timing: "Sent when content moderation reaches the ban threshold and automatically disables the user account.",
    categoryLabel: "Risk Control",
  REDACTED,
  "ops.alert": {
    label: "Ops Alert",
    timing: "Sent to ops recipients when an ops monitoring rule fires and email notification settings allow it.",
    categoryLabel: "Ops",
  REDACTED,
  "ops.scheduled_report": {
    label: "Ops Scheduled Report",
    timing: "Sent when a configured daily, weekly, error digest, or account health report reaches its scheduled send time. Every daily and weekly summary metric is editable in this template.",
    categoryLabel: "Ops",
  REDACTED,
REDACTED;

function normalizeEventOption(option: EmailTemplateEventOption): EmailTemplateOption {
  if (typeof option === "string") {
    return { value: option REDACTED;
  REDACTED
  return option;
REDACTED

function eventMetaFor(option?: EmailTemplateOption | null) {
  if (!option) return null;
  const displayMeta = (
    locale.value.toLowerCase().startsWith("zh")
      ? eventDisplayMeta
      : eventDisplayMetaEn
  )[option.value];
  const label = displayMeta?.label || option.label || option.value;
  const timing = displayMeta?.timing || option.description || "";
  const categoryLabel =
    displayMeta?.categoryLabel || formatCategory(option.category || "");
  return {
    label,
    timing,
    categoryLabel,
    optional: option.optional === true,
  REDACTED;
REDACTED

function formatEventOptionLabel(option: EmailTemplateOption): string {
  const meta = eventMetaFor(option);
  if (!meta) return option.label || option.value;
  return meta.label;
REDACTED

function formatCategory(category: string): string {
  const normalized = category.trim().toLowerCase();
  if (!normalized) return localText("通知", "Notification");
  const labels: Record<string, { zh: string; en: string REDACTED> = {
    auth: { zh: "认证安全", en: "Auth" REDACTED,
    subscription: { zh: "订阅", en: "Subscription" REDACTED,
    billing: { zh: "计费", en: "Billing" REDACTED,
    admin: { zh: "管理告警", en: "Admin" REDACTED,
    risk_control: { zh: "风控", en: "Risk Control" REDACTED,
    ops: { zh: "运维", en: "Ops" REDACTED,
  REDACTED;
  const item = labels[normalized];
  return item ? localText(item.zh, item.en) : category;
REDACTED

const selectedEventOption = computed(() => {
  return (
    eventOptions.value.find((option) => option.value === selectedEvent.value) ||
    null
  );
REDACTED);

const selectedEventMeta = computed(() => eventMetaFor(selectedEventOption.value));

const selectedEventDescription = computed(() => {
  return (
    selectedEventOption.value?.description || ""
  );
REDACTED);

const placeholderList = computed(() => {
  const combined = placeholders.value.length
    ? placeholders.value
    : fallbackPlaceholders;
  return Array.from(
    new Set(
      combined
        .map((item) => formatPlaceholder(item))
        .filter((item) => item.length > 0),
    ),
  );
REDACTED);

function formatPlaceholder(placeholder: string): string {
  const trimmed = placeholder.trim();
  if (!trimmed) return "";
  if (trimmed.startsWith("{{") && trimmed.endsWith("REDACTEDREDACTED")) return trimmed;
  return `{{${trimmedREDACTEDREDACTEDREDACTED`;
REDACTED

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
  REDACTED
  if (lower === "en" || lower.startsWith("en-")) {
    return t("admin.settings.emailTemplates.localeEn");
  REDACTED
  return locale;
REDACTED

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
REDACTED

function applyTemplate(template: {
  subject: string;
  html: string;
  is_custom?: boolean;
  placeholders?: string[];
REDACTED) {
  subject.value = template.subject;
  html.value = template.html;
  isCustomTemplate.value = template.is_custom === true;
  placeholders.value = template.placeholders || [];
REDACTED

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
  REDACTED catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t("common.error")));
  REDACTED finally {
    loadingTemplate.value = false;
  REDACTED
REDACTED

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
  REDACTED catch (err: unknown) {
    initializingSelection.value = false;
    appStore.showError(extractApiErrorMessage(err, t("common.error")));
  REDACTED finally {
    loadingList.value = false;
  REDACTED
REDACTED

async function saveTemplate() {
  if (!canSave.value) {
    appStore.showError(t("admin.settings.emailTemplates.validationRequired"));
    return;
  REDACTED
  saving.value = true;
  try {
    const template = await adminAPI.settings.updateEmailTemplate(
      selectedEvent.value,
      selectedLocale.value,
      {
        subject: subject.value,
        html: html.value,
      REDACTED,
    );
    applyTemplate(template);
    await refreshPreview();
    appStore.showSuccess(t("admin.settings.emailTemplates.saveSuccess"));
  REDACTED catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t("common.error")));
  REDACTED finally {
    saving.value = false;
  REDACTED
REDACTED

async function refreshPreview() {
  if (!canPreview.value) {
    previewSubject.value = "";
    previewHtml.value = "";
    return;
  REDACTED
  previewing.value = true;
  try {
    const preview = await adminAPI.settings.previewEmailTemplate({
      event: selectedEvent.value,
      locale: selectedLocale.value,
      subject: subject.value,
      html: html.value,
    REDACTED);
    previewSubject.value = preview.subject;
    previewHtml.value = preview.html;
  REDACTED catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t("common.error")));
  REDACTED finally {
    previewing.value = false;
  REDACTED
REDACTED

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
  REDACTED catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t("common.error")));
  REDACTED finally {
    restoring.value = false;
  REDACTED
REDACTED

async function copyPlaceholder(placeholder: string) {
  try {
    await navigator.clipboard.writeText(placeholder);
    appStore.showSuccess(t("admin.settings.emailTemplates.placeholderCopied"));
  REDACTED catch {
    appStore.showError(t("common.error"));
  REDACTED
REDACTED

watch([selectedEvent, selectedLocale], ([eventValue, localeValue], [oldEvent, oldLocale]) => {
  if (initializingSelection.value) return;
  if (!eventValue || !localeValue) return;
  if (eventValue === oldEvent && localeValue === oldLocale) return;
  void loadTemplate();
REDACTED);

onMounted(() => {
  void loadTemplateList();
REDACTED);
</script>
