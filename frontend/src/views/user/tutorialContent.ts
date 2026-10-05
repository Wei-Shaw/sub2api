// 教程正文内容(中文)。
// - UI 框架文案(标签/按钮/搜索提示)在 i18n 的 tutorial 段;正文更新频率高且站点受众为中文,
//   故独立成模块,不进 i18n(en 用户通过 fallbackLocale=zh 回退看到中文正文)。
// - 内容与部署仓库 docs/user-help.md 保持同步,新增客户端/FAQ 两边都要改。
// - base 为站点接入地址(运行时取当前站点 origin),model 为用户在页面上下拉选中的模型。

export type EndpointKey = 'openai' | 'anthropic' | 'responses' | 'models'

export interface TutorialFile {
  path: string
  language: string
  content: string
}

export interface TutorialStep {
  text: string
  commands?: string[]
  appendText?: string
  appendCommands?: string[]
  file?: TutorialFile
}

export interface TutorialGuide {
  key: string
  name: string
  endpoint: EndpointKey
  intro?: string
  steps: TutorialStep[]
  extra?: { text: string; commands: string[] }
  tip?: string
}

export interface FaqItem {
  id: string
  category: 'config' | 'billing' | 'quality' | 'support'
  q: string
  a: string
}

export const TUTORIAL_MODELS = [
  'deepseek-v4-pro',
  'deepseek-v4-flash',
  'qwen3.8-max',
  'qwen3.8-flash',
  'kimi-k2',
  'glm-5.3',
]

export const FAQ_CATEGORIES = [
  { key: 'all', label: '全部' },
  { key: 'config', label: '接入与报错' },
  { key: 'billing', label: '计费与扣费' },
  { key: 'quality', label: '线路与质量' },
  { key: 'support', label: '账户与支持' },
] as const

export function buildEndpoints(base: string): { key: EndpointKey; name: string; url: string; desc: string }[] {
  return [
    { key: 'openai', name: 'OpenAI 兼容', url: `${base}/v1/chat/completions`, desc: 'Cherry Studio、Next-Web、LobeChat、Cursor、沉浸式翻译等' },
    { key: 'anthropic', name: 'Anthropic 兼容', url: base, desc: 'Claude Code · 地址不带 /v1' },
    { key: 'responses', name: 'OpenAI Responses', url: `${base}/v1/responses`, desc: 'Codex CLI' },
    { key: 'models', name: '模型列表', url: `${base}/v1/models`, desc: 'GET 请求 · 查看当前可用模型' },
  ]
}

function claudeSettings(base: string, model: string): string {
  return JSON.stringify(
    {
      env: {
        ANTHROPIC_BASE_URL: base,
        ANTHROPIC_AUTH_TOKEN: 'sk-你的密钥',
        ANTHROPIC_MODEL: model,
        ANTHROPIC_DEFAULT_OPUS_MODEL: model,
        ANTHROPIC_DEFAULT_SONNET_MODEL: model,
        ANTHROPIC_DEFAULT_HAIKU_MODEL: model,
      },
      hasCompletedOnboarding: true,
    },
    null,
    2,
  )
}

