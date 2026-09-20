<template>
  <section :class="embedded ? '' : 'mt-4 rounded-lg border border-gray-200 bg-gray-50 p-4 dark:border-dark-600 dark:bg-dark-800'" :aria-labelledby="embedded ? undefined : 'ticket-proxy-pool-title'">
    <div v-if="!embedded" class="flex flex-wrap items-center justify-between gap-2">
      <h4 id="ticket-proxy-pool-title" class="font-medium text-gray-900 dark:text-white">{{ t(`${prefix}.poolTitle`) }}</h4>
      <button type="button" class="text-sm text-primary-600 hover:underline disabled:opacity-50" :disabled="loading || importing || providerBusy" @click="loadStatus">{{ t(`${prefix}.poolRefresh`) }}</button>
    </div>
    <p v-if="!embedded" class="mt-2 text-sm text-gray-600 dark:text-gray-300">{{ t(`${prefix}.poolRule`) }}</p>
    <p v-if="status && !embedded" class="mt-2 text-sm font-medium text-gray-700 dark:text-gray-200" data-testid="pool-status" aria-live="polite">
      {{ t(`${prefix}.poolStatus`, { count: status.count, removed: status.removed_count }) }}
    </p>
    <p v-if="!embedded && status?.managed && status.count === 0" class="mt-1 text-sm text-amber-600">{{ t(`${prefix}.poolEmpty`) }}</p>
    <OnesProxyImport v-if="!embedded" @imported="onProviderImported" @busy-change="providerBusy = $event" />
    <label for="ticket-proxy-import" class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300" :class="{ 'mt-4': !embedded }">{{ t(`${prefix}.poolInput`) }}</label>
    <textarea
      id="ticket-proxy-import"
      v-model="input"
      rows="7"
      class="input w-full resize-y rounded-xl font-mono text-sm leading-7"
      :placeholder="placeholder"
      :disabled="importing || providerBusy"
      autocomplete="off"
      spellcheck="false"
      aria-describedby="ticket-proxy-formats"
    />
    <p id="ticket-proxy-formats" class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t(`${prefix}.poolFormats`) }}</p>
    <div class="mt-3 flex flex-wrap items-center gap-3">
      <button type="button" class="btn btn-primary" :disabled="importing || providerBusy || readingFile || loading || !input.trim()" data-testid="pool-import" @click="importProxies">
        {{ t(`${prefix}.${importing ? 'poolImporting' : 'poolImport'}`) }}
      </button>
      <label class="btn btn-secondary cursor-pointer" :class="{ 'pointer-events-none opacity-50': importing || providerBusy || readingFile }">
        {{ t(`${prefix}.poolFile`) }}
        <input type="file" accept=".txt,text/plain" class="sr-only" :disabled="importing || providerBusy || readingFile" @change="readFile" />
      </label>
      <span class="text-xs text-gray-500">{{ t(`${prefix}.poolImmediate`) }}</span>
    </div>
    <p v-if="message" class="mt-3 text-sm text-emerald-600" role="status">{{ message }}</p>
    <p v-if="error || statusError" class="mt-3 text-sm text-red-600" role="alert">{{ error || statusError }}</p>
  </section>
</template>

<script setup lang="ts">
import { onMounted, onUnmounted, ref } from "vue";
import { useI18n } from "vue-i18n";
import { adminAPI } from "@/api";
import type { TicketProxyPoolStatus } from "@/api/admin/settings";
import OnesProxyImport from "./OnesProxyImport.vue";
import { extractApiErrorMessage } from "@/utils/apiError";

const { t } = useI18n();
const props = withDefaults(defineProps<{ embedded?: boolean }>(), { embedded: false });
const emit = defineEmits<{ imported: [result: TicketProxyPoolStatus]; busyChange: [busy: boolean] }>();
const prefix = "admin.settings.gatewayForwarding";
const placeholder = "192.0.2.1:8080\n192.0.2.2:8080:username:password\nsocks5h://username:password@192.0.2.3:1080";
const input = ref("");
const status = ref<TicketProxyPoolStatus | null>(null);
const importing = ref(false);
const providerBusy = ref(false);
const readingFile = ref(false);
const loading = ref(false);
const message = ref("");
const error = ref("");
const statusError = ref("");
let timer: ReturnType<typeof setInterval> | undefined;
let statusRequest = 0;

function onProviderImported(result: TicketProxyPoolStatus) {
  statusRequest++;
  status.value = result;
  statusError.value = "";
}

async function loadStatus() {
  if (loading.value || importing.value || providerBusy.value) return;
  loading.value = true;
  const request = ++statusRequest;
  try {
    const result = await adminAPI.settings.getTicketProxyPool();
    if (request === statusRequest) status.value = result;
    statusError.value = "";
  } catch {
    statusError.value = t(`${prefix}.poolLoadError`);
  } finally {
    loading.value = false;
  }
}

async function readFile(event: Event) {
  const element = event.target as HTMLInputElement;
  const file = element.files?.[0];
  if (!file) return;
  error.value = "";
  message.value = "";
  readingFile.value = true;
  try {
    if (file.size > 2 * 1024 * 1024) throw new Error(t(`${prefix}.poolFileTooLarge`));
    input.value = await file.text();
  } catch (err) {
    error.value = err instanceof Error ? err.message : t(`${prefix}.poolLoadError`);
  } finally {
    readingFile.value = false;
    element.value = "";
  }
}

async function importProxies() {
  if (importing.value || loading.value || providerBusy.value) return;
  error.value = "";
  message.value = "";
  const proxies = input.value.split(/\r?\n/).map((line) => line.trim()).filter(Boolean);
  if (!proxies.length || proxies.length > 2000) {
    error.value = t(`${prefix}.poolLimit`);
    return;
  }
  importing.value = true;
  emit("busyChange", true);
  try {
    const result = await adminAPI.settings.importTicketProxyPool(proxies);
    status.value = result;
    emit("imported", result);
    input.value = "";
    message.value = t(`${prefix}.poolSuccess`, { added: result.added, duplicates: result.duplicates, count: result.count });
  } catch (err) {
    error.value = extractApiErrorMessage(err, t(`${prefix}.poolImportError`));
  } finally {
    importing.value = false;
    emit("busyChange", false);
  }
}

onMounted(() => {
  if (props.embedded) return;
  void loadStatus();
  timer = setInterval(() => { void loadStatus(); }, 15000);
});
onUnmounted(() => { if (timer) clearInterval(timer); });
</script>
