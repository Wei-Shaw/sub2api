<template>
  <div class="grid gap-5 lg:grid-cols-[minmax(0,1.15fr)_minmax(320px,0.85fr)]">
    <section class="min-w-0">
      <div class="mb-3 flex items-center justify-between gap-3">
        <h3 class="text-sm font-semibold text-gray-900 dark:text-white">
          {{ t("admin.groups.compositeRoutes.routes") }}
        </h3>
        <button
          type="button"
          class="btn btn-secondary btn-sm"
          :disabled="loading || !schemeId"
          @click="loadRoutes"
        >
          <Icon
            name="refresh"
            size="sm"
            :class="loading ? 'animate-spin' : ''"
          />
        </button>
      </div>

      <div
        class="overflow-hidden rounded-lg border border-gray-200 dark:border-dark-600"
      >
        <div
          v-if="!schemeId"
          class="flex h-36 items-center justify-center text-sm text-gray-500 dark:text-gray-400"
        >
          {{ t("admin.routeManagement.selectSchemeHint") }}
        </div>
        <div
          v-else-if="loading"
          class="flex h-36 items-center justify-center text-sm text-gray-500 dark:text-gray-400"
        >
          {{ t("common.loading") }}
        </div>
        <div
          v-else-if="routes.length === 0"
          class="flex h-36 items-center justify-center text-sm text-gray-500 dark:text-gray-400"
        >
          {{ t("admin.groups.compositeRoutes.empty") }}
        </div>
        <div v-else class="overflow-x-auto">
          <table class="min-w-full divide-y divide-gray-200 text-sm dark:divide-dark-600">
            <thead class="bg-gray-50 text-left text-xs font-medium uppercase tracking-wide text-gray-500 dark:bg-dark-800 dark:text-gray-400">
              <tr>
                <th class="px-3 py-2">
                  {{ t("admin.groups.compositeRoutes.publicModel") }}
                </th>
                <th class="px-3 py-2">
                  {{ t("admin.groups.compositeRoutes.target") }}
                </th>
                <th class="px-3 py-2">
                  {{ t("admin.groups.compositeRoutes.scope") }}
                </th>
                <th class="px-3 py-2 text-right">
                  {{ t("admin.groups.columns.actions") }}
                </th>
              </tr>
            </thead>
            <tbody class="divide-y divide-gray-100 bg-white dark:divide-dark-700 dark:bg-dark-900">
              <tr
                v-for="route in routes"
                :key="route.id"
                :class="!route.enabled && 'opacity-60'"
              >
                <td class="max-w-[15rem] px-3 py-2">
                  <div class="break-all font-medium text-gray-900 dark:text-white">
                    {{ route.public_model }}
                  </div>
                  <div class="mt-1 flex flex-wrap items-center gap-1.5">
                    <span class="badge badge-gray">{{
                      matchLabel(route.match_type)
                    }}</span>
                    <span
                      v-if="!route.enabled"
                      class="badge badge-danger"
                    >
                      {{ t("admin.accounts.status.inactive") }}
                    </span>
                  </div>
                </td>
                <td class="px-3 py-2">
                  <div class="flex items-center gap-1.5 text-gray-900 dark:text-white">
                    <PlatformIcon :platform="route.target_platform" size="xs" />
                    <span>{{ formatPlatform(route.target_platform) }}</span>
                  </div>
                  <div class="mt-1 break-all text-xs text-gray-500 dark:text-gray-400">
                    {{ route.upstream_model || route.public_model }}
                  </div>
                </td>
                <td class="px-3 py-2">
                  <div class="text-gray-700 dark:text-gray-300">
                    {{ formatEndpoint(route.endpoint) }}
                  </div>
                  <div class="text-xs text-gray-500 dark:text-gray-400">
                    {{ t("admin.groups.compositeRoutes.priority") }}:
                    {{ route.priority }}
                  </div>
                </td>
                <td class="px-3 py-2">
                  <div class="flex justify-end gap-1">
                    <button
                      type="button"
                      class="rounded p-1.5 text-gray-500 hover:bg-gray-100 hover:text-primary-600 dark:hover:bg-dark-700 dark:hover:text-primary-400"
                      :title="t('common.edit')"
                      @click="editRoute(route)"
                    >
                      <Icon name="edit" size="sm" />
                    </button>
                    <button
                      type="button"
                      class="rounded p-1.5 text-gray-500 hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-900/20 dark:hover:text-red-400"
                      :title="t('common.delete')"
                      @click="removeRoute(route)"
                    >
                      <Icon name="trash" size="sm" />
                    </button>
                  </div>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
    </section>

    <section class="space-y-5">
      <form class="space-y-3" @submit.prevent="saveRoute">
        <div class="flex items-center justify-between gap-3">
          <h3 class="text-sm font-semibold text-gray-900 dark:text-white">
            {{
              editingId
                ? t("admin.groups.compositeRoutes.editRoute")
                : t("admin.groups.compositeRoutes.addRoute")
            }}
          </h3>
          <button
            v-if="editingId"
            type="button"
            class="text-xs font-medium text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200"
            @click="resetForm"
          >
            {{ t("common.cancel") }}
          </button>
        </div>

        <div>
          <label class="input-label">{{
            t("admin.groups.compositeRoutes.publicModel")
          }}</label>
          <input
            v-model.trim="form.public_model"
            type="text"
            class="input"
            required
            :disabled="!schemeId"
            placeholder="openrouter/gpt-5"
          />
        </div>

        <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
          <div>
            <label class="input-label">{{
              t("admin.groups.compositeRoutes.matchType")
            }}</label>
            <Select
              v-model="form.match_type"
              :options="matchOptions"
              :disabled="!schemeId"
            />
          </div>
          <div>
            <label class="input-label">{{
              t("admin.groups.compositeRoutes.endpoint")
            }}</label>
            <Select
              v-model="form.endpoint"
              :options="endpointOptions"
              :disabled="!schemeId"
            />
          </div>
        </div>

        <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
          <div>
            <label class="input-label">{{
              t("admin.groups.compositeRoutes.targetPlatform")
            }}</label>
            <Select
              v-model="form.target_platform"
              :options="platformOptions"
              :disabled="!schemeId"
            />
          </div>
          <div>
            <label class="input-label">{{
              t("admin.groups.compositeRoutes.priority")
            }}</label>
            <input
              v-model.number="form.priority"
              type="number"
              min="1"
              step="1"
              class="input"
              :disabled="!schemeId"
            />
          </div>
        </div>

        <div>
          <label class="input-label">{{
            t("admin.groups.compositeRoutes.upstreamModel")
          }}</label>
          <input
            v-model.trim="form.upstream_model"
            type="text"
            class="input"
            :disabled="!schemeId"
            placeholder="gpt-5"
          />
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
            {{ t("admin.groups.compositeRoutes.upstreamModelHint") }}
          </p>
        </div>

        <div>
          <label class="input-label">{{
            t("admin.groups.compositeRoutes.notes")
          }}</label>
          <textarea
            v-model.trim="form.notes"
            rows="2"
            class="input"
            :disabled="!schemeId"
          ></textarea>
        </div>

        <div class="flex items-center justify-between gap-3">
          <label class="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
            <input
              v-model="form.enabled"
              type="checkbox"
              class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500 dark:border-dark-600 dark:bg-dark-700"
              :disabled="!schemeId"
            />
            {{ t("admin.groups.compositeRoutes.enabled") }}
          </label>
          <button
            type="submit"
            class="btn btn-primary"
            :disabled="saving || !schemeId"
          >
            <Icon
              v-if="!saving"
              name="check"
              size="sm"
              class="mr-2"
            />
            {{ editingId ? t("common.update") : t("common.create") }}
          </button>
        </div>
      </form>

      <div class="border-t border-gray-200 pt-4 dark:border-dark-600">
        <h3 class="mb-3 text-sm font-semibold text-gray-900 dark:text-white">
          {{ t("admin.groups.compositeRoutes.preview") }}
        </h3>
        <div class="space-y-3">
          <input
            v-model.trim="previewModel"
            type="text"
            class="input"
            placeholder="openrouter/gpt-5"
            :disabled="!schemeId"
            @keyup.enter="runPreview"
          />
          <div class="flex gap-2">
            <Select
              v-model="previewEndpoint"
              :options="endpointOptions"
              class="min-w-0 flex-1"
              :disabled="!schemeId"
            />
            <button
              type="button"
              class="btn btn-secondary"
              :disabled="previewLoading || !previewModel || !schemeId"
              @click="runPreview"
            >
              <Icon name="play" size="sm" />
            </button>
          </div>

          <div
            v-if="previewDecision"
            class="rounded-lg border border-gray-200 bg-gray-50 p-3 text-sm dark:border-dark-600 dark:bg-dark-800"
          >
            <div class="mb-2 flex items-center gap-2">
              <span
                :class="[
                  'badge',
                  previewDecision.matched ? 'badge-success' : 'badge-danger',
                ]"
              >
                {{
                  previewDecision.matched
                    ? t("admin.groups.compositeRoutes.matched")
                    : t("admin.groups.compositeRoutes.notMatched")
                }}
              </span>
              <span class="badge badge-gray">
                {{ sourceLabel(previewDecision.source) }}
              </span>
            </div>
            <div
              v-if="previewDecision.matched"
              class="space-y-1 text-gray-700 dark:text-gray-300"
            >
              <div>
                {{ t("admin.groups.compositeRoutes.targetPlatform") }}:
                {{ formatPlatform(previewDecision.target_platform) }}
              </div>
              <div class="break-all">
                {{ t("admin.groups.compositeRoutes.upstreamModel") }}:
                {{ previewDecision.upstream_model }}
              </div>
            </div>
            <div v-else class="text-gray-500 dark:text-gray-400">
              {{ previewDecision.reason }}
            </div>
          </div>
        </div>
      </div>
    </section>
  </div>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import { useAppStore } from "@/stores/app";
