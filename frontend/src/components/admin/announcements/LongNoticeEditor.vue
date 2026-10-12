<template>
  <section class="mb-5 rounded-xl border border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-800">
    <header class="flex flex-wrap items-center justify-between gap-3 border-b border-gray-100 px-5 py-4 dark:border-dark-700">
      <div><h2 class="font-semibold text-gray-900 dark:text-white">长期通知</h2><p class="mt-1 text-xs text-gray-500 dark:text-gray-400">编辑铃铛“通知”标签中的长期运营内容</p></div>
      <div class="flex gap-2"><button class="btn btn-secondary" type="button" @click="previewOpen = !previewOpen">{{ previewOpen ? '收起预览' : '实时预览' }}</button><button class="btn btn-primary" type="button" :disabled="saving" @click="save">{{ saving ? '保存中' : '保存长期通知' }}</button></div>
    </header>
    <div class="grid gap-5 p-5" :class="previewOpen ? 'lg:grid-cols-[minmax(0,1fr)_minmax(320px,420px)]' : ''">
      <div class="space-y-3">
        <div v-for="(block, index) in document.blocks" :key="block.id" class="rounded-lg border border-gray-200 p-4 dark:border-dark-700">
          <div class="mb-3 flex flex-wrap items-center gap-2">
            <select v-model="block.type" class="input max-w-36"><option v-for="option in blockTypes" :key="option.value" :value="option.value">{{ option.label }}</option></select>
            <select v-model="block.tone" class="input max-w-28"><option v-for="tone in tones" :key="tone.value" :value="tone.value">{{ tone.label }}</option></select>
            <div class="ml-auto flex gap-1"><button class="newapi-icon-button" type="button" :disabled="index === 0" title="上移" @click="move(index, -1)">↑</button><button class="newapi-icon-button" type="button" :disabled="index === document.blocks.length - 1" title="下移" @click="move(index, 1)">↓</button><button class="newapi-icon-button text-red-500" type="button" title="删除" @click="document.blocks.splice(index, 1)">×</button></div>
          </div>
          <input v-if="['status','heading','feature','callout'].includes(block.type)" v-model="block.title" class="input" :placeholder="block.type === 'status' ? '例如：稳定服务中' : '标题'" />
          <div v-if="block.type === 'feature'" class="mt-2 grid gap-2 sm:grid-cols-[7rem_minmax(0,1fr)]">
            <input v-model="block.number" class="input" placeholder="编号，例如 01" />
            <input v-model="block.highlight" class="input" placeholder="高亮文字（可选）" />
          </div>
          <input v-if="block.type === 'feature'" v-model="tagsText[index]" class="input mt-2" placeholder="标签，用逗号分隔，例如 GPT,Codex" @change="syncTags(block, index)" />
          <textarea v-if="['text','callout'].includes(block.type)" v-model="block.content" class="input mt-2" rows="3" placeholder="内容"></textarea>
          <textarea v-if="block.type === 'feature'" v-model="block.content" class="input mt-2" rows="2" placeholder="模块说明"></textarea>
          <div v-if="block.type === 'metrics'" class="space-y-2">
            <div v-for="(item, itemIndex) in block.metrics" :key="itemIndex" class="flex gap-2"><input v-model="item.label" class="input" placeholder="名称，例如 GPT" /><input v-model="item.value" class="input" placeholder="数值，例如 0.2 倍" /><button type="button" class="newapi-icon-button text-red-500" @click="block.metrics?.splice(itemIndex,1)">×</button></div>
            <button type="button" class="btn btn-secondary" @click="block.metrics?.push({ label: '', value: '' })">添加数据</button>
          </div>
          <div v-if="block.type === 'buttons'" class="space-y-2">
            <div v-for="(item, itemIndex) in block.buttons" :key="itemIndex" class="grid grid-cols-[1fr_1.6fr_auto] gap-2"><input v-model="item.label" class="input" placeholder="按钮名称" /><input v-model="item.url" class="input" placeholder="https://..." /><button type="button" class="newapi-icon-button text-red-500" @click="block.buttons?.splice(itemIndex,1)">×</button></div>
            <button type="button" class="btn btn-secondary" @click="block.buttons?.push({ label: '', url: '' })">添加按钮</button>
          </div>
        </div>
        <div class="flex flex-wrap gap-2"><button v-for="option in blockTypes" :key="option.value" type="button" class="btn btn-secondary" @click="addBlock(option.value)">+ {{ option.label }}</button></div>
      </div>
      <aside v-if="previewOpen" class="self-start rounded-xl border border-gray-200 bg-white p-4 shadow-sm dark:border-dark-700 dark:bg-dark-900 lg:sticky lg:top-20"><div class="mb-3 text-sm font-semibold text-gray-900 dark:text-white">通知预览</div><LongNoticeRenderer :document="document" /><p v-if="document.blocks.length === 0" class="py-12 text-center text-sm text-gray-400">添加模块后在这里预览</p></aside>
    </div>
  </section>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { adminAPI } from '@/api/admin'
