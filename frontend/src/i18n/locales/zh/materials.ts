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
    openLink: '打开',
    rename: '改名',
    renameTitle: '修改素材名称',
    renamePlaceholder: '输入新的素材名称',
    renameSuccess: '素材名称已更新',
    previewImage: '点击查看大图',
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
    selectedCount: '已选 {n} 个',
    remainingSlots: '还可选 {n} 个',
    confirmPick: '确认选择',
    maxSelectReached: '最多只能再选 {n} 个',
    // ---- 媒体 URL 组控件 ----
    imageUrlsTitle: '图片组',
    imageUrlsEmptyTitle: '添加图片',
    videoUrlsTitle: '视频组',
    videoUrlsEmptyTitle: '添加视频',
    audioUrlsTitle: '音频组',
    audioUrlsEmptyTitle: '添加音频',
    mediaUrlsEmptyHint: '可拖拽文件到此处，支持 {extensions}，也可从素材库选择 / 粘贴 URL',
    addMedia: '添加',
    clearAll: '清空',
    // 两个来源按钮的文案统一复用 importUrlBtn（'从 URL 导入'），
    // 因此不再需要单独的 pasteUrl / pasteUrls。
    pasteUrlsPlaceholder: 'https://...\nhttps://...\n（一行一个）',
    pasteUrlsHint: '一行一个，会先导入到素材库再填入',
    thumbBroken: '无法预览',
    uploadingProgress: '上传中 {i}/{n}',
    importingProgress: '导入中 {i}/{n}',
    addedCount: '已添加 {n} 个',
    uploadPartialFailed: '{n} 个上传失败',
    importPartialFailed: '{n} 个 URL 导入失败',
    // 带原因的版本：批量场景下只说"N 张失败"无法排查，带上首个失败原因
    uploadPartialFailedWithReason: '{n} 个上传失败：{msg}',
    importPartialFailedWithReason: '{n} 个 URL 导入失败：{msg}',
    maxItemsReached: '已达上限 {n} 个',
    maxItemsSkipped: '超出上限，已忽略 {n} 个',
    invalidMediaFiles: '已忽略 {n} 个不支持的文件；仅支持：{extensions}',
    invalidMediaUrls: '已忽略 {n} 个后缀不支持的 URL；仅支持：{extensions}',
    mediaKindMismatch: '文件实际媒体类型与当前控件不一致，已忽略。',
    // ---- 后端错误码 → 友好文案 ----
    // 键名必须与后端 reason 严格一致（见 service/user_material.go 与 cos_transfer.go），
    // 由 extractI18nErrorMessage 按 reason 自动查表；查不到时回落到后端原始 message。
    errors: {
      COS_NOT_CONFIGURED: '素材库尚不可用：需要管理员先在「系统设置 → 图片转存（COS / S3 兼容）」中填好存储桶与密钥并勾选“启用图片转存”。',
      URL_BLOCKED: '该地址不被允许（指向内网或本机地址），请换一个公网可访问的图片链接。',
      URL_FETCH_FAILED: '无法访问该链接，请确认地址可公开访问且未过期。',
      EMPTY_REMOTE_FILE: '该链接返回的内容为空，请确认图片地址是否正确。',
      EMPTY_FILE: '文件内容为空。',
      FILE_TOO_LARGE: '文件体积超出限制。',
      UNSUPPORTED_CONTENT_TYPE: '不支持该文件类型，请上传图片、音频或视频。',
      UNSUPPORTED_KIND: '不支持该素材类型。',
      INVALID_FILE_NAME: '素材名称不能为空，且不能超过 512 个字符。',
      MATERIAL_NOT_FOUND: '素材不存在、已删除或不属于当前用户。',
      INVALID_URL: '链接格式不正确，需以 http:// 或 https:// 开头。',
      MATERIAL_COUNT_QUOTA_EXCEEDED: '素材数量已达上限，请先删除一些不再使用的素材。',
      MATERIAL_SIZE_QUOTA_EXCEEDED: '素材总容量已达上限，请先删除一些不再使用的素材。',
    },
  },
}