import { adminAPI } from "@/api/admin";
import { CONCRETE_PLATFORM_OPTIONS } from "@/constants/platforms";
import type {
  CompositeModelRoute,
  CompositeModelRouteInput,
  CompositeRouteDecision,
  CompositeRouteEndpoint,
  CompositeRouteMatchType,
  GroupPlatform,
} from "@/types";
import Icon from "@/components/icons/Icon.vue";
import Select from "@/components/common/Select.vue";
import PlatformIcon from "@/components/common/PlatformIcon.vue";

const props = defineProps<{
  schemeId: number | null;
}>();

const emit = defineEmits<{
  changed: [];
}>();

type ConcreteGroupPlatform = Exclude<GroupPlatform, "composite">;
type FormState = {
  public_model: string;
  match_type: CompositeRouteMatchType;
  target_platform: ConcreteGroupPlatform;
  upstream_model: string;
  endpoint: CompositeRouteEndpoint;
  priority: number;
  enabled: boolean;
  notes: string;
};

const { t } = useI18n();
const appStore = useAppStore();

const routes = ref<CompositeModelRoute[]>([]);
const loading = ref(false);
const saving = ref(false);
const editingId = ref<number | null>(null);
const previewModel = ref("");
const previewEndpoint = ref<CompositeRouteEndpoint>("any");
const previewLoading = ref(false);
const previewDecision = ref<CompositeRouteDecision | null>(null);
const form = reactive<FormState>({
  public_model: "",
  match_type: "exact",
  target_platform: "openai",
  upstream_model: "",
  endpoint: "any",
  priority: 100,
  enabled: true,
  notes: "",
});

