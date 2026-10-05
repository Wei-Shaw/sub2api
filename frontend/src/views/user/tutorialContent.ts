// 教程正文内容(中文)。
// - UI 框架文案(标签/按钮/搜索提示)在 i18n 的 tutorial 段;正文更新频率高且站点受众为中文,
//   故独立成模块,不进 i18n(en 用户通过 fallbackLocale=zh 回退看到中文正文)。
// - 客户端清单与 krill-code 使用教程对齐(2026-10 从其线上产物提取步骤结构,接入地址替换为本站);
//   内容同时镜像到部署仓库 docs/user-help.md,两处更新需同步。
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
  /** 客户端官方图标(public/tutorial-logos/ 下的 SVG 路径) */
  icon?: string
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
    { key: 'openai', name: 'OpenAI 兼容', url: `${base}/v1/chat/completions`, desc: 'Cherry Studio、Next-Web、LobeChat、Cursor、TRAE SOLO、WorkBuddy 等' },
    { key: 'anthropic', name: 'Anthropic 兼容', url: base, desc: 'Claude Code · 地址不带 /v1' },
    { key: 'responses', name: 'OpenAI Responses', url: `${base}/v1`, desc: 'Codex CLI · dsh' },
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

function opencodeConfig(base: string, model: string): string {
  return JSON.stringify(
    {
      $schema: 'https://opencode.ai/config.json',
      provider: {
        wende: {
          npm: '@ai-sdk/openai',
          name: '稳得 AI',
          options: {
            baseURL: `${base}/v1`,
            apiKey: 'sk-你的密钥',
          },
          models: {
            [model]: {
              name: model,
              reasoning: true,
              tool_call: true,
              limit: { context: 272000, output: 128000 },
              options: {
                reasoningEffort: 'medium',
                textVerbosity: 'low',
                reasoningSummary: 'auto',
                store: false,
              },
              variants: {
                low: { reasoningEffort: 'low', textVerbosity: 'low' },
                medium: { reasoningEffort: 'medium', textVerbosity: 'low' },
                high: { reasoningEffort: 'high', textVerbosity: 'low' },
                xhigh: { reasoningEffort: 'xhigh', textVerbosity: 'low' },
              },
            },
          },
        },
      },
    },
    null,
    2,
  )
}

function openclawConfig(base: string, model: string): string {
  return JSON.stringify(
    {
      models: {
        mode: 'merge',
        providers: {
          wende: {
            baseUrl: `${base}/v1`,
            apiKey: 'sk-你的密钥',
            api: 'openai',
            models: [
              {
                id: model,
                name: model,
                reasoning: false,
                input: ['text', 'image'],
                contextWindow: 1000000,
                maxTokens: 65536,
              },
            ],
          },
        },
      },
      agents: {
        defaults: {
          model: { primary: `wende/${model}` },
          models: { [`wende/${model}`]: {} },
        },
      },
      gateway: { mode: 'local' },
    },
    null,
    2,
  )
}

function dshSettings(base: string, model: string): string {
  return `llm-pi-ai:
  providers:
    wende:
      displayName: 稳得 AI
      apiKeyEnv: WENDE_API_KEY
      api: openai-responses
      baseURL: ${base}/v1
      models:
        - id: ${model}
          name: ${model} · 稳得 AI
          contextWindow: 272000
          maxTokens: 128000
          input: [text, image]

agent-default-model:
  provider: wende
  model: ${model}`
}

function hermesConfig(base: string, model: string): string {
  return `model:
  default: ${model}
  provider: custom
  base_url: ${base}/v1
  api_key: sk-你的密钥`
}

