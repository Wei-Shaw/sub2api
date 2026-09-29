export default {
  tutorial: {
    title: 'Tutorial',
    description: 'API integration guide and frequently asked questions',
    tabs: {
      guide: 'Setup Guide',
      faq: 'FAQ',
    },

    guide: {
      steps: [
        {
          title: 'Create an API key',
          desc: 'Create one on the "API Keys" page after signing in; pick a group (it decides routing and billing rate)',
        },
        {
          title: 'Point your client at this site',
          desc: 'Set the client base_url to our endpoint and fill in your sk- API key',
        },
        {
          title: 'Check channels and lines',
          desc: 'Confirm model availability on the "Available Channels" page and speed-test lines when needed',
        },
      ],
      tabCurl: 'curl',
      tabPython: 'Python',
      tabSdk: 'OpenAI SDK',
      copy: 'Copy',
      copied: 'Copied',
      compatibilityNote:
        'Works with any tool that supports custom OpenAI endpoints: ChatGPT-Next-Web, LobeChat, Cursor, Cherry Studio and more',
      clientsTitle: 'Common client setups',
      clientsDesc: '$BASE below is this site\'s endpoint, copyable from the "API Keys" page',
      clients: [
        {
          key: 'openai',
          name: 'OpenAI-compatible clients',
          desc: 'ChatGPT-Next-Web, LobeChat, Cherry Studio, etc.',
          code: 'Base URL: $BASE/v1\nAPI Key: sk-***',
        },
        {
          key: 'claude-code',
          name: 'Claude Code',
          desc: 'Connect via the Anthropic-compatible endpoint (/v1/messages)',
          code: 'export ANTHROPIC_BASE_URL="$BASE"\nexport ANTHROPIC_AUTH_TOKEN="sk-***"',
        },
        {
          key: 'gemini-cli',
          name: 'Gemini CLI',
          desc: 'Connect via the Gemini-compatible endpoint (/v1beta)',
          code: 'export GEMINI_API_BASE="$BASE/v1beta"\nexport GEMINI_API_KEY="sk-***"',
        },
      ],
      ccsTip:
        'Tip: click "Import to CCS" on the "API Keys" page to import the key into cc-switch for Claude Code / Gemini CLI.',
    },

    faq: {
      searchPlaceholder: 'Search questions, e.g. rate limit, 429',
      categories: {
        all: 'All',
        billing: 'Billing',
        plans: 'Plans & Quota',
        quality: 'Lines & Quality',
        support: 'Support',
      },
      resultCount: '{count} questions',
      noResult: 'No matching questions. Try another keyword.',
      items: [
        {
          id: 'billing-how',
          category: 'billing',
          question: 'How does billing work? When is my balance charged?',
          summary: 'Pay-as-you-go: each request is charged by model price × group rate once it completes.',
          answer:
            'Billing is usage-based: after each request completes, the fee (model price × group rate) is deducted from your balance in real time. Input and output tokens are priced separately and cache reads/writes follow channel rules; every charge is itemized on the "Usage" page.',
          points: [
            'Group rate decides the final cost: $1 of official usage at rate 1.5 costs $1.5',
            'Requests are rejected when the balance runs out; top up to restore access',
            'Every charge can be verified on the "Usage" page by time and model',
          ],
          note: 'Pay for what you use; balance is deducted in real time with full details on the Usage page.',
        },
        {
          id: 'group-multiplier',
          category: 'billing',
          question: 'What is a group rate? Why does the same model cost differently?',
          summary: 'Different groups map to different upstream lines and billing rates; the group picked at key creation sets the price.',
          answer:
            'A group is the unit combining routing and billing: the same model may come from different upstream lines in different groups, with different rates. Choosing a group when creating a key chooses both the line and the price; supported models per group are listed on the "Available Channels" page.',
          points: [
            'The rate scales the official model price; lower is cheaper',
            'Some groups are exclusive and only visible to specific users',
            'Peak-hour rates may apply on some lines, as shown on the subscription page',
          ],
          note: 'Group = line + price; see the "Available Channels" page.',
        },
        {
          id: 'request-logs',
          category: 'billing',
          question: 'What does ↑2k(1.8k) in usage records mean?',
          summary: 'The up arrow is input tokens (parentheses: cache reads); the down arrow is output tokens (parentheses: cache writes).',
          answer:
            'Token stats in usage records read as "↑input (cache read) ↓output (cache write)": the up arrow is input tokens with cache-read hits in parentheses; the down arrow is output tokens with cache writes in parentheses. Cache hits are usually priced lower, so more hits mean lower cost.',
          points: [],
          note: 'Parenthesized numbers are cache read/write volumes; higher hit rates cost less.',
        },
        {
          id: 'plan-vs-balance',
          category: 'plans',
          question: 'Should I top up balance or subscribe to a plan?',
          summary: 'Balance is more flexible for light usage; plans are cheaper for steady usage but may limit models or endpoints.',
          answer:
            'Balance is pay-as-you-go and usually covers all models and endpoints; subscription plans offer bundled pricing that is cheaper but may restrict models or endpoints. Pick by your usage pattern — you can hold both at the same time.',
          points: [
            'Occasional or changing model needs → balance first',
            'Daily usage with fixed models → plans are cheaper',
            'Plan quota and limits follow the subscription page details',
          ],
          note: 'Light usage → balance; daily usage → plan.',
        },
        {
          id: 'missing-model',
          category: 'plans',
          question: 'Why does it say a model does not exist or is unavailable?',
          summary: 'Model availability is decided by the key\'s group; different groups connect to different upstream lines.',
          answer:
            'A "model not found"-style error usually means the key\'s group does not include that model. Check the group-model mapping on the "Available Channels" page and, if needed, create a key in the matching group.',
          points: [
            'Confirm the model-group mapping on the "Available Channels" page first',
            'Then verify which group the requesting key belongs to',
            'Model names must match the list exactly (case and version suffix matter)',
          ],
          note: 'Check "Available Channels" first, then the key\'s group.',
        },
        {
          id: 'redeem-code',
          category: 'plans',
          question: 'Where do I redeem a code? What if the credit does not arrive?',
          summary: 'Redeem on the "Redeem" page; if credit is missing, check records first and then contact support.',
          answer:
            'Sign in and enter the code on the "Redeem" page. If it says success but the balance does not change, wait a moment and refresh; if it is still missing, keep the code and contact support.',
          points: [
            'Codes are case-sensitive and usually single-use',
            'Credit arrives in real time, occasionally with a few seconds delay',
            'For redemption issues, provide the code and time to support',
          ],
          note: 'Redeem on the "Redeem" page; refresh first, then contact support.',
        },
        {
          id: 'fidelity',
          category: 'quality',
          question: 'Are models original? Any degradation or context truncation?',
          summary: 'Requests are forwarded as-is to upstream official endpoints; no model substitution or content rewriting.',
          answer:
            'This site is built as a long-term stable relay: requests are forwarded with original parameters, without degradation, model substitution, or context truncation. If quality seems off, check line health on the "Channel Status" page and switch to a healthier group.',
          points: [
            'Session stickiness: one conversation routes to the same upstream to keep context coherent',
            'On line failures the system fails over across upstream accounts automatically',
            'For quality-sensitive workloads prefer stable high-priority groups',
          ],
          note: 'No rewriting of requests or responses; see "Channel Status" for line health.',
        },
        {
          id: 'errors-429',
          category: 'quality',
          question: 'What should I do about 429 / 5xx errors?',
          summary: '429 usually means rate limiting and 5xx transient upstream faults; the system retries and fails over — switch groups if it persists.',
          answer:
            '429 means a limit was hit (key concurrency cap or upstream quota); 5xx is usually a transient upstream fault. The system fails over across upstream accounts automatically. If errors persist, lower concurrency, retry later, or switch to a key in another group.',
          points: [
            'Each key has a concurrency cap, viewable on the "API Keys" page',
            'During peak hours switch to healthier lines shown on "Channel Status"',
            'Frequent 429s: split load across multiple keys',
          ],
          note: 'Check "Channel Status" first, then lower concurrency or switch groups.',
        },
        {
          id: 'base-url',
          category: 'quality',
          question: 'Which endpoint should each client use?',
          summary: 'OpenAI-compatible: $BASE/v1; Claude Code: $BASE (/v1/messages); Gemini CLI: $BASE/v1beta.',
          answer:
            'The site exposes OpenAI, Anthropic, and Gemini compatible endpoints on the same domain; see the "Setup Guide" tab for per-client snippets.',
          points: [
            'OpenAI-compatible: $BASE/v1 (chat/completions)',
            'Claude Code: ANTHROPIC_BASE_URL=$BASE (/v1/messages)',
            'Gemini CLI: $BASE/v1beta',
          ],
          note: 'All three endpoints share the same sk- keys.',
        },
        {
          id: 'refund',
          category: 'support',
          question: 'Can I get a refund if I am not satisfied?',
          summary: 'Balance can be refunded via the original channel under the rules; plans are generally non-refundable once activated.',
          answer:
            'Unused balance can be refunded via the original payment channel by contacting support; a handling fee may apply. Subscription plans are virtual services that take effect immediately and are non-refundable except for force majeure. The site terms and support confirmation prevail.',
          points: [],
          note: 'Balance can be refunded via the original channel; plans are non-refundable.',
        },
        {
          id: 'contact',
          category: 'support',
          question: 'How do I get support?',
          summary: 'Contact info is on the "Profile" page after signing in; service updates are posted under "Announcements".',
          answer:
            'The "Profile" page shows the support contact for account and billing questions; service changes and incident notices are posted under "Announcements"; charges can be self-checked on the "Usage" page.',
          points: [
            'For account/billing issues, include the key name, time, and request ID',
            'For line failures, attach a "Channel Status" screenshot for faster triage',
            'Important changes and maintenance notices are published under "Announcements"',
          ],
          note: 'Support contact on the "Profile" page; updates under "Announcements".',
        },
      ],
    },
  },
}