const platformOptions = computed(() => [...CONCRETE_PLATFORM_OPTIONS]);
const endpointOptions = computed(() => [
  { value: "any", label: t("admin.groups.compositeRoutes.endpoints.any") },
  { value: "messages", label: t("admin.groups.compositeRoutes.endpoints.messages") },
  { value: "count_tokens", label: t("admin.groups.compositeRoutes.endpoints.countTokens") },
  { value: "responses", label: t("admin.groups.compositeRoutes.endpoints.responses") },
  { value: "chat_completions", label: t("admin.groups.compositeRoutes.endpoints.chatCompletions") },
  { value: "embeddings", label: t("admin.groups.compositeRoutes.endpoints.embeddings") },
  { value: "images", label: t("admin.groups.compositeRoutes.endpoints.images") },
  { value: "gemini", label: t("admin.groups.compositeRoutes.endpoints.gemini") },
]);
const matchOptions = computed(() => [
  { value: "exact", label: t("admin.groups.compositeRoutes.match.exact") },
  { value: "prefix", label: t("admin.groups.compositeRoutes.match.prefix") },
]);

const matchLabel = (matchType: CompositeRouteMatchType) =>
  matchOptions.value.find((option) => option.value === matchType)?.label || matchType;

const formatEndpoint = (endpoint: CompositeRouteEndpoint) =>
  endpointOptions.value.find((option) => option.value === endpoint)?.label || endpoint;

