export default {
  tutorial: {
    title: '使用教程',
    description: 'API 接入指南与常见问题解答',
    tabs: {
      guide: '配置指南',
      faq: '常见问题',
    },

    guide: {
      steps: [
        {
          title: '创建 API 密钥',
          desc: '登录后在「API 密钥」页一键创建，创建时选择分组（决定线路与计费倍率）',
        },
        {
          title: '替换客户端接入地址',
          desc: '将客户端的 base_url 指向本站接入地址，密钥填入 sk- 开头的 API Key',
        },
        {
          title: '确认渠道与线路',
          desc: '在「可用渠道」页确认模型支持情况，必要时测速选择状态最优的线路',
        },
      ],
      tabCurl: 'curl',
      tabPython: 'Python',
      tabSdk: 'OpenAI SDK',
      copy: '复制',
      copied: '已复制',
      compatibilityNote:
        '兼容所有支持自定义 OpenAI 接入的工具：ChatGPT-Next-Web、LobeChat、Cursor、Cherry Studio 等',
      clientsTitle: '常用客户端配置',
      clientsDesc: '以下 $BASE 均为本站接入地址，可在「API 密钥」页复制',
      clients: [
        {
          key: 'openai',
          name: 'OpenAI 兼容客户端',
          desc: 'ChatGPT-Next-Web、LobeChat、Cherry Studio 等',
          code: 'Base URL: $BASE/v1\nAPI Key: sk-***',
        },
        {
          key: 'claude-code',
          name: 'Claude Code',
          desc: '通过环境变量接入 Anthropic 兼容端点（/v1/messages）',
          code: 'export ANTHROPIC_BASE_URL="$BASE"\nexport ANTHROPIC_AUTH_TOKEN="sk-***"',
        },
        {
          key: 'gemini-cli',
          name: 'Gemini CLI',
          desc: '通过环境变量接入 Gemini 兼容端点（/v1beta）',
          code: 'export GEMINI_API_BASE="$BASE/v1beta"\nexport GEMINI_API_KEY="sk-***"',
        },
      ],
      ccsTip:
        '提示：在「API 密钥」页点击「导入到 CCS」，可将密钥一键导入 Claude Code / Gemini CLI 的 cc-switch 配置。',
    },

    faq: {
      searchPlaceholder: '搜索问题关键词，如：倍率、429',
      categories: {
        all: '全部',
        billing: '计费与扣费',
        plans: '套餐与额度',
        quality: '线路与质量',
        support: '售后与支持',
      },
      resultCount: '共 {count} 个问题',
      noResult: '没有找到相关问题，换个关键词试试',
      items: [
        {
          id: 'billing-how',
          category: 'billing',
          question: '计费是怎么算的？什么时候扣余额？',
          summary: '按实际用量计费，请求完成后按模型单价 × 分组倍率从余额实时扣除。',
          answer:
            '站点采用按量计费：每次请求完成后，系统按「模型单价 × 分组倍率」折算费用并从账户余额实时扣除。输入、输出 token 分别计价，缓存读写按渠道规则计价，每笔扣费均可在「使用记录」中查看明细。',
          points: [
            '分组倍率决定最终扣费：消耗 $1 官方额度，倍率 1.5 则扣 $1.5',
            '余额不足时请求会被拒绝，及时充值即可恢复',
            '明细可在「使用记录」页按时间、模型逐笔核查',
          ],
          note: '用多少扣多少，余额实时扣减，明细见使用记录。',
        },
        {
          id: 'group-multiplier',
          category: 'billing',
          question: '什么是分组倍率？为什么同一个模型价格不同？',
          summary: '不同分组对应不同的上游线路与计费倍率，创建密钥时选择的分组即最终价格。',
          answer:
            '分组是线路与计费的组合单位：同一模型在不同分组中可能来自不同上游线路，倍率也不同。创建密钥时选择分组，即同时选择了线路与价格；各分组支持的模型可在「可用渠道」页查看。',
          points: [
            '倍率是对模型官方价格的放大/缩小系数，倍率越低越划算',
            '部分分组为专属分组，仅对特定用户可见可用',
            '高峰时段部分线路可能启用高峰倍率，以订阅页展示为准',
          ],
          note: '选分组 = 选线路 + 选价格，以「可用渠道」页为准。',
        },
        {
          id: 'request-logs',
          category: 'billing',
          question: '使用记录里的 ↑2k(1.8k) 是什么意思？',
          summary: '向上箭头是输入 token（括号为缓存读取），向下箭头是输出 token（括号为缓存写入）。',
          answer:
            '使用记录中的 token 统计格式为「↑输入（缓存读取）↓输出（缓存写入）」：向上箭头代表输入 token，旁边括号内是缓存命中读取的部分；向下箭头代表输出 token，括号内是缓存写入的部分。缓存命中部分通常计价更低，命中越多越省。',
          points: [],
          note: '括号里的数字是缓存读写量，缓存命中越多费用越低。',
        },
        {
          id: 'plan-vs-balance',
          category: 'plans',
          question: '充值余额和订阅套餐怎么选？',
          summary: '用量少充余额更灵活；用量稳定买套餐价格更优，但可能有模型或接入范围限制。',
          answer:
            '余额按量实时扣费，通常覆盖全部模型和接入点；订阅套餐以更优惠的价格提供打包服务，但部分套餐有模型或接入范围限制。按自己的用量模式选择即可，两者也可以同时持有。',
          points: [
            '偶尔用、模型需求多变 → 优先余额',
            '每天都在用、模型固定 → 套餐更划算',
            '套餐的具体额度与限制以订阅页说明为准',
          ],
          note: '用量少 → 余额；天天用 → 套餐。',
        },
        {
          id: 'missing-model',
          category: 'plans',
          question: '为什么提示模型不存在或不可用？',
          summary: '模型可用性由密钥所属分组决定，不同分组接入的上游线路不同。',
          answer:
            '出现「模型不存在」或类似报错时，通常是当前密钥所属分组未接入该模型。可在「可用渠道」页查看各分组支持的模型列表，必要时创建一个对应分组的密钥再试。',
          points: [
            '先在「可用渠道」页确认模型与分组的对应关系',
            '再检查发起请求的密钥是否属于该分组',
            '模型名称需与列表完全一致（区分大小写与版本后缀）',
          ],
          note: '先查「可用渠道」页，再核对密钥的分组。',
        },
        {
          id: 'redeem-code',
          category: 'plans',
          question: '兑换码在哪里使用？充值后没到账怎么办？',
          summary: '在「兑换」页输入兑换码即可充值；未到账先查订单与到账记录，仍有问题联系客服。',
          answer:
            '登录后进入「兑换」页输入兑换码完成充值。如果显示兑换成功但余额未变化，可稍等片刻后刷新页面；仍未到账请保留兑换码并联系客服核查。',
          points: [
            '兑换码区分大小写，且通常只能使用一次',
            '兑换成功后余额实时到账，个别情况延迟数秒',
            '兑换遇到问题请提供兑换码与时间联系客服',
          ],
          note: '「兑换」页输入即充，未到账先刷新再找客服。',
        },
        {
          id: 'fidelity',
          category: 'quality',
          question: '模型是原版吗？会不会降智或截断上下文？',
          summary: '请求按原参数转发到上游官方接口，不替换模型、不改写内容。',
          answer:
            '站点定位是长期稳定的中转服务：请求按原参数转发到对应上游接口，不降智、不替换模型、不截断上下文。如遇到质量异常，可在「渠道状态」页查看线路健康度，并切换到状态更好的分组。',
          points: [
            '会话保持：同一对话自动路由到相同上游，保证上下文连贯',
            '线路异常时系统自动在多个上游账号间故障转移',
            '对质量敏感的场景建议选择状态稳定的高优先级分组',
          ],
          note: '不改写请求与响应，线路健康度见「渠道状态」页。',
        },
        {
          id: 'errors-429',
          category: 'quality',
          question: '遇到 429 / 5xx 报错怎么办？',
          summary: '429 多为触发限流，5xx 多为上游瞬时故障；系统会自动重试切换，持续报错可换分组。',
          answer:
            '429 表示触发限流（密钥并发上限或上游配额），5xx 多为上游瞬时故障。系统会自动在多个上游账号间故障转移；如果持续出现，可降低请求并发、稍后重试，或切换到其他分组的密钥。',
          points: [
            '每个密钥有并发上限，可在「API 密钥」页查看当前并发',
            '高峰期可切换到「渠道状态」页中更健康的线路',
            '频繁 429 建议拆分多个密钥分摊并发',
          ],
          note: '先看「渠道状态」页，再降并发或换分组。',
        },
        {
          id: 'base-url',
          category: 'quality',
          question: '不同客户端的接入地址分别是什么？',
          summary: 'OpenAI 兼容用 $BASE/v1，Claude Code 用 $BASE（/v1/messages），Gemini CLI 用 $BASE/v1beta。',
          answer:
            '站点同时提供 OpenAI、Anthropic、Gemini 三种兼容端点，接入地址均为本站域名，具体路径见「配置指南」页的常用客户端配置。',
          points: [
            'OpenAI 兼容：$BASE/v1（chat/completions）',
            'Claude Code：ANTHROPIC_BASE_URL=$BASE（/v1/messages）',
            'Gemini CLI：$BASE/v1beta',
          ],
          note: '三种端点共用同一批 sk- 密钥。',
        },
        {
          id: 'refund',
          category: 'support',
          question: '使用不满意可以退款吗？',
          summary: '余额可按规则申请原路退回；套餐开通后一般不支持退款。',
          answer:
            '未使用的余额可联系客服按充值渠道申请原路退回，可能产生手续费；订阅套餐属于即时生效的虚拟服务，开通后除不可抗力因素外不支持退款。具体以站内条款与客服确认结果为准。',
          points: [],
          note: '余额可申请原路退回，套餐不支持退款。',
        },
        {
          id: 'contact',
          category: 'support',
          question: '遇到问题如何获得支持？',
          summary: '登录后在「个人资料」页查看客服联系方式，服务动态见「公告」。',
          answer:
            '登录后「个人资料」页展示客服联系方式，可直接咨询账户与计费问题；服务变更与故障通报会通过「公告」发布；扣费明细可在「使用记录」页自助核查。',
          points: [
            '账户、扣费问题：优先提供密钥名称、时间与请求 ID',
            '线路故障：附上「渠道状态」页截图更快定位',
            '重要变更与维护通知以「公告」页为准',
          ],
          note: '客服联系方式在「个人资料」页，动态看「公告」。',
        },
      ],
    },
  },
}
