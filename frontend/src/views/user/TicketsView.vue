<template>
  <AppLayout>
    <TablePageLayout :pagination-inside-table="true">
      <template #actions>
        <div class="flex items-center justify-between gap-4">
          <div>
            <p class="text-sm font-medium text-primary-600">
              {{ t("nav.tickets") }}
            </p>
            <h1 class="mt-1 text-2xl font-bold text-gray-900 dark:text-white">
              帮助与支持
            </h1>
            <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
              提交问题、查看回复，并随时跟进工单进度。
            </p>
          </div>
          <button class="btn btn-primary shrink-0" @click="showCreate = true">
            <Icon name="plus" size="sm" class="mr-1.5" />新建工单
          </button>
        </div>
      </template>
      <template #table>
        <div
          class="flex items-center justify-between border-b border-gray-200/80 px-5 py-4 dark:border-dark-700"
        >
          <div>
            <h2 class="font-semibold text-gray-900 dark:text-white">
              我的工单
            </h2>
            <p class="mt-1 text-xs text-gray-500">
              共 {{ pagination.total }} 条
            </p>
          </div>
          <button
            class="btn btn-ghost btn-sm"
            :disabled="busy"
            title="刷新"
            @click="load()"
          >
            <Icon
              name="refresh"
              size="sm"
              :class="busy ? 'animate-spin' : ''"
            />
          </button>
        </div>
        <div
          v-if="error"
          class="m-4 rounded-xl border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700"
        >
          {{ error }}
        </div>
        <DataTable
          :columns="columns"
          :data="tickets"
          :loading="busy"
          :clickable-rows="true"
          row-key="id"
          @row-click="openTicket"
        >
          <template #cell-title="{ row }"
            ><div class="min-w-0">
              <div class="truncate font-medium text-gray-900 dark:text-white">
                {{ row.title }}
              </div>
              <div class="mt-1 text-xs text-gray-500">#{{ row.id }}</div>
            </div></template
          >
          <template #cell-status="{ value }"
            ><span :class="['badge', statusClass(value)]">{{
              statusLabel(value)
            }}</span></template
          >
          <template #cell-updated_at="{ value }"
            ><span class="text-sm text-gray-500">{{
              formatDate(value)
            }}</span></template
          >
          <template #cell-actions="{ row }"
            ><div
              class="inline-flex items-center gap-1 rounded-xl border border-gray-200 bg-gray-50/80 p-1 shadow-sm dark:border-dark-600 dark:bg-dark-800/80"
            >
              <button
                type="button"
                class="inline-flex h-8 w-8 items-center justify-center rounded-lg text-gray-500 transition-all hover:bg-blue-100 hover:text-blue-700 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500 focus-visible:ring-offset-1 dark:text-gray-400 dark:hover:bg-blue-950/60 dark:hover:text-blue-300"
                title="查看工单"
                aria-label="查看工单"
                @click.stop="openTicket(row)"
              >
                <Icon name="eye" size="sm" :stroke-width="2" /></button
              ><button
                type="button"
                class="inline-flex h-8 w-8 items-center justify-center rounded-lg text-amber-600 transition-all hover:bg-amber-100 hover:text-amber-700 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-amber-500 focus-visible:ring-offset-1 disabled:cursor-not-allowed disabled:text-gray-300 disabled:hover:bg-transparent dark:text-amber-400 dark:hover:bg-amber-950/60 dark:hover:text-amber-300 dark:disabled:text-dark-500"
                title="关闭工单"
                aria-label="关闭工单"
                :disabled="row.status === 'closed' || busy"
                @click.stop="closeTicket(row)"
              >
                <Icon name="archiveBox" size="sm" :stroke-width="2" />
              </button></div
          ></template>
          <template #empty
            ><div class="py-12 text-center text-sm text-gray-500">
              还没有工单
            </div></template
          >
        </DataTable>
      </template>
      <template #pagination
        ><Pagination
          v-if="pagination.total > 0"
          :page="pagination.page"
          :total="pagination.total"
          :page-size="pagination.page_size"
          @update:page="changePage"
          @update:pageSize="changePageSize"
      /></template>
    </TablePageLayout>
    <BaseDialog
      :show="!!selected"
      :title="selected?.title || ''"
      width="wide"
      @close="selected = null"
    >
      <template v-if="selected"
        ><div
          class="mb-4 flex items-center justify-between text-xs text-gray-500"
        >
          工单 #{{ selected.id }} · {{ formatDate(selected.created_at)
          }}<span :class="['badge', statusClass(selected.status)]">{{
            statusLabel(selected.status)
          }}</span>
        </div>
        <div
          ref="messageListRef"
          class="flex max-h-[min(55vh,32rem)] flex-col space-y-4 overflow-y-auto rounded-xl bg-gray-50/70 p-4 dark:bg-dark-800/50"
          @scroll="handleMessageScroll"
        >
          <div
            v-for="message in selected.messages"
            :key="message.id"
            class="flex w-fit max-w-[85%] flex-col"
            :class="
              message.sender_type === 'user'
                ? 'ml-8 self-end items-end'
                : 'mr-8 self-start items-start'
            "
          >
            <div
              class="mb-1 px-1 text-sm font-medium text-gray-700 dark:text-gray-300"
            >
              {{ message.sender_type === "user" ? "我" : "管理员" }}
            </div>
            <div
              class="ticket-message rounded-2xl p-4"
              :class="
                message.sender_type === 'user'
                  ? 'bg-gray-100 dark:bg-dark-700'
                  : 'bg-white dark:bg-dark-800'
              "
            >
              <div class="whitespace-pre-wrap text-sm">
                {{ message.content }}
              </div>
              <TicketImageAttachments
                class="mt-3"
                :images="message.images || []"
                @preview="previewImage"
              />
            </div>
            <div class="ticket-message-time mt-1 px-1 text-xs text-gray-500">
              {{ formatDate(message.created_at) }}
            </div>
          </div>
        </div>
        <button
          v-if="newMessageCount"
          class="mt-3 w-full rounded-full bg-primary-600 px-4 py-2 text-sm font-medium text-white"
          @click="scrollMessagesToBottom"
        >
          有 {{ newMessageCount }} 条新消息，点击查看
        </button>
        <form
          v-if="selected.status !== 'closed'"
          class="mt-4 border-t border-gray-100 pt-4 dark:border-dark-700"
          @submit.prevent="reply"
        >
          <textarea
            v-model="replyText"
            class="input min-h-28 w-full resize-y"
            placeholder="继续描述你的问题或回复管理员…"
            @paste="handlePaste($event, replyImages)"
          />
          <div class="mt-3 flex items-center justify-between gap-3">
            <div>
              <label class="btn btn-secondary btn-sm cursor-pointer"
                ><input
                  type="file"
                  accept="image/*"
                  multiple
                  class="hidden"
                  @change="pickFiles($event, replyImages)"
                /><Icon name="upload" size="sm" class="mr-1.5" />添加图片</label
              ><span class="ml-2 text-xs text-gray-500"
                >{{ replyImages.length }}/9</span
              ><TicketImageAttachments
                class="mt-2"
                :images="replyImages"
                removable
                @preview="previewImage"
                @remove="replyImages.splice($event, 1)"
              />
            </div>
            <button
              class="btn btn-primary"
              :disabled="busy || (!replyText.trim() && !replyImages.length)"
            >
              发送回复
            </button>
          </div>
        </form>
        <div
          v-else
          class="mt-4 border-t border-gray-100 pt-4 text-sm text-gray-500 dark:border-dark-700"
        >
          此工单已关闭，如需继续协助，请新建工单。
        </div></template
      >
    </BaseDialog>
    <div
      v-if="showCreate"
      class="fixed inset-0 z-50 flex items-center justify-center bg-gray-950/50 p-4 backdrop-blur-sm"
      @click.self="showCreate = false"
    >
      <form
        class="max-h-[90vh] w-full max-w-2xl overflow-y-auto rounded-2xl bg-white p-6 shadow-2xl dark:bg-dark-900"
        @submit.prevent="create"
      >
        <div class="flex items-start justify-between">
          <div>
            <h2 class="text-xl font-bold">新建工单</h2>
            <p class="mt-1 text-sm text-gray-500">
              可以直接将截图粘贴到问题描述框中。
            </p>
          </div>
          <button
            type="button"
            class="btn btn-ghost btn-sm"
            @click="showCreate = false"
          >
            <Icon name="close" />
          </button>
        </div>
        <div class="mt-6 space-y-4">
          <label class="block"
            ><span class="label">标题</span
            ><input
              v-model="form.title"
              class="input mt-1 w-full"
              required
              maxlength="200" /></label
          ><label class="block"
            ><span class="label">问题描述</span
            ><textarea
              v-model="form.description"
              class="input mt-1 min-h-40 w-full resize-y"
              required
              maxlength="10000"
              @paste="handlePaste($event, form.images)"
            />
          </label>
          <div class="flex items-center gap-3">
            <label class="btn btn-secondary btn-sm cursor-pointer"
              ><input
                type="file"
                accept="image/*"
                multiple
                class="hidden"
                @change="pickFiles($event, form.images)"
              /><Icon name="upload" size="sm" class="mr-1.5" />添加图片</label
            ><span class="text-xs text-gray-500"
              >{{ form.images.length }}/9</span
            >
          </div>
          <TicketImageAttachments
            class="mt-3"
            :images="form.images"
            removable
            @preview="previewImage"
            @remove="form.images.splice($event, 1)"
          />
        </div>
        <div class="mt-6 flex justify-end gap-3">
          <button
            type="button"
            class="btn btn-secondary"
            @click="showCreate = false"
          >
            取消</button
          ><button
            class="btn btn-primary"
            :disabled="busy || !form.title.trim() || !form.description.trim()"
          >
            {{ busy ? "提交中…" : "提交工单" }}
          </button>
        </div>
      </form>
    </div>
    <TicketImageLightbox v-model="lightboxImage" />
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from "vue";
import { useI18n } from "vue-i18n";
import AppLayout from "@/components/layout/AppLayout.vue";
import TablePageLayout from "@/components/layout/TablePageLayout.vue";
import DataTable from "@/components/common/DataTable.vue";
import Pagination from "@/components/common/Pagination.vue";
import BaseDialog from "@/components/common/BaseDialog.vue";
import Icon from "@/components/icons/Icon.vue";
import TicketImageAttachments from "@/components/common/TicketImageAttachments.vue";
import TicketImageLightbox from "@/components/common/TicketImageLightbox.vue";
import ticketsAPI, { type Ticket } from "@/api/tickets";
import type { Column } from "@/components/common/types";
const { t } = useI18n();
const tickets = ref<Ticket[]>([]);
const selected = ref<Ticket | null>(null);
const showCreate = ref(false);
const busy = ref(false);
const error = ref("");
const replyText = ref("");
const replyImages = ref<string[]>([]);
const lightboxImage = ref("");
const newMessageCount = ref(0);
const messageListRef = ref<HTMLElement | null>(null);
const form = ref({ title: "", description: "", images: [] as string[] });
const pagination = ref({ page: 1, page_size: 20, total: 0 });
const columns = computed<Column[]>(() => [
  { key: "title", label: "标题" },
  { key: "status", label: "状态" },
  { key: "updated_at", label: "最近更新" },
  { key: "actions", label: "操作" },
]);
const formatDate = (v: string) => new Date(v).toLocaleString();
const statusLabel = (s: string) =>
  s === "closed" ? "已关闭" : s === "processing" ? "处理中" : "待处理";
