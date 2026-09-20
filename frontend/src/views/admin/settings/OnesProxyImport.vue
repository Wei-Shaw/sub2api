<template>
  <section :class="embedded ? '' : 'mt-4 rounded-xl border border-gray-200 bg-white p-4 dark:border-dark-600 dark:bg-dark-900'" data-testid="onesproxy-import">
    <h3 v-if="!embedded" class="font-medium text-gray-900 dark:text-white">{{ t(`${prefix}.apiTitle`) }}</h3>
    <div class="flex flex-wrap items-center justify-between gap-3 rounded-xl bg-gray-50 px-4 py-3 dark:bg-dark-800">
      <div class="flex flex-wrap items-center gap-2.5 text-sm">
        <span class="font-semibold text-gray-900 dark:text-white">OnesProxy</span>
        <span class="rounded-full px-2 py-0.5 text-xs font-medium" :class="saved?.configured ? 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300' : 'bg-gray-200 text-gray-600 dark:bg-dark-700 dark:text-gray-300'">{{ t(`${prefix}.${saved?.configured ? 'apiConfigured' : 'ticketNotConfigured'}`) }}</span>
      </div>
      <button type="button" data-testid="provider-config-toggle" class="text-sm font-medium text-primary-600 hover:text-primary-700 disabled:opacity-50 dark:text-primary-400" :disabled="loading || busy" :aria-expanded="settingsExpanded" aria-controls="onesproxy-connection-fields" @click="settingsExpanded = !settingsExpanded">{{ t(`${prefix}.${settingsExpanded ? 'ticketCollapse' : 'ticketConfigure'}`) }}</button>
    </div>
    <fieldset class="mt-4 space-y-3" :disabled="loading || busy">
      <div id="onesproxy-connection-fields" v-show="settingsExpanded" class="space-y-4 rounded-xl border border-gray-200 p-4 dark:border-dark-600">
      <p class="text-xs leading-5 text-gray-500">{{ t(`${prefix}.apiDescription`) }}</p>
      <div>
        <label for="onesproxy-type" class="mb-1 block text-sm">{{ t(`${prefix}.apiType`) }}</label>
        <select id="onesproxy-type" v-model="form.type" class="input w-full">
          <option value="generate">{{ t(`${prefix}.apiGenerate`) }}</option>
          <option value="extract">{{ t(`${prefix}.apiExtract`) }}</option>
        </select>
      </div>
      <template v-if="form.type === 'generate'">
        <div class="grid gap-3 sm:grid-cols-2">
          <div>
            <label for="onesproxy-userid" class="mb-1 block text-sm">Userid</label>
            <input id="onesproxy-userid" v-model="form.user_id" class="input w-full" autocomplete="off" maxlength="256" />
          </div>
          <div>
            <label for="onesproxy-proxy-id" class="mb-1 block text-sm">{{ t(`${prefix}.apiProxyID`) }}</label>
            <input id="onesproxy-proxy-id" v-model="form.proxy_id" class="input w-full" placeholder="SC…" autocomplete="off" maxlength="256" />
          </div>
        </div>
        <div>
          <label for="onesproxy-token" class="mb-1 block text-sm">Token</label>
          <input id="onesproxy-token" v-model="form.token" type="password" class="input w-full" :placeholder="saved?.token_configured ? t(`${prefix}.apiSecretSaved`) : t(`${prefix}.apiTokenPlaceholder`)" autocomplete="new-password" maxlength="4096" />
        </div>
        <div class="grid gap-3 sm:grid-cols-2">
          <div>
            <label for="onesproxy-country" class="mb-1 block text-sm">{{ t(`${prefix}.apiCountry`) }}</label>
            <input id="onesproxy-country" v-model="form.country" class="input w-full uppercase" placeholder="US" maxlength="2" />
          </div>
          <div>
            <label for="onesproxy-route" class="mb-1 block text-sm">{{ t(`${prefix}.apiRoute`) }}</label>
            <input id="onesproxy-route" v-model="form.route" class="input w-full uppercase" placeholder="NA" maxlength="24" />
          </div>
          <div>
            <label for="onesproxy-mode" class="mb-1 block text-sm">{{ t(`${prefix}.apiMode`) }}</label>
            <select id="onesproxy-mode" v-model.number="form.mode" class="input w-full">
              <option :value="1">{{ t(`${prefix}.apiSticky`) }}</option>
              <option :value="2">{{ t(`${prefix}.apiRotate`) }}</option>
            </select>
          </div>
          <div v-if="form.mode === 1">
            <label for="onesproxy-session" class="mb-1 block text-sm">{{ t(`${prefix}.apiSession`) }}</label>
            <input id="onesproxy-session" v-model.number="form.session_time" type="number" min="1" max="120" class="input w-full" />
          </div>
        </div>
      </template>
      <div v-else>
        <label for="onesproxy-url" class="mb-1 block text-sm">{{ t(`${prefix}.apiURL`) }}</label>
        <input id="onesproxy-url" v-model="form.extract_url" type="password" class="input w-full" :placeholder="saved?.extract_configured ? t(`${prefix}.apiSecretSaved`) : 'https://thirdapi.onesproxy.com/api/proxy/getProxyList?key=…'" autocomplete="new-password" maxlength="8192" />
      </div>
      </div>
      <div class="max-w-xs">
        <label for="onesproxy-count" class="mb-1 block text-sm">{{ t(`${prefix}.apiCount`) }}</label>
        <input id="onesproxy-count" v-model.number="count" type="number" min="1" max="2000" step="1" class="input w-full" />
      </div>
      <p class="text-xs text-gray-500">{{ t(`${prefix}.apiLimitNote`) }}</p>
      <div class="rounded-xl border border-primary-100 bg-primary-50/50 p-4 dark:border-primary-900/50 dark:bg-primary-900/10">
        <label for="onesproxy-auto-refill" class="flex cursor-pointer items-center gap-2 text-sm font-medium">
          <input id="onesproxy-auto-refill" v-model="form.auto_refill" type="checkbox" class="h-4 w-4" aria-describedby="onesproxy-auto-refill-note" />
          {{ t(`${prefix}.apiAutoRefill`) }}
        </label>
        <p id="onesproxy-auto-refill-note" class="mt-1 text-xs text-gray-500">{{ t(`${prefix}.apiAutoRefillNote`) }}</p>
      </div>
      <div class="flex flex-wrap items-center gap-3">
        <button type="button" class="btn btn-primary" data-testid="onesproxy-fetch" @click="submit(true)">{{ t(`${prefix}.${fetching ? 'apiFetching' : 'apiFetch'}`) }}</button>
        <button type="button" class="btn btn-secondary" data-testid="onesproxy-save" @click="submit(false)">{{ t(`${prefix}.apiSave`) }}</button>
        <span v-if="saved?.configured" class="text-xs text-emerald-600">{{ t(`${prefix}.apiConfigured`) }}</span>
      </div>
    </fieldset>
    <p v-if="message" class="mt-3 text-sm text-emerald-600" role="status">{{ message }}</p>
    <p v-if="error" class="mt-3 text-sm text-red-600" role="alert">{{ error }}</p>
    <button v-if="loadFailed" type="button" class="mt-2 text-sm text-primary-600" :disabled="loading" @click="loadConfig">{{ t(`${prefix}.poolRefresh`) }}</button>
  </section>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from "vue";
