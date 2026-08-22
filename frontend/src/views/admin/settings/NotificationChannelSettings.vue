<template>
  <div class="card">
    <div
      class="flex flex-col gap-3 border-b border-gray-100 px-6 py-4 dark:border-dark-700 lg:flex-row lg:items-start lg:justify-between"
    >
      <div>
        <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
          {{ localText("通知渠道 / Webhook", "Notification channels / Webhook") }}
        </h2>
        <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
          {{
            localText(
              "把通知投递到自建 Webhook。邮件可以按事件单独关闭。",
              "Deliver notifications to your own webhook. Email can be switched off per event.",
            )
          }}
        </p>
      </div>
      <div class="flex flex-wrap gap-2">
        <button
          type="button"
          class="btn btn-primary btn-sm"
          :disabled="loading || saving"
          @click="save"
        >
          {{ saving ? localText("保存中…", "Saving…") : localText("保存", "Save") }}
        </button>
      </div>
    </div>

    <div class="space-y-6 p-6">
      <div
        v-if="loading"
        class="flex items-center gap-2 text-sm text-gray-500 dark:text-gray-400"
      >
        <span
          class="h-4 w-4 animate-spin rounded-full border-b-2 border-primary-600"
        ></span>
        {{ t("common.loading") }}
      </div>

      <template v-else>
        <!-- Global webhook transport -->
        <div class="space-y-4">
          <div class="flex items-start justify-between gap-4">
            <div>
              <label
                class="mb-0 block text-sm font-medium text-gray-700 dark:text-gray-300"
              >
                {{ localText("启用 Webhook 投递", "Enable webhook delivery") }}
              </label>
              <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                {{
                  localText(
                    "总开关。关闭后所有事件都不会推送 Webhook，无需逐个取消。",
                    "Master switch. When off, no event pushes a webhook.",
                  )
                }}
              </p>
            </div>
            <Toggle v-model="form.webhook.enabled" />
          </div>

          <div class="grid grid-cols-1 gap-4 md:grid-cols-2">
            <div class="md:col-span-2">
              <label class="input-label" for="notify-webhook-url">
                {{ localText("默认 Webhook 地址", "Default webhook URL") }}
              </label>
              <input
                id="notify-webhook-url"
                v-model.trim="form.webhook.endpoint.url"
                class="input"
                placeholder="https://webhook.internal/notify"
              />
            </div>

            <div class="md:col-span-2">
              <label class="input-label" for="notify-webhook-secret">
                {{ localText("签名密钥", "Signing secret") }}
              </label>
              <div class="flex gap-2">
                <input
                  id="notify-webhook-secret"
                  v-model.trim="form.webhook.secret"
                  class="input font-mono text-xs"
                  spellcheck="false"
                  :placeholder="
                    localText('保存后自动生成', 'Generated automatically on save')
                  "
                />
                <button
                  type="button"
                  class="btn btn-secondary btn-sm shrink-0"
                  :disabled="!form.webhook.secret"
                  @click="copySecret"
                >
                  {{ localText("复制", "Copy") }}
                </button>
                <button
                  type="button"
                  class="btn btn-secondary btn-sm shrink-0"
                  @click="regenerateSecret"
                >
                  {{ localText("重新生成", "Regenerate") }}
                </button>
              </div>
              <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                {{
                  localText(
                    "把它填到你的接收端用于验签。每次投递都会带 X-Sub2Api-Signature = hex(HMAC-SHA256(密钥, 时间戳 + \".\" + 报文))。",
                    'Copy it into your receiver to verify signatures. Every delivery carries X-Sub2Api-Signature = hex(HMAC-SHA256(secret, timestamp + "." + body)).',
                  )
                }}
              </p>
            </div>
          </div>

          <details class="rounded border border-gray-100 px-4 py-3 dark:border-dark-700">
            <summary
              class="cursor-pointer text-sm text-gray-600 dark:text-gray-300"
            >
              {{ localText("高级设置", "Advanced") }}
            </summary>
            <div class="mt-4 grid grid-cols-1 gap-4 md:grid-cols-2">
              <div>
                <label class="input-label" for="notify-webhook-timeout">
                  {{ localText("单次超时（秒）", "Timeout (seconds)") }}
                </label>
                <input
                  id="notify-webhook-timeout"
                  v-model.number="form.webhook.timeout_seconds"
                  type="number"
                  min="1"
                  max="30"
                  class="input"
                />
              </div>
              <div>
                <label class="input-label" for="notify-webhook-retries">
                  {{ localText("失败重试次数", "Max retries") }}
                </label>
                <input
                  id="notify-webhook-retries"
                  v-model.number="form.webhook.max_retries"
                  type="number"
                  min="0"
                  max="5"
                  class="input"
                />
                <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                  {{ localText("填 0 表示失败后不重试", "0 means never retry after a failure") }}
                </p>
              </div>
            </div>
          </details>
        </div>

        <div class="border-t border-gray-100 pt-6 dark:border-dark-700">
          <h3 class="text-sm font-semibold text-gray-900 dark:text-white">
            {{ localText("按事件配置通道", "Per-event channels") }}
          </h3>
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
            {{
              localText(
                "面向用户的事件推到 Webhook 后，载荷里带 user_id / username / email，供消费方定位到具体用户。",
                "For user-facing events the payload carries user_id / username / email so a consumer can address the right user.",
              )
            }}
          </p>

          <div class="mt-4 overflow-x-auto">
            <table class="min-w-full text-sm">
              <thead>
                <tr class="text-left text-xs uppercase text-gray-500 dark:text-gray-400">
                  <th class="px-3 py-2">{{ localText("事件", "Event") }}</th>
                  <th class="px-3 py-2">{{ localText("受众", "Audience") }}</th>
                  <th class="px-3 py-2 text-center">{{ localText("邮件", "Email") }}</th>
                  <th class="px-3 py-2 text-center">Webhook</th>
                  <th class="px-3 py-2"></th>
                </tr>
              </thead>
              <tbody>
                <template v-for="event in events" :key="event.event">
                  <tr class="border-t border-gray-100 dark:border-dark-700">
                    <td class="px-3 py-3">
                      <div class="font-medium text-gray-900 dark:text-white">
                        {{ event.label }}
                      </div>
                      <div class="mt-0.5 font-mono text-xs text-gray-500 dark:text-gray-400">
                        {{ event.event }}
                      </div>
                      <div
                        v-if="event.event === 'ops.alert'"
                        class="mt-1 text-xs text-gray-500 dark:text-gray-400"
                      >
                        {{
                          localText(
                            '对符合通知最低级别且未静默的启用规则生效，不受规则邮件开关影响。',
                            'Applies to enabled rules that pass the notification floor and are not silenced, independent of the rule email switch.',
                          )
                        }}
                      </div>
                    </td>
                    <td class="px-3 py-3">
                      <span
                        class="rounded px-2 py-0.5 text-xs"
                        :class="
                          event.audience === 'admin'
                            ? 'bg-blue-50 text-blue-700 dark:bg-blue-900/30 dark:text-blue-300'
                            : 'bg-gray-100 text-gray-700 dark:bg-dark-700 dark:text-gray-300'
                        "
                      >
                        {{
                          event.audience === "admin"
                            ? localText("管理员", "Admin")
                            : localText("用户", "User")
                        }}
                      </span>
                    </td>
                    <td class="px-3 py-3 text-center">
                      <Toggle v-model="channelState[event.event].email" />
                    </td>
                    <td class="px-3 py-3 text-center">
                      <div :class="form.webhook.enabled ? '' : 'opacity-50'">
                        <Toggle v-model="channelState[event.event].webhook" />
                      </div>
                    </td>
                    <td class="px-3 py-3 text-right">
                      <button
                        type="button"
                        class="btn btn-secondary btn-sm"
                        @click="toggleExpanded(event.event)"
                      >
                        {{
                          expanded === event.event
                            ? localText("收起", "Close")
                            : localText("高级", "Advanced")
                        }}
                      </button>
                      <button
                        type="button"
                        class="btn btn-secondary btn-sm ml-2"
                        :disabled="testing === event.event || !form.webhook.enabled"
                        :title="
                          localText(
                            '会先保存当前配置，再推送一条示例报文',
                            'Saves the current configuration first, then pushes a sample payload',
                          )
                        "
                        @click="testEvent(event.event)"
                      >
                        {{
                          testing === event.event
                            ? localText("推送中…", "Sending…")
                            : localText("测试", "Test")
                        }}
                      </button>
                    </td>
                  </tr>
                  <tr
                    v-if="expanded === event.event"
                    class="bg-gray-50 dark:bg-dark-800"
                  >
                    <td colspan="5" class="px-3 py-4">
                      <div class="space-y-3">
                        <div>
                          <label class="input-label">
                            {{ localText("独立 Webhook 地址（可选）", "Dedicated webhook URL (optional)") }}
                          </label>
                          <input
                            v-model.trim="channelState[event.event].url"
                            class="input"
                            :placeholder="
                              form.webhook.endpoint.url ||
                              localText('留空则用默认地址', 'Blank uses the default URL')
                            "
                          />
                        </div>
                        <div>
                          <label class="input-label">
                            {{ localText("自定义报文模板（可选）", "Custom body template (optional)") }}
                          </label>
                          <textarea
                            v-model="channelState[event.event].bodyTemplate"
                            class="input font-mono text-xs"
                            rows="4"
                            :placeholder="bodyTemplatePlaceholder"
                          ></textarea>
                          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                            {{
                              localText(
                                "留空发送默认结构化 JSON。填写后必须渲染成合法 JSON，可直接产出接收端需要的格式。",
                                "Blank sends the default structured JSON. A custom template must render to valid JSON and can emit the format your receiver expects.",
                              )
                            }}
                          </p>
                          <p class="mt-2 text-xs text-gray-500 dark:text-gray-400">
                            {{ localText("可用占位符：", "Available placeholders: ") }}
                            <span class="font-mono">{{
                              availablePlaceholders(event).join(" ")
                            }}</span>
                          </p>
                        </div>
                      </div>
                    </td>
                  </tr>
                </template>
              </tbody>
            </table>
          </div>
        </div>
      </template>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from "vue";
