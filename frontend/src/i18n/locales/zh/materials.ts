export default {
  materials: {
    title: '素材库',
    description: '你的私人图片/音频/视频素材。演练台的图片输入控件也从这里选取。',
    // tabs / 分类
    kindImage: '图片',
    kindAudio: '音频',
    kindVideo: '视频',
    // 工具栏
    uploadBtn: '上传',
    importUrlBtn: '从 URL 导入',
    importUrlConfirm: '导入',
    importToLibraryBtn: '导入到素材库',
    searchPlaceholder: '按文件名搜索',
    fromLibrary: '从素材库',
    pasteUrl: '粘贴 URL',
    openLink: '打开',
    // 列表
    empty: '暂无素材。可以上传一张图片，或从 URL 导入。',
    // 弹窗
    pickerTitle: '选择素材',
    // 提示
    uploadSuccess: '已上传到素材库',
    confirmRemove: '确定要删除素材"{name}"吗？（删除后其他任务中已使用该素材的链接会失效）',
    imageInputEmptyHint: '点击上方按钮上传图片、从素材库选择或粘贴 URL。',
    pageInfo: '第 {page} / {total} 页',
    // 分页按钮：不要复用 common.prev/common.next —— common.next 是向导语义的
    // "下一步"，而 common.prev 根本没定义（只能靠 fallback 显示英文）。
    prevPage: '上一页',
    nextPage: '下一页',
    // ---- 素材库弹窗多选（图片组控件使用）----
    selectedCount: '已选 {n} 张',
    remainingSlots: '还可选 {n} 张',
    confirmPick: '确认选择',
    maxSelectReached: '最多只能再选 {n} 张',
    // ---- 图片组控件（array + widget=imageUrls）----
    imageUrlsTitle: '图片组',
    imageUrlsEmptyTitle: '添加图片',
    imageUrlsEmptyHint: '可拖拽图片到此处，或从素材库选择 / 粘贴 URL',
    addImage: '添加',
    clearAll: '清空',
    pasteUrls: '粘贴 URL',
    pasteUrlsPlaceholder: 'https://...\nhttps://...\n（一行一个）',
    pasteUrlsHint: '一行一个，会先导入到素材库再填入',
    thumbBroken: '无法预览',
    uploadingProgress: '上传中 {i}/{n}',
    importingProgress: '导入中 {i}/{n}',
    addedCount: '已添加 {n} 张',
    uploadPartialFailed: '{n} 张上传失败',
    importPartialFailed: '{n} 个 URL 导入失败',
    maxItemsReached: '已达上限 {n} 张',
    maxItemsSkipped: '超出上限，已忽略 {n} 张',
  },
}
