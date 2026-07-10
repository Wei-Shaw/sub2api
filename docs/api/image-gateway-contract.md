# Image Gateway Contract

> 鐘舵€侊細鍐呴儴鍙敤 / 寰呭鏍? 
> 閫傜敤鏂癸細QCanvas / TapCanvas / 鏈潵鍐呴儴鎺ュ叆鏂? 
> 鏈€鍚庢洿鏂帮細2026-07-05  
> 鍏宠仈浠诲姟涔︼細`docs/superpowers/codex-handoff/CODEX_TASK_API_CONTRACT.md`銆乣docs/superpowers/codex-handoff/CODEX_TASK_BILLING.md`

鏈枃妗ｅ浐鍖?Sub2API 鍥剧墖鐢熸垚/缂栬緫閫氶亾濂戠害锛岄噸鐐硅鐩?Gemini 3.1 Flash Image Preview锛屼篃灏辨槸浠诲姟涔︿腑鐨?Nano Banana 2銆?
## 1. 绔偣

| 鍦烘櫙 | Method | Path | Auth | 璇存槑 |
|------|--------|------|------|------|
| Claude messages 鍏煎璺緞 | POST | `/v1/messages` | `Authorization: Bearer <api-key>` | 鎺ュ叆鏂圭敤 Claude 鏍煎紡鍙?Gemini 鍥剧墖鐢熸垚/缂栬緫璇锋眰銆?|
| Gemini 鍘熺敓璺緞 | POST | `/v1beta/models/{model}:generateContent` | `Authorization: Bearer <api-key>` | Gemini SDK/CLI 鐩磋繛锛屽師鐢?`generateContent` 閫忎紶銆?|
| OpenAI Images 璺緞 | POST | `/v1/images/generations` | `Authorization: Bearer <api-key>` | OpenAI 骞冲彴缁勪娇鐢紝闈炴湰 NB2 濂戠害閲嶇偣銆?|
| OpenAI Images Edit 璺緞 | POST | `/v1/images/edits` | `Authorization: Bearer <api-key>` | OpenAI 骞冲彴缁勪娇鐢紝闈炴湰 NB2 濂戠害閲嶇偣銆?|

`/v1/messages` 浼氭寜 API key 鎵€灞炲垎缁勫钩鍙拌矾鐢便€侴emini 鍥剧墖鑳藉姏瑕佹眰鍒嗙粍鍏佽鍥剧墖鐢熸垚锛屼笖妯″瀷/璇锋眰琚瘑鍒负鍥剧墖鎰忓浘銆?
## 2. `/v1/messages` 璇锋眰 Schema

鍏煎 Claude messages 涓讳綋锛屽苟鍏佽鍥剧墖鐢熸垚閰嶇疆浠庨《灞傛垨 `generationConfig` 閫忎紶鍒?Gemini锛?
| 瀛楁 | 绫诲瀷 | 蹇呭～ | 璇存槑 |
|------|------|------|------|
| `model` | string | 鏄?| 鎺ㄨ崘 `gemini-3.1-flash-image-preview`銆?|
| `max_tokens` | number | 鏄?| Claude 鍏煎瀛楁銆?|
| `messages` | array | 鏄?| Claude messages 鏁扮粍銆?|
| `imageConfig` | object | 鍚?| 鍏煎椤跺眰鍐欐硶锛屼細杞负 Gemini `generationConfig.imageConfig`銆?|
| `generationConfig.imageConfig` | object | 鍚?| 鍘熺敓鍐欐硶锛岄€忎紶鍒?Gemini銆?|
| `responseModalities` | array | 鍚?| 鍏煎椤跺眰鍐欐硶锛屼細杞负 Gemini `generationConfig.responseModalities`銆傚浘鐗囩敓鎴愬簲浼?`["TEXT", "IMAGE"]`銆?|
| `generationConfig.responseModalities` | array | 鍚?| 鍘熺敓鍐欐硶锛岄€忎紶鍒?Gemini銆?|
| `stream` | bool | 鍚?| streaming 涓?non-streaming 閮戒細鎸夊搷搴?`inlineData` 缁熻鍥剧墖寮犳暟銆?|

### imageConfig

| 瀛楁 | 绫诲瀷 | 蹇呭～ | 璇存槑 |
|------|------|------|------|
| `aspectRatio` | string | 鍚?| 鏀寔 `1:1`銆乣3:2`銆乣2:3`銆乣3:4`銆乣4:3`銆乣4:5`銆乣5:4`銆乣9:16`銆乣16:9`銆乣21:9`銆乣1:4`銆乣4:1`銆乣1:8`銆乣8:1`銆乣9:21`銆?|
| `imageSize` | string | 鍚?| Gemini 瀹樻柟鍊间负 `"512"` / `"1K"` / `"2K"` / `"4K"`銆傚缓璁皟鐢ㄦ柟鎸夎繖涓ぇ灏忓啓浼犮€?|

璁¤垂褰掍竴鍖栵細

- `"512"`銆乣"512px"`銆乣"0.5K"` 浼氬綊涓€涓?`0.5K`銆?- `"1K"` / `"1024px"` 褰掍竴涓?`1K`銆?- `"2K"` / `"2048px"` 褰掍竴涓?`2K`銆?- `"4K"` / `"4096px"` 褰掍竴涓?`4K`銆?- 鏈煡鍊奸粯璁ゆ寜 `2K` 璁¤垂骞惰褰?warning銆?
### 鍥剧墖杈撳叆

Claude content block 鏀寔锛?
```json
{
  "type": "image",
  "source": {
    "type": "base64",
    "media_type": "image/png",
    "data": "<base64>"
  }
}
```

涔熸敮鎸?URL source锛屽苟杞负 Gemini `fileData`锛?
```json
{
  "type": "image",
  "source": {
    "type": "url",
    "url": "https://example.invalid/reference.png",
    "media_type": "image/png"
  }
}
```

URL source 蹇呴』閫氳繃 SSRF/allowlist 鏍￠獙銆侴emini 鍗曟鏈€澶?14 寮犲弬鑰冨浘锛屾帴鍏ユ柟搴斿湪鍓嶇鍏堝仛鏁伴噺闄愬埗銆?
媒体 URL allowlist 统一优先读取 `SUB2API_MEDIA_URL_ALLOWLIST`；未配置时兼容 fallback 到旧 `SUB2API_VIDEO_URL_ALLOWLIST`。图片和视频共用这一组素材域名控制。

## 3. `/v1/messages` 璇锋眰绀轰緥

```json
{
  "model": "gemini-3.1-flash-image-preview",
  "max_tokens": 8192,
  "imageConfig": {
    "aspectRatio": "1:1",
    "imageSize": "512"
  },
  "responseModalities": ["TEXT", "IMAGE"],
  "messages": [
    {
      "role": "user",
      "content": [
        {"type": "text", "text": "Generate a clean product concept image."},
        {
          "type": "image",
          "source": {
            "type": "url",
            "url": "https://example.invalid/reference.png",
            "media_type": "image/png"
          }
        }
      ]
    }
  ]
}
```

Sub2API 杞粰 Gemini 鐨勫叧閿粨鏋勶細

```json
{
  "generationConfig": {
    "imageConfig": {
      "aspectRatio": "1:1",
      "imageSize": "512"
    },
    "responseModalities": ["TEXT", "IMAGE"]
  },
  "contents": [
    {
      "role": "user",
      "parts": [
        {"text": "Generate a clean product concept image."},
        {
          "fileData": {
            "fileUri": "https://example.invalid/reference.png",
            "mimeType": "image/png"
          }
        }
      ]
    }
  ]
}
```

## 4. Gemini 鍘熺敓璺緞

`POST /v1beta/models/{model}:generateContent`

鎺ュ叆鏂瑰彲鐩存帴浼?Gemini 鍘熺敓 body銆係ub2API 鍘熺敓璺緞淇濈暀 `thoughtSignature`锛屽苟鍙湪璐﹀彿鍒囨崲/缁戝畾缂哄け绛夐渶瑕侀噸璇曟仮澶嶆椂娓呯悊鏃х鍚嶃€?
```json
{
  "contents": [
    {
      "role": "user",
      "parts": [
        {"text": "Generate an image."}
      ]
    }
  ],
  "generationConfig": {
    "responseModalities": ["TEXT", "IMAGE"],
    "imageConfig": {
      "aspectRatio": "16:9",
      "imageSize": "1K"
    }
  }
}
```

澶氳疆缂栬緫娉ㄦ剰锛?
- 鍝嶅簲閲岀殑 `thoughtSignature` 鏄?Gemini 浼氳瘽杩炵画鎬х殑涓€閮ㄥ垎銆?- 瀹㈡埛绔笅涓€杞簲鍘熸牱甯﹀洖鐩稿叧 signed parts銆?- Sub2API 鐨勭┖ part 杩囨护涓嶄細鍒犻櫎鍥剧墖/鎬濊€冪鍚?part銆?
## 5. 鍝嶅簲涓庡浘鐗囪鏁?
Gemini 鍥剧墖杈撳嚭浠ュ搷搴斾腑鐨?`inlineData` 鍥剧墖 part 涓哄噯銆係ub2API 璁¤垂涓嶄細鍥哄畾鍐欐 1 寮狅紝鑰屾槸缁熻瀹為檯杩斿洖鐨勫浘鐗?part 鏁帮細

- non-streaming锛氱粺璁℃渶缁?JSON body 涓殑 `inlineData` 鍥剧墖 part銆?- streaming锛氭寜娴佸紡鍝嶅簲鑱氬悎缁熻 `inlineData` 鍥剧墖 part銆?- native 涓?`/v1/messages` 鍏煎璺緞閮戒娇鐢ㄥ悓涓€璁℃暟璇箟銆?
鎺ュ叆鏂规嬁鍒板搷搴斿悗锛屽簲鎸変笂娓稿崗璁В鏋?`content` / `parts` / `inlineData`锛屼笉瑕佸亣璁炬瘡娆″彧杩斿洖涓€寮犲浘銆?
## 6. 閿欒鐮佽〃

| HTTP | type / reason | 瑙﹀彂鏉′欢 |
|------|---------------|----------|
| 400 | `invalid_request_error` 鎴?`VALIDATION_ERROR` | JSON 鏃犳晥銆佸瓧娈电被鍨嬩笉鍖归厤銆佷笂娓告嫆缁濊姹傘€?|
| 400 | URL 鏍￠獙閿欒 | URL source 鏈€氳繃 SSRF/allowlist 鏍￠獙銆?|
| 403 | `permission_error` | 鍒嗙粍涓嶅厑璁稿浘鐗囩敓鎴愭垨妯″瀷鑼冨洿涓嶅厑璁搞€?|
| 404 | `not_found_error` | 闈?OpenAI 骞冲彴璋冪敤 `/v1/images/*`锛屾垨涓嶆敮鎸佺殑 endpoint銆?|
| 429 | `rate_limit_error` | 鍥剧墖骞跺彂妲戒綅鑰楀敖鎴栦笂娓搁檺娴併€?|
| 502/503 | upstream error | 涓婃父涓嶅彲鐢ㄣ€佽秴鏃舵垨鍝嶅簲瑙ｆ瀽澶辫触銆?|

## 7. 璁¤垂璇存槑

Nano Banana 2 妯″瀷锛歚gemini-3.1-flash-image-preview`銆?
| imageSize | 褰掍竴妗ｄ綅 | 鏍囧噯姣忓浘浠锋牸 |
|-----------|----------|--------------|
| `512` | `0.5K` | `$0.045` |
| `1K` | `1K` | `$0.067` |
| `2K` | `2K` | `$0.101` |
| `4K` | `4K` | `$0.151` |

鍏朵粬璁¤垂瑙勫垯锛?
- 鏂囨湰/鍥剧墖杈撳叆锛歚$0.50 / 1M tokens`銆?- 鏂囨湰杈撳嚭锛歚$3.00 / 1M tokens`銆?- 鍥剧墖杈撳嚭锛歚$60.00 / 1M image tokens`锛屼唬鐮佷晶鎸変笂琛ㄦ瘡鍥句环钀借处銆?- `usage_logs.media_type` 鍦ㄥ浘鐗囦换鍔′腑鍐欎负 `image`銆?- `usage_logs.image_count` 涓哄疄闄呰緭鍑哄浘鐗囨暟銆?- `total_cost` 涓烘ā鍨嬪師濮嬫垚鏈紱`actual_cost` 浼氫箻鐢ㄦ埛/鍒嗙粍 rate銆俙rate=1` 鏃朵袱鑰呯浉鍚屻€?
## 8. 鎺ュ叆鏂规敞鎰?
- 鍥剧墖灏哄閫夋嫨鍣ㄥ缓璁彧缁?`512`銆乣1K`銆乣2K`銆乣4K` 鍥涙。锛岄伩鍏嶆湭鐭ュ€兼寜 `2K` 鍏滃簳璁¤垂銆?- 澶氬浘杈撳嚭瑕佹寜瀹為檯 `inlineData` 鏁板睍绀轰笌涓嬭浇銆?- 澶氳疆缂栬緫蹇呴』淇濆瓨骞跺洖浼?`thoughtSignature`锛屽惁鍒欎笂娓稿彲鑳芥嫆缁濆悗缁姹傘€?- URL 鍥剧墖杈撳叆浼氳蛋 SSRF/allowlist 鏍￠獙锛涚敓浜ф帴鍏ュ墠闇€瑕佺‘璁ょ礌鏉?CDN 鍩熷悕宸叉斁琛屻€?