import { useI18n } from "vue-i18n";
import { adminAPI } from "@/api";
import type {
  NotificationChannelConfigResponse,
  NotificationChannelEvent,
  NotificationChannelUpdateRequest,
} from "@/api/admin/settings";
import Toggle from "@/components/common/Toggle.vue";
import { useAppStore } from "@/stores";
import { extractApiErrorMessage } from "@/utils/apiError";

const { t, locale } = useI18n();
const appStore = useAppStore();

function localText(zh: string, en: string): string {
  return locale.value.toLowerCase().startsWith("zh") ? zh : en;
}

interface EventChannelState {
  email: boolean;
  webhook: boolean;
  url: string;
  bodyTemplate: string;
}

const loading = ref(true);
const saving = ref(false);
const testing = ref("");
const expanded = ref("");
const events = ref<NotificationChannelEvent[]>([]);
const webhookPlaceholders = ref<string[]>([]);
const channelState = reactive<Record<string, EventChannelState>>({});

const form = reactive({
  webhook: {
    enabled: false,
    endpoint: { url: "" },
    secret: "",
    timeout_seconds: 5,
    max_retries: 2,
  },
});

const bodyTemplatePlaceholder = `{"event":"{{event}}","source_id":"{{source_id}}","occurred_at":"{{occurred_at}}"}`;