export function buildGuides(base: string, model: string): TutorialGuide[] {
  return [
    {
      key: 'claude-code',
      name: 'Claude Code',
      endpoint: 'anthropic',
      intro: 'Claude Code 通过 Anthropic 协议接入,本站原生支持,无需任何插件。',
      steps: [
        { text: '安装或更新 Node.js(v18.0 或更高版本)。已安装过的可先执行 node --version 检查,满足则跳过此步。' },
        { text: '在终端中执行下列命令,安装 Claude Code:', commands: ['npm install -g @anthropic-ai/claude-code'] },
        { text: '运行以下命令验证安装。若有版本号输出,则表示安装成功:', commands: ['claude --version'] },
        {
          text: '创建并打开配置文件 ~/.claude/settings.json。如果 .claude 目录不存在,需要先行创建:',
          commands: ['mkdir -p ~/.claude'],
          appendText: '然后打开配置文件:',
          appendCommands: ['nano ~/.claude/settings.json'],
        },
        {
          text: '编辑配置文件,将以下内容写入并保存(Windows 路径为 C:\\Users\\你的用户名\\.claude\\settings.json):',
          file: { path: '~/.claude/settings.json', language: 'json', content: claudeSettings(base, model) },
        },
        { text: '保存配置文件,重新打开一个终端即生效。在终端运行 claude,看到欢迎界面并能正常对话回复,即配置完成。' },
      ],
      extra: {
        text: '免配置文件的临时用法:直接用环境变量启动,适合快速试用:',
        commands: [`export ANTHROPIC_BASE_URL="${base}"`, 'export ANTHROPIC_AUTH_TOKEN="sk-你的密钥"', 'claude'],
      },
      tip: '在「API 密钥」页点击「导入到 CCS」,可将密钥一键写入 cc-switch,在多套 Claude Code 配置之间快速切换。',
    },
    {
      key: 'codex',
      name: 'Codex CLI',
      endpoint: 'responses',
      intro: 'Codex CLI 通过 OpenAI Responses 协议接入,同样原生支持。',
      steps: [
        { text: '在终端中安装 Codex CLI:', commands: ['npm install -g @openai/codex'] },
        {
          text: '创建并打开配置文件 ~/.codex/config.toml:',
          commands: ['mkdir -p ~/.codex'],
          appendText: '然后打开配置文件:',
          appendCommands: ['nano ~/.codex/config.toml'],
        },
        {
          text: '写入模型服务配置并保存:',
          file: {
            path: '~/.codex/config.toml',
            language: 'toml',
            content: `model_provider = "wende"
preferred_auth_method = "apikey"
model = "${model}"

[model_providers.wende]
name = "wende-ai"
base_url = "${base}/v1"
wire_api = "responses"
requires_openai_auth = true`,
          },
        },
        {
          text: '打开(不存在会自动新建)~/.codex/auth.json,写入密钥:',
          commands: ['nano ~/.codex/auth.json'],
          file: {
            path: '~/.codex/auth.json',
            language: 'json',
            content: JSON.stringify({ OPENAI_API_KEY: 'sk-你的密钥' }, null, 2),
          },
        },
        { text: '在终端运行 codex,发送一句话能得到正常回复,即配置完成。Windows 用户路径为 C:\\Users\\你的用户名\\.codex\\。' },
      ],
    },
    {
      key: 'cherry-studio',
      name: 'Cherry Studio',
      endpoint: 'openai',
      steps: [
        { text: '打开 设置 → 模型服务 → 添加供应商,协议选择 OpenAI 兼容。' },
        { text: 'API 地址填本站接入地址(右侧 OpenAI 兼容卡片,不带 /v1 也可),API Key 填你的 sk- 密钥。' },
        { text: '点击「获取模型列表」自动拉取,或手动添加模型名,对话时选择模型即可。' },
      ],
    },
    {
      key: 'nextweb-lobechat',
      name: 'Next-Web / LobeChat',
      endpoint: 'openai',
      steps: [
        { text: '设置中接口类型选择 OpenAI。' },
        { text: 'API 地址填本站接入地址(无需再拼 /v1,客户端会自动拼接),API Key 填你的 sk- 密钥。' },
        { text: '添加模型名后保存,即可开始对话。' },
      ],
    },
    {
      key: 'immersive-translate',
      name: '沉浸式翻译',
      endpoint: 'openai',
      steps: [
        { text: '设置 → AI 服务 → OpenAI。' },
        { text: '自定义 API 接口地址填完整路径(右侧 OpenAI 兼容卡片的地址)。' },
        { text: '填入 API Key 与模型名,保存后即可翻译。' },
      ],
    },
    {
      key: 'cursor',
      name: 'Cursor',
      endpoint: 'openai',
      steps: [
        { text: 'Settings → Models → OpenAI API Key。' },
        { text: 'Base URL 填本站接入地址加 /v1 后缀,并填入你的 API Key。' },
        { text: '手动添加模型名,添加完成后点击 Verify 验证,出现绿色对勾即接入成功。' },
      ],
    },
    {
      key: 'api',
      name: '代码调用',
      endpoint: 'openai',
      intro: '直接调用 OpenAI 兼容接口,任选一种语言,把 model 换成上方选中的模型。',
      steps: [],
    },
  ]
}