const formatPlatform = (platform: string) => {
  if (!platform) return "—";
  return t(`admin.groups.platforms.${platform}`);
};

const sourceLabel = (source: string) => {
  if (source === "route") return t("admin.groups.compositeRoutes.sources.route");
  if (source === "detector") return t("admin.groups.compositeRoutes.sources.detector");
  return source || "—";
};

const resetForm = () => {
  editingId.value = null;
  form.public_model = "";
  form.match_type = "exact";
  form.target_platform = "openai";
  form.upstream_model = "";
  form.endpoint = "any";
  form.priority = 100;
  form.enabled = true;
  form.notes = "";
};

const toInput = (): CompositeModelRouteInput => ({
  public_model: form.public_model.trim(),
  match_type: form.match_type,
  target_platform: form.target_platform,
  upstream_model: form.upstream_model.trim(),
  endpoint: form.endpoint,
  priority: Number(form.priority) || 100,
  enabled: form.enabled,
  notes: form.notes.trim(),
});

const loadRoutes = async () => {
  if (!props.schemeId) {
    routes.value = [];
    return;
  }
  loading.value = true;
  try {
    const list = await adminAPI.routeSchemes.listRoutes(props.schemeId);
    routes.value = list.sort((a, b) => {
      if (a.priority !== b.priority) return a.priority - b.priority;
      return a.id - b.id;
    });
  } catch (error: any) {
    appStore.showError(
      error.response?.data?.detail ||
        error.response?.data?.message ||
        t("admin.groups.compositeRoutes.failedToLoad"),
    );
  } finally {
    loading.value = false;
  }
};

const editRoute = (route: CompositeModelRoute) => {
  editingId.value = route.id;
  form.public_model = route.public_model;
  form.match_type = route.match_type;
  form.target_platform = route.target_platform;
  form.upstream_model = route.upstream_model;
  form.endpoint = route.endpoint;
  form.priority = route.priority || 100;
  form.enabled = route.enabled;
  form.notes = route.notes || "";
};

const saveRoute = async () => {
  if (!props.schemeId) return;
  if (!form.public_model.trim()) {
    appStore.showError(t("admin.groups.compositeRoutes.publicModelRequired"));
    return;
  }
  saving.value = true;
  try {
    const payload = toInput();
    if (editingId.value) {
      await adminAPI.routeSchemes.updateRoute(props.schemeId, editingId.value, payload);
      appStore.showSuccess(t("admin.groups.compositeRoutes.routeUpdated"));
    } else {
      await adminAPI.routeSchemes.createRoute(props.schemeId, payload);
      appStore.showSuccess(t("admin.groups.compositeRoutes.routeCreated"));
    }
    resetForm();
    await loadRoutes();
    emit("changed");
  } catch (error: any) {
    appStore.showError(
      error.response?.data?.detail ||
        error.response?.data?.message ||
        t("admin.groups.compositeRoutes.failedToSave"),
    );
  } finally {
    saving.value = false;
  }
};

const removeRoute = async (route: CompositeModelRoute) => {
  if (!props.schemeId) return;
  if (!window.confirm(t("admin.groups.compositeRoutes.deleteConfirm"))) return;
  try {
    await adminAPI.routeSchemes.deleteRoute(props.schemeId, route.id);
    if (editingId.value === route.id) {
      resetForm();
    }
    appStore.showSuccess(t("admin.groups.compositeRoutes.routeDeleted"));
    await loadRoutes();
    emit("changed");
  } catch (error: any) {
    appStore.showError(
      error.response?.data?.detail ||
        error.response?.data?.message ||
        t("admin.groups.compositeRoutes.failedToDelete"),
    );
  }
};

const runPreview = async () => {
  if (!props.schemeId || !previewModel.value.trim()) return;
  previewLoading.value = true;
  try {
    previewDecision.value = await adminAPI.routeSchemes.preview(props.schemeId, {
      model: previewModel.value.trim(),
      endpoint: previewEndpoint.value,
    });
  } catch (error: any) {
    appStore.showError(
      error.response?.data?.detail ||
        error.response?.data?.message ||
        t("admin.groups.compositeRoutes.failedToPreview"),
    );
  } finally {
    previewLoading.value = false;
  }
};

watch(
  () => props.schemeId,
  async () => {
    resetForm();
    previewModel.value = "";
    previewEndpoint.value = "any";
    previewDecision.value = null;
    await loadRoutes();
  },
  { immediate: true },
);
</script>