function availablePlaceholders(event: NotificationChannelEvent): string[] {
  const all = [...event.placeholders, ...webhookPlaceholders.value];
  return Array.from(new Set(all)).map((name) => `{{${name}}}`);
}

function toggleExpanded(event: string): void {
  expanded.value = expanded.value === event ? "" : event;
}

function regenerateSecret(): void {
  const bytes = new Uint8Array(32);
  crypto.getRandomValues(bytes);
  form.webhook.secret = Array.from(bytes)
    .map((b) => b.toString(16).padStart(2, "0"))
    .join("");
}

async function copySecret(): Promise<void> {
  try {
    await navigator.clipboard.writeText(form.webhook.secret);
    appStore.showSuccess(localText("已复制", "Copied"));
  } catch {
    appStore.showError(localText("复制失败，请手动选择", "Copy failed, select it manually"));
  }
}

function applyResponse(data: NotificationChannelConfigResponse): void {
  form.webhook.enabled = data.webhook.enabled;
  form.webhook.endpoint.url = data.webhook.endpoint.url ?? "";
  form.webhook.secret = data.webhook.secret ?? "";
  form.webhook.timeout_seconds = data.webhook.timeout_seconds || 5;
  // `??` not `||`: 0 is a meaningful value here ("never retry") and must
  // survive a round trip through the form.
  form.webhook.max_retries = data.webhook.max_retries ?? 2;

  events.value = data.events ?? [];
  webhookPlaceholders.value = data.webhook_placeholders ?? [];
  for (const event of events.value) {
    channelState[event.event] = {
      email: event.email,
      webhook: event.webhook,
      url: event.endpoint?.url ?? "",
      bodyTemplate: event.endpoint?.body_template ?? "",
    };
  }
}

