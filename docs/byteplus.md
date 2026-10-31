# BytePlus ModelArk accounts

Select **BytePlus** next to Kimi, Zhipu GLM, DeepSeek and MiniMax when creating
an account. The account and its groups use the `byteplus` platform and the
existing OpenAI-compatible text gateway.

1. Create an API key in the appropriate ModelArk region and activate the models
   you intend to use. Keys, model activation and model availability are regional.
2. Select BytePlus and enter the API key. The default is Johor/AP:
   `https://ark.ap-southeast.bytepluses.com/api/v3`. The EU preset is
   `https://ark.eu-west.bytepluses.com/api/v3`; confirm your models are available
   there before changing regions.
3. Select **Seed and text models**, **Seedance video**, or both. These endpoint
   restrictions are saved with the account and preserved by scheduler metadata.
   Automatic upstream billing-rate probing defaults to off for BytePlus.
4. Create a BytePlus group, bind the account, and configure its allowed models,
   pricing and spending limits. Enable **Allow Seedance video** in the group for native video tasks. Composite groups can explicitly target BytePlus.
5. Use Chat Completions for text, or the native Ark task API for Seedance:
   `POST /api/v3/contents/generations/tasks`, then
   `GET /api/v3/contents/generations/tasks/{task_id}` with the same client key.
   Video status/delete requests stay bound to their authenticated task owner,
   including through composite groups. Completion-token billing is deduplicated
   across repeated successful polls.

BytePlus resource packs, promotional coupons and provider token limits are
managed in the BytePlus console. Gateway limits do not configure provider-side
limits. Only pay-as-you-go/resource-pack mode is offered; this integration does
not invent a BytePlus Coding Plan or an unsupported balance-probe endpoint.

Existing OpenAI-platform accounts configured with an Ark base URL and explicit
Seedance capability remain supported. They are not automatically migrated or
reassigned to BytePlus groups.

References:

- [Regional availability](https://console.byteplus.com/ark/region%3Aap-southeast-1/docs/region-availability)
- [Chat API](https://docs.byteplus.com/en/docs/ModelArk/chat-api)
- [Native video task API](https://console.byteplus.com/ark/region%3Aap-southeast-1/docs/create-video-generation-task-api)