import { useI18n } from "vue-i18n";
import { adminAPI } from "@/api";
import type { TicketProxyProviderConfig, TicketProxyProviderStatus, TicketProxyFetchResult } from "@/api/admin/settings";
import { extractApiErrorMessage } from "@/utils/apiError";

withDefaults(defineProps<{ embedded?: boolean }>(), { embedded: false });
const emit = defineEmits<{ imported: [result: TicketProxyFetchResult]; busyChange: [busy: boolean]; saved: [status: TicketProxyProviderStatus] }>();
const { t } = useI18n();
const prefix = "admin.settings.gatewayForwarding";
const form = reactive<TicketProxyProviderConfig>({ type: "generate", user_id: "", token: "", proxy_id: "", extract_url: "", country: "US", mode: 1, session_time: 30, route: "NA", auto_refill: false });
const count = ref(2000);
const saved = ref<TicketProxyProviderStatus | null>(null);
const loading = ref(true);
const saving = ref(false);
const fetching = ref(false);
const loadFailed = ref(false);
const settingsExpanded = ref(false);
const busy = computed(() => saving.value || fetching.value);
const message = ref("");
const error = ref("");

async function loadConfig() {
  loading.value = true;
  error.value = "";
  try {
    const status = await adminAPI.settings.getTicketProxyProvider();
    saved.value = status;
    settingsExpanded.value = !status.configured;
    Object.assign(form, { type: status.type, user_id: status.user_id, proxy_id: status.proxy_id, country: status.country, mode: status.mode, session_time: status.session_time, route: status.route, auto_refill: status.auto_refill ?? false, token: "", extract_url: "" });
    loadFailed.value = false;
  } catch (err) {
    loadFailed.value = true;
    error.value = extractApiErrorMessage(err, t(`${prefix}.apiLoadError`));
  } finally { loading.value = false; }
}

async function submit(fetch: boolean) {
  if (busy.value || loading.value) return;
  message.value = "";
  error.value = "";
  if (form.type === "generate" && (!form.user_id.trim() || !form.proxy_id.trim() || (!form.token?.trim() && !saved.value?.token_configured))) {
    error.value = t(`${prefix}.apiMissingCredentials`);
    return;
  }
  if (form.type === "extract" && !form.extract_url?.trim() && !saved.value?.extract_configured) {
    error.value = t(`${prefix}.apiMissingURL`);
    return;
  }
  if (fetch && (!Number.isInteger(count.value) || count.value < 1 || count.value > 2000)) {
    error.value = t(`${prefix}.poolLimit`);
    return;
  }
  saving.value = true;
  emit("busyChange", true);
  try {
    saved.value = await adminAPI.settings.saveTicketProxyProvider({ ...form });
    emit("saved", saved.value);
    form.token = "";
    form.extract_url = "";
    if (!fetch) {
      message.value = t(`${prefix}.apiSaved`);
      return;
    }
    fetching.value = true;
    const result = await adminAPI.settings.fetchTicketProxyProvider(count.value);
    emit("imported", result);
    message.value = t(`${prefix}.apiSuccess`, { requested: result.requested, received: result.received, added: result.added, duplicates: result.duplicates, count: result.count });
  } catch (err) {
    error.value = extractApiErrorMessage(err, t(`${prefix}.apiError`));
  } finally {
    saving.value = false;
    fetching.value = false;
    emit("busyChange", false);
  }
}

onMounted(() => { void loadConfig(); });
</script>