async function load(): Promise<void> {
  loading.value = true;
  try {
    applyResponse(await adminAPI.settings.getNotificationChannels());
  } catch (err) {
    appStore.showError(extractApiErrorMessage(err, t("common.error")));
  } finally {
    loading.value = false;
  }
}

function buildRequest(): NotificationChannelUpdateRequest {
  const request: NotificationChannelUpdateRequest = {
    webhook: {
      enabled: form.webhook.enabled,
      endpoint: { url: form.webhook.endpoint.url },
      secret: form.webhook.secret || undefined,
      timeout_seconds: form.webhook.timeout_seconds,
      max_retries: form.webhook.max_retries,
    },
    events: {},
  };

  for (const event of events.value) {
    const state = channelState[event.event];
    if (!state) continue;
    const entry: NonNullable<NotificationChannelUpdateRequest["events"]>[string] =
      {
        email: state.email,
        webhook: state.webhook,
      };
    if (state.url || state.bodyTemplate) {
      entry.endpoint = {
        url: state.url || undefined,
        body_template: state.bodyTemplate || undefined,
      };
    }
    request.events![event.event] = entry;
  }
  return request;
}

async function save(): Promise<void> {
  saving.value = true;
  try {
    applyResponse(
      await adminAPI.settings.updateNotificationChannels(buildRequest()),
    );
    appStore.showSuccess(localText("已保存", "Saved"));
  } catch (err) {
    appStore.showError(extractApiErrorMessage(err, t("common.error")));
  } finally {
    saving.value = false;
  }
}

// Testing hits the stored configuration, so unsaved edits must be persisted
// first or the operator would be testing the previous endpoint.
async function testEvent(event: string): Promise<void> {
  testing.value = event;
  try {
    await adminAPI.settings.updateNotificationChannels(buildRequest());
    await adminAPI.settings.testNotificationWebhook(event);
    appStore.showSuccess(localText("测试推送已送达", "Test webhook delivered"));
  } catch (err) {
    appStore.showError(extractApiErrorMessage(err, t("common.error")));
  } finally {
    testing.value = "";
  }
}

onMounted(load);
</script>