export function buildGuides(base: string, model: string): TutorialGuide[] {
  return [
    {
      key: 'claude-code',
      icon: '/tutorial-logos/claude-code.svg',
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
      icon: '/tutorial-logos/codex.svg',
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
      key: 'dsh',
      icon: '/tutorial-logos/dsh.svg',
      name: 'dsh',
      endpoint: 'responses',
      intro: 'dsh(DeepSeek Harness)通过 OpenAI Responses 协议接入。',
      steps: [
        { text: 'dsh 需要 Node.js 22.19 或更高版本,请先安装并确认版本:', commands: ['node --version'] },
        { text: '在终端中执行下列命令,全局安装 dsh:', commands: ['npm install -g @deepseek-ai/dsh'] },
        { text: '运行以下命令验证安装。若有版本号输出,则表示安装成功:', commands: ['dsh --version'] },
        {
          text: '创建并打开配置文件 ~/.dsh/settings.yaml。如果 .dsh 目录不存在,需要先行创建:',
          commands: ['mkdir -p ~/.dsh'],
          appendText: '然后打开配置文件:',
          appendCommands: ['nano ~/.dsh/settings.yaml'],
        },
        {
          text: '写入以下模型服务配置并保存:',
          file: { path: '~/.dsh/settings.yaml', language: 'yaml', content: dshSettings(base, model) },
        },
        {
          text: '打开(不存在会自动新建)~/.dsh/.credentials.yaml,写入密钥:',
          commands: ['nano ~/.dsh/.credentials.yaml'],
          file: {
            path: '~/.dsh/.credentials.yaml',
            language: 'yaml',
            content: 'version: 1\n\nrefs:\n  WENDE_API_KEY: sk-你的密钥',
          },
        },
        {
          text: 'macOS / Linux 还需收紧凭据文件权限,否则 dsh 会拒绝加载该文件(Windows 无此限制):',
          commands: ['chmod 600 ~/.dsh/.credentials.yaml'],
        },
        { text: '在终端运行 dsh,发送一句话能得到正常回复,即配置完成。' },
      ],
    },
    {
      key: 'opencode',
      icon: '/tutorial-logos/opencode.svg',
      name: 'OpenCode',
      endpoint: 'openai',
      intro: 'OpenCode 通过 OpenAI 兼容协议接入,支持按 low / medium / high / xhigh 四档调整思考强度。',
      steps: [
        { text: '安装或更新 Node.js(v18.0 或更高版本)。' },
        { text: '在终端中执行以下命令安装 OpenCode:', commands: ['npm install -g opencode-ai'] },
        { text: '运行以下命令验证安装。若有版本号输出,则表示安装成功:', commands: ['opencode -v'] },
        {
          text: '创建并打开配置文件 ~/.config/opencode/opencode.json:',
          commands: ['mkdir -p ~/.config/opencode'],
          appendText: '然后打开配置文件:',
          appendCommands: ['nano ~/.config/opencode/opencode.json'],
        },
        {
          text: '将以下配置写入文件,保存后退出并重新启动 OpenCode 使新配置生效(Windows 路径为 C:\\Users\\你的用户名\\.config\\opencode\\opencode.json):',
          file: { path: '~/.config/opencode/opencode.json', language: 'json', content: opencodeConfig(base, model) },
        },
        { text: '在终端运行 opencode,看到对话界面并能正常回复,即配置完成。' },
      ],
    },
    {
      key: 'openclaw',
      icon: '/tutorial-logos/openclaw.svg',
      name: 'OpenClaw',
      endpoint: 'openai',
      intro: 'OpenClaw 通过自定义模型供应商接入。',
      steps: [
        { text: '在终端执行以下命令打开配置文件 ~/.openclaw/openclaw.json:', commands: ['nano ~/.openclaw/openclaw.json'] },
        {
          text: '首次配置:将以下内容复制到配置文件,把 sk-你的密钥 替换为你的 API Key。已有配置请勿直接全量替换,按结构合并进 models 与 agents 段:',
          file: { path: '~/.openclaw/openclaw.json', language: 'json', content: openclawConfig(base, model) },
        },
        {
          text: '保存文件并退出,运行以下命令使配置生效:',
          commands: ['openclaw gateway restart'],
        },
        { text: '启动后发送一条消息,能正常回复即配置完成。' },
      ],
    },
    {
      key: 'hermes',
      icon: '/tutorial-logos/hermes.svg',
      name: 'Hermes',
      endpoint: 'openai',
      intro: 'Hermes 通过 hermes config 命令或 YAML 配置文件接入。',
      steps: [
        {
          text: '在终端中执行以下命令,安装脚本会自动安装 Python、Git 等依赖(Windows 不支持原生安装,请先安装 WSL2 并在 WSL2 终端中运行):',
          commands: ['curl -fsSL https://raw.githubusercontent.com/NousResearch/hermes-agent/main/scripts/install.sh | bash'],
        },
        {
          text: '安装完成后,重新加载终端环境(macOS 执行 source ~/.zshrc,Linux 执行 source ~/.bashrc),然后验证安装:',
          commands: ['hermes --version'],
        },
        {
          text: '在终端中依次执行以下命令,配置模型提供商、Base URL、API Key 和默认模型:',
          commands: [
            'hermes config set model.provider custom',
            `hermes config set model.base_url ${base}/v1`,
            'hermes config set model.api_key sk-你的密钥',
            `hermes config set model.default ${model}`,
          ],
        },
        {
          text: '以上命令会将配置写入 ~/.hermes/config.yaml,也可以直接编辑该文件,写入以下内容:',
          file: { path: '~/.hermes/config.yaml', language: 'yaml', content: hermesConfig(base, model) },
        },
        {
          text: '配置完成后,可以通过 -m 参数在对话时切换模型:',
          commands: [`hermes chat -m ${model}`],
          appendText: '也可以直接设置默认模型:',
          appendCommands: [`hermes config set model.default ${model}`],
        },
        {
          text: '验证配置,执行以下命令发送一条测试消息。如果返回正常的 AI 回复,则配置成功;进入交互式对话模式直接执行 hermes:',
          commands: ['hermes chat -q "你好"'],
        },
      ],
    },
    {
      key: 'cherry-studio',
      icon: '/tutorial-logos/cherry-studio.svg',
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
      icon: '/tutorial-logos/chat-apps.svg',
      name: 'Next-Web / LobeChat',
      endpoint: 'openai',
      steps: [
        { text: '设置中接口类型选择 OpenAI。' },
        { text: 'API 地址填本站接入地址(无需再拼 /v1,客户端会自动拼接),API Key 填你的 sk- 密钥。' },
        { text: '添加模型名后保存,即可开始对话。' },
      ],
    },
    {
      key: 'workbuddy',
      icon: '/tutorial-logos/workbuddy.svg',
      name: 'WorkBuddy',
      endpoint: 'openai',
      intro: 'WorkBuddy 仅支持 OpenAI 兼容协议 API。',
      steps: [
        { text: '打开 WorkBuddy,点击左下角的账户头像,在弹出的账户菜单中点击「设置」。' },
        { text: '在设置窗口左侧选择「模型」,然后点击右上方的「+ 添加模型」。' },
        {
          text: '提供商选择「自定义 / Custom」,接口地址填完整路径(右侧 OpenAI 兼容卡片的地址),API Key 填你的 sk- 密钥,模型名称填上方选中的模型。根据模型能力确认高级配置后,点击「保存」。',
        },
        { text: '返回任务页面,在输入框右下角选择刚保存的模型。发送一条消息并收到正常回复,即表示配置成功。' },
      ],
      tip: '测试提示语可以用「你好,请确认当前模型是否连接成功。」。',
    },
    {
      key: 'zcode',
      icon: '/tutorial-logos/zcode.svg',
      name: 'ZCode',
      endpoint: 'openai',
      steps: [
        { text: '打开 ZCode 设置并进入「Model settings」,点击「+ Add provider」。供应商名称填 稳得 AI,API Base URL 填本站接入地址(右侧 OpenAI Responses 卡片,不带 /chat/completions),API Key 填你的 sk- 密钥,然后保存。' },
        { text: '在该供应商的模型列表中点击「Add model」。Model ID 填上方选中的模型,Context window 按所选模型填写,然后保存。' },
        { text: '返回工作区,点击输入框右下角的「Choose model」,展开稳得 AI 并选择刚添加的模型。' },
        { text: '发送一条消息进行测试。看到 ZCode 正常回复,即表示配置成功。' },
      ],
    },
    {
      key: 'cursor',
      icon: '/tutorial-logos/cursor.svg',
      name: 'Cursor',
      endpoint: 'openai',
      steps: [
        { text: '打开 Cursor Settings,进入 Network,将 HTTP Compatibility Mode 改为 HTTP/1.1。' },
        { text: '进入 Models 页面,展开 API Keys,填入你的 sk- 密钥。' },
        { text: '打开 Override OpenAI Base URL,填入本站接入地址(右侧 OpenAI 兼容卡片)。' },
        { text: '手动添加模型名,保存设置后返回对话页面选择该模型;记得点击 Verify 验证,能正常回复即配置完成。' },
      ],
    },
    {
      key: 'trae',
      icon: '/tutorial-logos/trae.svg',
      name: 'TRAE SOLO',
      endpoint: 'openai',
      steps: [
        { text: '打开 TRAE SOLO 的模型设置,点击「添加模型」,展开「自定义配置」。' },
        { text: 'API 格式选择「OpenAI 兼容」。自定义请求地址填本站接入地址,保持「完整 URL」关闭,不要在地址末尾添加斜杠(系统会自动补全 /chat/completions 路径)。' },
        { text: '模型 ID 填上方选中的模型,API 密钥填你的 sk- 密钥。模型支持图片时,可以开启「多模态」。' },
        { text: '提交保存后回到 TRAE SOLO 对话界面,选择刚添加的模型,发送一条消息测试,能正常回复即表示配置完成。' },
      ],
    },
    {
      key: 'api',
      icon: '/tutorial-logos/api.svg',
      name: '代码调用',
      endpoint: 'openai',
      intro: '直接调用 OpenAI 兼容接口,任选一种语言,把 model 换成上方选中的模型。',
      steps: [],
    },
    {
      key: 'read-frog',
      icon: '/tutorial-logos/read-frog.svg',
      name: 'Read Frog',
      endpoint: 'anthropic',
      steps: [
        { text: '打开 Read Frog 设置页面,在左侧 API Providers 列表中选择 Anthropic 提供商。' },
        { text: '按照界面填写 API Key、Base URL(本站接入地址,右侧 Anthropic 兼容卡片)和 Model(勾选自定义模型名称,填上方选中的模型)。' },
        { text: '点击「Test Connection」验证连接,提示成功即配置完成。' },
      ],
    },
    {
      key: 'kiss-translator',
      icon: '/tutorial-logos/kiss-translator.svg',
      name: 'Kiss Translator',
      endpoint: 'anthropic',
      steps: [
        { text: '打开 Kiss Translator 设置页面,点击「Add」添加翻译服务,选择「Claude」。' },
        { text: '按照界面填写 URL、Key 和 Model。注意 URL 需要填写完整地址(含 /v1/messages,右侧 Anthropic 兼容卡片加 /v1/messages),Key 填 sk- 密钥,Model 填上方选中的模型。' },
        { text: '保存配置后,刷新页面即可使用翻译功能。' },
      ],
    },
    {
      key: 'immersive-translate',
      icon: '/tutorial-logos/immersive-translate.svg',
      name: '沉浸式翻译',
      endpoint: 'openai',
      steps: [
        { text: '打开沉浸式翻译设置页面,进入「AI 服务」选择「OpenAI」。' },
        { text: '自定义 API 接口地址填完整路径(右侧 OpenAI 兼容卡片的地址),勾选「输入自定义模型名称」手动填入上方选中的模型,APIKEY 填你的 sk- 密钥。' },
        { text: '保存配置后,打开任意英文网页即可看到翻译效果。' },
      ],
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