import { useAppStore } from '@/stores/app'
import LongNoticeRenderer from '@/components/common/LongNoticeRenderer.vue'
import { emptyLongNoticeDocument, parseLongNoticeDocument, type LongNoticeBlockType, type LongNoticeDocument } from '@/types/longNotice'
const appStore = useAppStore()
const document = ref<LongNoticeDocument>(emptyLongNoticeDocument())
const saving = ref(false)
const previewOpen = ref(true)
const blockTypes = [{ value: 'status', label: '状态条' }, { value: 'heading', label: '标题' }, { value: 'text', label: '正文' }, { value: 'metrics', label: '数据块' }, { value: 'feature', label: '功能模块' }, { value: 'callout', label: '彩色提示' }, { value: 'buttons', label: '跳转按钮' }] as const
const tones = [{ value: 'blue', label: '蓝色' }, { value: 'green', label: '绿色' }, { value: 'amber', label: '橙色' }, { value: 'red', label: '红色' }, { value: 'gray', label: '灰色' }] as const
const tagsText = ref<string[]>([])
function syncTags(block: LongNoticeDocument['blocks'][number], index: number) { block.tags = (tagsText.value[index] || '').split(',').map(tag => tag.trim()).filter(Boolean) }
function addBlock(type: LongNoticeBlockType) {
  document.value.blocks.push({ id: crypto.randomUUID(), type, tone: type === 'status' ? 'green' : 'blue', title: '', content: '', number: type === 'feature' ? '' : undefined, tags: type === 'feature' ? [] : undefined, highlight: type === 'feature' ? '' : undefined, metrics: type === 'metrics' ? [{ label: '', value: '' }] : undefined, buttons: type === 'buttons' ? [{ label: '', url: '' }] : undefined })
  tagsText.value.push('')
}
function move(index: number, offset: number) { const target = index + offset; if (target < 0 || target >= document.value.blocks.length) return; const [item] = document.value.blocks.splice(index, 1); document.value.blocks.splice(target, 0, item) }
function validate() { for (const block of document.value.blocks) for (const button of block.buttons || []) { try { const url = new URL(button.url); if (!['http:', 'https:'].includes(url.protocol)) throw new Error() } catch { throw new Error(`按钮“${button.label || '未命名'}”的链接无效`) } } }
async function save() { try { validate(); saving.value = true; await adminAPI.settings.updateSettings({ long_notice_content: JSON.stringify(document.value) }); await appStore.fetchPublicSettings(true); appStore.showSuccess('长期通知已保存') } catch (error: any) { appStore.showError(error?.message || error?.response?.data?.detail || '保存失败') } finally { saving.value = false } }
onMounted(async () => { try { const settings = await adminAPI.settings.getSettings(); document.value = parseLongNoticeDocument(settings.long_notice_content); tagsText.value = document.value.blocks.map(block => (block.tags || []).join(',')) } catch { appStore.showError('长期通知加载失败') } })
</script>
