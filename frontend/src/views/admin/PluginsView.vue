<template>
  <AppLayout>
    <div class="space-y-6">
      <section
        class="flex flex-col gap-4 border-b border-gray-200 pb-5 dark:border-dark-700 sm:flex-row sm:items-end sm:justify-between"
      >
        <div class="min-w-0">
          <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
            {{ t("admin.plugins.title") REDACTEDREDACTED
          </h2>
          <p class="mt-1 max-w-3xl text-sm text-gray-500 dark:text-gray-400">
            {{ t("admin.plugins.description") REDACTEDREDACTED
          </p>
          <div
            class="mt-3 flex flex-wrap gap-2 text-xs text-gray-600 dark:text-gray-300"
          >
            <span class="rounded bg-gray-100 px-2 py-1 dark:bg-dark-700">{{
              t("admin.plugins.onlyOpenAI")
            REDACTEDREDACTED</span>
            <span class="rounded bg-gray-100 px-2 py-1 dark:bg-dark-700">{{
              t("admin.plugins.noAccountCoupling")
            REDACTEDREDACTED</span>
          </div>
        </div>

        <div class="flex flex-shrink-0 items-center gap-2">
          <input
            ref="fileInput"
            class="hidden"
            type="file"
            accept=".s2plugin,application/zip"
            @change="handleFileSelected"
          />
          <button
            type="button"
            class="btn btn-primary"
            :disabled="uploading"
            @click="fileInput?.click()"
          >
            <Icon name="upload" size="sm" />
            {{ uploading ? t("common.processing") : t("admin.plugins.upload") REDACTEDREDACTED
          </button>
          <button
            type="button"
            class="btn btn-secondary"
            :disabled="loading"
            :title="t('common.refresh')"
            @click="loadPlugins"
          >
            <Icon name="refresh" size="sm" />
            <span class="sr-only">{{ t("common.refresh") REDACTEDREDACTED</span>
          </button>
        </div>
      </section>

      <p class="text-xs text-gray-500 dark:text-gray-400">
        {{ t("admin.plugins.uploadHint") REDACTEDREDACTED
      </p>

      <div
        class="border border-blue-200 bg-blue-50 px-4 py-3 text-sm text-blue-800 dark:border-blue-900/60 dark:bg-blue-950/30 dark:text-blue-200"
      >
        <p>{{ t("admin.plugins.runtimeNotice") REDACTEDREDACTED</p>
        <p class="mt-1">{{ t("admin.plugins.menuNotice") REDACTEDREDACTED</p>
      </div>

      <div
        v-if="loading"
        class="flex min-h-48 items-center justify-center text-sm text-gray-500"
      >
        {{ t("common.loading") REDACTEDREDACTED
      </div>

      <div
        v-else-if="plugins.length === 0"
        class="flex min-h-56 flex-col items-center justify-center border border-dashed border-gray-300 px-6 text-center dark:border-dark-600"
      >
        <Icon name="cube" size="xl" class="text-gray-400" />
        <p class="mt-3 font-medium text-gray-800 dark:text-gray-200">
          {{ t("admin.plugins.empty") REDACTEDREDACTED
        </p>
        <p class="mt-1 max-w-lg text-sm text-gray-500 dark:text-gray-400">
          {{ t("admin.plugins.emptyHint") REDACTEDREDACTED
        </p>
      </div>

      <div v-else class="grid grid-cols-1 gap-4 xl:grid-cols-2">
        <article
          v-for="plugin in plugins"
          :key="plugin.id"
          class="card overflow-hidden border border-gray-200 dark:border-dark-700"
        >
          <div
            class="flex flex-wrap items-start justify-between gap-3 border-b border-gray-100 p-5 dark:border-dark-700"
          >
            <div class="min-w-0">
              <div class="flex flex-wrap items-center gap-2">
                <h3
                  class="truncate text-base font-semibold text-gray-900 dark:text-white"
                >
                  {{ plugin.name REDACTEDREDACTED
                </h3>
                <span class="font-mono text-xs text-gray-500"
                  >v{{ plugin.version REDACTEDREDACTED</span
                >
                <span
                  class="rounded px-2 py-0.5 text-xs font-medium"
                  :class="stateClass(plugin.state)"
                >
                  {{ t(`admin.plugins.${plugin.stateREDACTED`) REDACTEDREDACTED
                </span>
              </div>
              <p class="mt-1 text-xs text-gray-500">
                {{ plugin.plugin_key
                REDACTEDREDACTED<span v-if="plugin.author"> · {{ plugin.author REDACTEDREDACTED</span>
              </p>
              <p
                v-if="plugin.description"
                class="mt-2 text-sm text-gray-600 dark:text-gray-300"
              >
                {{ plugin.description REDACTEDREDACTED
              </p>
            </div>
            <button
              type="button"
              class="btn btn-secondary btn-sm"
              @click="openConfiguration(plugin)"
            >
              <Icon name="cog" size="sm" />
              {{ t("admin.plugins.configure") REDACTEDREDACTED
            </button>
          </div>

          <div class="grid grid-cols-1 gap-x-6 gap-y-4 p-5 md:grid-cols-2">
            <div>
              <p class="text-xs font-medium uppercase text-gray-500">
                {{ t("admin.plugins.compatibility") REDACTEDREDACTED
              </p>
              <div class="mt-2 flex items-center gap-2">
                <span
                  class="rounded px-2 py-0.5 text-xs font-medium"
                  :class="compatibilityClass(plugin.compatibility.status)"
                >
                  {{ t(`admin.plugins.${plugin.compatibility.statusREDACTED`) REDACTEDREDACTED
                </span>
                <span class="text-xs text-gray-500 dark:text-gray-400">{{
                  plugin.compatibility.message
                REDACTEDREDACTED</span>
              </div>
              <dl
                class="mt-3 grid grid-cols-[auto,1fr] gap-x-3 gap-y-1 text-xs"
              >
                <dt class="text-gray-500">
                  {{ t("admin.plugins.currentVersion") REDACTEDREDACTED
                </dt>
                <dd class="font-mono text-gray-800 dark:text-gray-200">
                  {{ plugin.compatibility.current_sub2api_version REDACTEDREDACTED
                </dd>
                <dt class="text-gray-500">
                  {{ t("admin.plugins.requiredVersion") REDACTEDREDACTED
                </dt>
                <dd class="font-mono text-gray-800 dark:text-gray-200">
                  {{ plugin.compatibility.required_sub2api_version REDACTEDREDACTED
                </dd>
                <dt class="text-gray-500">
                  {{ t("admin.plugins.recommendedVersion") REDACTEDREDACTED
                </dt>
                <dd class="font-mono text-gray-800 dark:text-gray-200">
                  {{ plugin.compatibility.recommended_sub2api_version || "-" REDACTEDREDACTED
                </dd>
              </dl>
            </div>

            <div>
              <p class="text-xs font-medium uppercase text-gray-500">
                {{ t("admin.plugins.runtime") REDACTEDREDACTED
              </p>
              <div class="mt-2 flex flex-wrap gap-2 text-xs">
                <span
                  class="rounded px-2 py-0.5"
                  :class="
                    plugin.runtime_healthy
                      ? 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300'
                      : 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-300'
                  "
                >
                  {{
                    plugin.runtime_healthy
                      ? t("admin.plugins.healthy")
                      : t("admin.plugins.unhealthy")
                  REDACTEDREDACTED
                </span>
                <span
                  class="rounded bg-gray-100 px-2 py-0.5 text-gray-600 dark:bg-dark-700 dark:text-gray-300"
                >
                  {{ t("admin.plugins.signature") REDACTEDREDACTED:
                  {{ t(`admin.plugins.${plugin.signature_statusREDACTED`) REDACTEDREDACTED
                </span>
              </div>
              <p
                v-if="plugin.last_error"
                class="mt-3 break-words text-xs text-red-600 dark:text-red-400"
              >
                {{ plugin.last_error REDACTEDREDACTED
              </p>
              <p
                v-else-if="plugin.runtime_message"
                class="mt-3 break-words text-xs text-gray-500"
              >
                {{ plugin.runtime_message REDACTEDREDACTED
              </p>
            </div>

            <div class="md:col-span-2">
              <label
                class="flex items-center justify-between gap-4 text-xs font-medium text-gray-600 dark:text-gray-300"
              >
                <span>{{ t("admin.plugins.rollout") REDACTEDREDACTED</span>
                <span class="w-11 text-right font-mono"
                  >{{
                    rolloutValues[plugin.id] ?? currentRollout(plugin)
                  REDACTEDREDACTED%</span
                >
              </label>
              <input
                :value="rolloutValues[plugin.id] ?? currentRollout(plugin)"
                type="range"
                min="1"
                max="100"
                step="1"
                class="mt-2 w-full accent-primary-600"
                :disabled="hasEnabledBinding(plugin)"
                @input="setRollout(plugin.id, $event)"
              />
            </div>
          </div>

          <div
            class="flex flex-wrap justify-end gap-2 border-t border-gray-100 px-5 py-4 dark:border-dark-700"
          >
            <button
              type="button"
              class="btn btn-secondary btn-sm"
              :disabled="busyID === plugin.id"
              @click="testPlugin(plugin)"
            >
              <Icon name="beaker" size="sm" />
              {{ t("admin.plugins.test") REDACTEDREDACTED
            </button>
            <button
              v-if="hasEnabledBinding(plugin)"
              type="button"
              class="btn btn-secondary btn-sm"
              :disabled="busyID === plugin.id"
              @click="disablePlugin(plugin)"
            >
              <Icon name="ban" size="sm" />
              {{ t("admin.plugins.disable") REDACTEDREDACTED
            </button>
            <button
              v-else
              type="button"
              class="btn btn-primary btn-sm"
              :disabled="
                busyID === plugin.id ||
                plugin.state === 'starting' ||
                !plugin.compatibility.compatible
              "
              @click="enablePlugin(plugin)"
            >
              <Icon name="play" size="sm" />
              {{ t("admin.plugins.enable") REDACTEDREDACTED
            </button>
            <button
              type="button"
              class="btn btn-danger btn-sm"
              :disabled="busyID === plugin.id || hasEnabledBinding(plugin)"
              @click="uninstallPlugin(plugin)"
            >
              <Icon name="trash" size="sm" />
              {{ t("admin.plugins.uninstall") REDACTEDREDACTED
            </button>
          </div>
        </article>
      </div>

      <BaseDialog
        :show="configPlugin !== null"
        :title="
          t('admin.plugins.configTitle', { name: configPlugin?.name || '' REDACTED)
        "
        width="full"
        @close="closeConfiguration"
      >
        <div
          class="relative min-h-[520px] overflow-hidden bg-gray-50 dark:bg-dark-900"
          :style="{ height: `${iframeHeightREDACTEDpx` REDACTED"
        >
          <div
            v-if="uiLoading"
            class="absolute inset-0 z-10 flex items-center justify-center text-sm text-gray-500"
          >
            {{ t("admin.plugins.loadingUI") REDACTEDREDACTED
          </div>
          <div
            v-if="uiError"
            class="absolute inset-0 z-20 flex flex-col items-center justify-center p-8 text-center"
          >
            <Icon name="exclamationTriangle" size="xl" class="text-amber-500" />
            <p class="mt-3 font-medium text-gray-800 dark:text-gray-200">
              {{ t("admin.plugins.uiUnavailable") REDACTEDREDACTED
            </p>
            <p class="mt-1 max-w-xl text-sm text-gray-500">{{ uiError REDACTEDREDACTED</p>
          </div>
          <iframe
            v-if="uiSession"
            ref="pluginFrame"
            :src="uiSession.url"
            sandbox="allow-scripts"
            referrerpolicy="no-referrer"
            class="h-full w-full border-0 bg-white dark:bg-dark-900"
            :title="
              t('admin.plugins.configTitle', { name: configPlugin?.name || '' REDACTED)
            "
            @load="handlePluginFrameLoad"
          />
        </div>
      </BaseDialog>

      <TotpStepUpDialog :controller="pluginStepUp" />
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref REDACTED from "vue";
import { useI18n REDACTED from "vue-i18n";
import {
  adminAPI,
  type PluginInstallation,
  type PluginUISession,
REDACTED from "@/api/admin";
import { useAppStore REDACTED from "@/stores";
import AppLayout from "@/components/layout/AppLayout.vue";
import BaseDialog from "@/components/common/BaseDialog.vue";
import Icon from "@/components/icons/Icon.vue";
import TotpStepUpDialog from "@/components/auth/TotpStepUpDialog.vue";
import {
  isStepUpBlocked,
  isStepUpCancelled,
  stepUpBlockReason,
  useStepUp,
REDACTED from "@/composables/useStepUp";

interface PluginBridgeMessage {
  source?: string;
  bridge_token?: string;
  type?: string;
  request_id?: string;
  config?: unknown;
  height?: unknown;
  level?: unknown;
  message?: unknown;
REDACTED

const { t REDACTED = useI18n();
const appStore = useAppStore();
const pluginStepUp = useStepUp();
const plugins = ref<PluginInstallation[]>([]);
const loading = ref(false);
const uploading = ref(false);
const busyID = ref<number | null>(null);
const fileInput = ref<HTMLInputElement | null>(null);
const rolloutValues = ref<Record<number, number>>({REDACTED);
const configPlugin = ref<PluginInstallation | null>(null);
const uiSession = ref<PluginUISession | null>(null);
const pluginFrame = ref<HTMLIFrameElement | null>(null);
const uiLoading = ref(false);
const uiError = ref("");
const iframeHeight = ref(640);
const pendingBridgeRequests = new Map<string, number>();

function errorMessage(error: unknown): string {
  if (typeof error === "object" && error !== null && "message" in error) {
    return String(
      (error as { message?: unknown REDACTED).message || t("common.unknownError"),
    );
  REDACTED
  return t("common.unknownError");
REDACTED

function reportSensitiveActionError(error: unknown): void {
  if (isStepUpCancelled(error)) return;
  if (isStepUpBlocked(error)) {
    appStore.showError(
      stepUpBlockReason(error) === "STEP_UP_ADMIN_API_KEY_FORBIDDEN"
        ? t("stepUp.adminApiKeyForbidden")
        : t("stepUp.notEnabled"),
    );
    return;
  REDACTED
  appStore.showError(errorMessage(error));
REDACTED

async function loadPlugins(): Promise<void> {
  loading.value = true;
  try {
    plugins.value = await adminAPI.plugins.list();
    for (const plugin of plugins.value) {
      rolloutValues.value[plugin.id] = currentRollout(plugin);
    REDACTED
  REDACTED catch (error: unknown) {
    appStore.showError(errorMessage(error));
  REDACTED finally {
    loading.value = false;
  REDACTED
REDACTED

async function handleFileSelected(event: Event): Promise<void> {
  const target = event.target as HTMLInputElement;
  const file = target.files?.[0];
  target.value = "";
  if (!file || !file.name.toLowerCase().endsWith(".s2plugin")) {
    appStore.showError(t("admin.plugins.fileRequired"));
    return;
  REDACTED
  uploading.value = true;
  try {
    await pluginStepUp.run(() => adminAPI.plugins.upload(file));
    appStore.showSuccess(t("admin.plugins.uploadSuccess"));
    await loadPlugins();
  REDACTED catch (error: unknown) {
    reportSensitiveActionError(error);
  REDACTED finally {
    uploading.value = false;
  REDACTED
REDACTED

function currentRollout(plugin: PluginInstallation): number {
  return (
    plugin.bindings.find(
      (binding) => binding.capability === "openai.oauth.outbound_transport.v1",
    )?.rollout_percent || 100
  );
REDACTED

function hasEnabledBinding(plugin: PluginInstallation): boolean {
  return plugin.bindings.some((binding) => binding.enabled);
REDACTED

function setRollout(id: number, event: Event): void {
  const value = Number((event.target as HTMLInputElement).value);
  rolloutValues.value[id] = Math.min(100, Math.max(1, value));
REDACTED

async function enablePlugin(plugin: PluginInstallation): Promise<void> {
  let acceptUntested = false;
  if (!plugin.compatibility.tested) {
    acceptUntested = window.confirm(t("admin.plugins.confirmUntested"));
    if (!acceptUntested) return;
  REDACTED
  busyID.value = plugin.id;
  try {
    await pluginStepUp.run(() =>
      adminAPI.plugins.enable(
        plugin.id,
        rolloutValues.value[plugin.id] || 100,
        acceptUntested,
      ),
    );
    appStore.showSuccess(t("admin.plugins.enableSuccess"));
    await loadPlugins();
  REDACTED catch (error: unknown) {
    reportSensitiveActionError(error);
  REDACTED finally {
    busyID.value = null;
  REDACTED
REDACTED

async function disablePlugin(plugin: PluginInstallation): Promise<void> {
  if (!window.confirm(t("admin.plugins.confirmDisable"))) return;
  busyID.value = plugin.id;
  try {
    await pluginStepUp.run(() => adminAPI.plugins.disable(plugin.id));
    appStore.showSuccess(t("admin.plugins.disableSuccess"));
    await loadPlugins();
  REDACTED catch (error: unknown) {
    reportSensitiveActionError(error);
  REDACTED finally {
    busyID.value = null;
  REDACTED
REDACTED

async function uninstallPlugin(plugin: PluginInstallation): Promise<void> {
  if (!window.confirm(t("admin.plugins.confirmUninstall"))) return;
  busyID.value = plugin.id;
  try {
    await pluginStepUp.run(() => adminAPI.plugins.remove(plugin.id));
    appStore.showSuccess(t("admin.plugins.uninstallSuccess"));
    await loadPlugins();
  REDACTED catch (error: unknown) {
    reportSensitiveActionError(error);
  REDACTED finally {
    busyID.value = null;
  REDACTED
REDACTED

async function testPlugin(plugin: PluginInstallation): Promise<void> {
  busyID.value = plugin.id;
  try {
    const result = await pluginStepUp.run(() =>
      adminAPI.plugins.test(plugin.id),
    );
    if (result.success)
      appStore.showSuccess(result.message || t("admin.plugins.testSuccess"));
    else appStore.showError(result.message || t("common.error"));
  REDACTED catch (error: unknown) {
    reportSensitiveActionError(error);
  REDACTED finally {
    busyID.value = null;
  REDACTED
REDACTED

async function openConfiguration(plugin: PluginInstallation): Promise<void> {
  configPlugin.value = plugin;
  uiSession.value = null;
  uiLoading.value = true;
  uiError.value = "";
  iframeHeight.value = 640;
  try {
    uiSession.value = await adminAPI.plugins.createUISession(plugin.id);
  REDACTED catch (error: unknown) {
    uiLoading.value = false;
    uiError.value = errorMessage(error);
  REDACTED
REDACTED

function closeConfiguration(): void {
  clearPendingBridgeRequests();
  configPlugin.value = null;
  uiSession.value = null;
  uiLoading.value = false;
  uiError.value = "";
REDACTED

function clearPendingBridgeRequests(): void {
  for (const timeout of pendingBridgeRequests.values()) window.clearTimeout(timeout);
  pendingBridgeRequests.clear();
REDACTED

function handlePluginFrameLoad(): void {
  // A load can also be caused by a plugin navigating its iframe. Drop all
  // outstanding responses so a late config response is never sent to the new document.
  clearPendingBridgeRequests();
  uiLoading.value = false;
REDACTED

function registerBridgeRequest(requestID: string): void {
  const timeout = window.setTimeout(() => {
    pendingBridgeRequests.delete(requestID);
  REDACTED, 30_000);
  pendingBridgeRequests.set(requestID, timeout);
REDACTED

function postBridgeResult(
  request: PluginBridgeMessage,
  payload: Record<string, unknown>,
): void {
  if (!pluginFrame.value?.contentWindow || !uiSession.value) return;
  const requestID = typeof request.request_id === "string" ? request.request_id.trim() : "";
  const timeout = pendingBridgeRequests.get(requestID);
  if (!requestID || timeout === undefined) return;
  window.clearTimeout(timeout);
  pendingBridgeRequests.delete(requestID);
  pluginFrame.value.contentWindow.postMessage(
    {
      source: "sub2api-plugin-host",
      bridge_token: uiSession.value.bridge_token,
      type: `${request.typeREDACTED.result`,
      request_id: requestID,
      ...payload,
    REDACTED,
    // The sandboxed iframe has an opaque origin, so no fixed target origin exists.
    // Pending request tracking plus load invalidation prevents cross-navigation leaks.
    "*",
  );
REDACTED

async function handleBridgeMessage(event: MessageEvent): Promise<void> {
  if (
    !uiSession.value ||
    !configPlugin.value ||
    event.source !== pluginFrame.value?.contentWindow ||
    event.origin !== "null"
  )
    return;
  const message = event.data as PluginBridgeMessage;
  if (
    !message ||
    message.source !== "sub2api-plugin-ui" ||
    message.bridge_token !== uiSession.value.bridge_token
  )
    return;

  const requestID = typeof message.request_id === "string" ? message.request_id.trim() : "";
  const expectsResponse =
    message.type === "config.load" ||
    message.type === "config.save" ||
    message.type === "config.test";
  if (expectsResponse) {
    if (!requestID || pendingBridgeRequests.has(requestID)) return;
    registerBridgeRequest(requestID);
  REDACTED

  try {
    switch (message.type) {
      case "sub2api.plugin.ready":
        uiLoading.value = false;
        break;
      case "config.load": {
        const config = await adminAPI.plugins.getConfig(configPlugin.value.id);
        postBridgeResult(message, { ok: true, config REDACTED);
        break;
      REDACTED
      case "config.save": {
        if (
          !message.config ||
          typeof message.config !== "object" ||
          Array.isArray(message.config)
        ) {
          throw new Error(t("admin.plugins.bridgeRejected"));
        REDACTED
        const config = await pluginStepUp.run(() =>
          adminAPI.plugins.saveConfig(
            configPlugin.value!.id,
            message.config as Record<string, unknown>,
          ),
        );
        postBridgeResult(message, { ok: true, config REDACTED);
        appStore.showSuccess(t("common.saved"));
        break;
      REDACTED
      case "config.test": {
        const result = await pluginStepUp.run(() =>
          adminAPI.plugins.test(configPlugin.value!.id),
        );
        postBridgeResult(message, { ok: result.success, result REDACTED);
        if (result.success)
          appStore.showSuccess(
            result.message || t("admin.plugins.testSuccess"),
          );
        else appStore.showError(result.message || t("common.error"));
        break;
      REDACTED
      case "ui.resize": {
        const height = Number(message.height);
        if (Number.isFinite(height))
          iframeHeight.value = Math.min(960, Math.max(520, Math.round(height)));
        break;
      REDACTED
      case "ui.notify": {
        const text =
          typeof message.message === "string"
            ? message.message.slice(0, 500)
            : "";
        if (!text) break;
        if (message.level === "error") appStore.showError(text);
        else if (message.level === "success") appStore.showSuccess(text);
        else appStore.showInfo(text);
        break;
      REDACTED
    REDACTED
  REDACTED catch (error: unknown) {
    if (isStepUpBlocked(error)) reportSensitiveActionError(error);
    postBridgeResult(message, {
      ok: false,
      error: isStepUpCancelled(error) ? t("common.cancel") : errorMessage(error),
    REDACTED);
  REDACTED
REDACTED

function stateClass(state: PluginInstallation["state"]): string {
  if (state === "enabled")
    return "bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300";
  if (state === "error" || state === "incompatible")
    return "bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-300";
  if (state === "starting")
    return "bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300";
  return "bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-300";
REDACTED

function compatibilityClass(
  status: PluginInstallation["compatibility"]["status"],
): string {
  if (status === "compatible")
    return "bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300";
  if (status === "untested")
    return "bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300";
  return "bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-300";
REDACTED

onMounted(() => {
  window.addEventListener("message", handleBridgeMessage);
  void loadPlugins();
REDACTED);

onBeforeUnmount(() => {
  window.removeEventListener("message", handleBridgeMessage);
  clearPendingBridgeRequests();
REDACTED);
</script>