const statusClass = (s: string) =>
  s === "closed"
    ? "badge-gray"
    : s === "processing"
      ? "badge-info"
      : "badge-warning";
const previewImage = (image: string) => {
  lightboxImage.value = image;
};
const isAtBottom = () => {
  const e = messageListRef.value;
  return !e || e.scrollHeight - e.scrollTop - e.clientHeight < 24;
};
async function scrollMessagesToBottom() {
  await nextTick();
  if (messageListRef.value)
    messageListRef.value.scrollTop = messageListRef.value.scrollHeight;
  newMessageCount.value = 0;
}
function handleMessageScroll() {
  if (isAtBottom()) newMessageCount.value = 0;
}
async function readFiles(files: File[]) {
  return Promise.all(
    files
      .filter((f) => f.type.startsWith("image/"))
      .map(
        (file) =>
          new Promise<string>((resolve, reject) => {
            const reader = new FileReader();
            reader.onload = () => resolve(String(reader.result));
            reader.onerror = reject;
            reader.readAsDataURL(file);
          }),
      ),
  );
}
async function pickFiles(event: Event, target: string[]) {
  const input = event.target as HTMLInputElement;
  target.push(
    ...(await readFiles(
      Array.from(input.files || []).slice(0, 9 - target.length),
    )),
  );
  input.value = "";
}
async function handlePaste(event: ClipboardEvent, target: string[]) {
  const files = Array.from(event.clipboardData?.items || [])
    .filter((item) => item.type.startsWith("image/"))
    .map((item) => item.getAsFile())
    .filter((f): f is File => !!f);
  if (files.length) {
    event.preventDefault();
    target.push(...(await readFiles(files.slice(0, 9 - target.length))));
  }
}
async function refreshSelected(silent: boolean) {
  if (!selected.value) return;
  const previous = selected.value.messages?.at(-1)?.id;
  const wasAtBottom = isAtBottom();
  const detail = await ticketsAPI.get(selected.value.id);
  const latest = detail.messages?.at(-1)?.id;
  selected.value = detail;
  if (!silent) {
    await scrollMessagesToBottom();
  } else if (previous && latest && previous !== latest) {
    if (wasAtBottom) await scrollMessagesToBottom();
    else newMessageCount.value += 1;
  }
}
async function load(silent = false) {
  if (!silent) busy.value = true;
  try {
    const result = await ticketsAPI.list({
      page: pagination.value.page,
      page_size: pagination.value.page_size,
    });
    tickets.value = result.items;
    pagination.value = {
      page: result.page,
      page_size: result.page_size,
      total: result.total,
    };
    if (selected.value && result.items.some((t) => t.id === selected.value?.id))
      await refreshSelected(silent);
  } catch (e) {
    if (!silent) error.value = String(e);
  } finally {
    if (!silent) busy.value = false;
  }
}
async function openTicket(ticket: Ticket) {
  try {
    selected.value = await ticketsAPI.get(ticket.id);
    await scrollMessagesToBottom();
  } catch (e) {
    error.value = String(e);
  }
}
async function create() {
  busy.value = true;
  try {
    const ticket = await ticketsAPI.create(form.value);
    form.value = { title: "", description: "", images: [] };
    showCreate.value = false;
    pagination.value.page = 1;
    await load();
    await openTicket(ticket);
  } catch (e) {
    error.value = String(e);
  } finally {
    busy.value = false;
  }
}
async function reply() {
  if (!selected.value || (!replyText.value.trim() && !replyImages.value.length))
    return;
  busy.value = true;
  try {
    await ticketsAPI.reply(selected.value.id, {
      content: replyText.value,
      images: replyImages.value,
    });
    replyText.value = "";
    replyImages.value = [];
    await openTicket(selected.value);
    await load(true);
  } catch (e) {
    error.value = String(e);
  } finally {
    busy.value = false;
  }
}
async function closeTicket(ticket: Ticket) {
  busy.value = true;
  try {
    await ticketsAPI.close(ticket.id);
    if (selected.value?.id === ticket.id) selected.value = null;
    await load();
  } catch (e) {
    error.value = String(e);
  } finally {
    busy.value = false;
  }
}
function changePage(page: number) {
  pagination.value.page = page;
  void load();
}
function changePageSize(size: number) {
  pagination.value.page_size = size;
  pagination.value.page = 1;
  void load();
}
let refreshTimer: ReturnType<typeof setInterval> | undefined;
onMounted(() => {
  void load();
  refreshTimer = setInterval(() => {
    if (!busy.value) void load(true);
  }, 15000);
});
onBeforeUnmount(() => {
  if (refreshTimer) clearInterval(refreshTimer);
});
</script>