export const FAQ_ITEMS: FaqItem[] = [
  { id: 'invalid-key', category: 'config', q: '报错 API_KEY_REQUIRED 或 401 无效密钥?', a: '检查请求头是否为 Authorization: Bearer sk-...;确认密钥未过期、未禁用、未删除。密钥只在创建时完整展示一次,丢失请在「API 密钥」页重建。' },
  { id: 'claude-model-missing', category: 'config', q: 'Claude Code 报「模型不存在」或 No available accounts?', a: 'Claude Code 默认请求 claude-sonnet-*、claude-haiku-* 等模型名,站内没有对应通道时就会报错。按上方步骤在配置里把默认模型映射到本站模型(如 deepseek-v4-pro)即可解决。' },
  { id: 'no-accounts', category: 'config', q: '报错 No available accounts?', a: '该模型的后端服务池暂时没有可用通道,一般稍后重试或切换其他模型即可;也可在「可用渠道」页确认当前密钥分组是否接入该模型。如果持续出现,请联系管理员。' },
  { id: 'compat', category: 'config', q: '和官方 API 格式完全兼容吗?', a: '兼容 OpenAI Chat Completions 与 Anthropic Messages 两种格式,现有代码只需替换 base_url 与密钥,无需修改业务逻辑。' },
  { id: 'cli-support', category: 'config', q: '支持 Claude Code、Codex 这类命令行工具吗?', a: '支持。Claude Code 走 Anthropic 协议、Codex CLI 走 Responses 协议,均已原生适配,按上方配置即可。' },
  { id: 'insufficient-balance', category: 'billing', q: '提示余额不足?', a: '请前往控制台充值后再试。充值前可先在「用量」页核对消费明细。' },
  { id: 'billing-how', category: 'billing', q: '计费是怎么算的?什么时候扣余额?', a: '按量计费,请求完成后按「模型单价 × 分组倍率」从余额实时扣除,输入、输出 Token 分别计价,每笔扣费均可在「使用记录」页按时间、模型逐笔核对。' },
  { id: 'price-diff', category: 'billing', q: '为什么同一个模型价格不一样?', a: '不同分组对应不同的上游线路与计费倍率,创建密钥时选定的分组即最终价格。可在「可用渠道」页查看各分组支持的模型。' },
  { id: 'token-arrows', category: 'billing', q: '使用记录里的 ↑2k(1.8k)、↓500(200) 是什么意思?', a: '向上箭头是输入 Token(括号内为缓存读取),向下箭头是输出 Token(括号内为缓存写入)。缓存命中部分计价更低,命中越多越省。' },
  { id: 'plan-vs-balance', category: 'billing', q: '充值余额和订阅套餐怎么选?', a: '偶尔用、模型需求多变,充余额更灵活;每天都在用、模型固定,套餐更划算。两者可同时持有,具体额度与限制以「充值 / 订阅」页说明为准。' },
  { id: 'redeem-code', category: 'billing', q: '兑换码在哪里使用?充值后没到账怎么办?', a: '登录后进入「兑换」页输入兑换码即可充值;兑换码区分大小写,通常只能使用一次。显示成功但余额未变化时先刷新页面,仍未到账请联系客服并提供兑换码与时间。' },
  { id: 'errors-429', category: 'quality', q: '报错 429 / 5xx?', a: '429 多为触发了密钥的并发或频率限额,降低请求频率即可;5xx 多为上游瞬时故障,系统会自动重试切换。持续报错可在「渠道状态」页查看线路健康度,或联系管理员调整限额配置。' },
  { id: 'fidelity', category: 'quality', q: '模型是原版吗?会不会降智或截断上下文?', a: '请求按原参数转发到对应上游接口,不替换模型、不改写内容、不截断上下文;同一对话自动路由到相同上游,保证上下文连贯。线路健康度可在「渠道状态」页查看。' },
  { id: 'refund', category: 'support', q: '使用不满意可以退款吗?', a: '未使用的余额可联系客服按充值渠道申请原路退回;订阅套餐属于即时生效的虚拟服务,开通后一般不支持退款,具体以站内条款与客服确认结果为准。' },
  { id: 'contact', category: 'support', q: '遇到问题如何获得支持?', a: '登录后「个人资料」页展示客服联系方式,咨询时提供密钥名称、时间与报错信息能更快定位;服务变更与故障通报见「公告」。' },
  { id: 'key-safety', category: 'support', q: '如何保证密钥安全?', a: '密钥等同于账户凭证,请勿提交到代码仓库、勿暴露在前端页面;发现泄露立即在「API 密钥」页删除并重建。' },
]
