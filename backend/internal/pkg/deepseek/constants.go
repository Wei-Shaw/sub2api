package deepseek

const (
	DeepSeekHost             = "chat.deepseek.com"
	DeepSeekBaseURL          = "https://chat.deepseek.com"
	DeepSeekCreateSessionURL = DeepSeekBaseURL + "/api/v0/chat_session/create"
	DeepSeekCreatePowURL     = DeepSeekBaseURL + "/api/v0/chat/create_pow_challenge"
	DeepSeekCompletionURL    = DeepSeekBaseURL + "/api/v0/chat/completion"
	DeepSeekDeleteSessionURL = DeepSeekBaseURL + "/api/v0/chat_session/delete"

	CompletionTargetPath = "/api/v0/chat/completion"
)

var defaultHeaders = map[string]string{
	"Host":                  DeepSeekHost,
	"Accept":                "*/*",
	"Accept-Language":       "zh-CN,zh;q=0.9,en-US;q=0.8,en;q=0.7",
	"Content-Type":          "application/json",
	"accept-charset":        "UTF-8",
	"x-app-version":         "2.0.0",
	"x-client-bundle-id":    "com.deepseek.chat",
	"x-client-locale":       "zh_CN",
	"x-client-platform":     "web",
	"x-client-version":      "2.0.0",
	"x-client-timezone-offset": "28800",
	"sec-ch-ua":            "\"Google Chrome\";v=\"149\", \"Chromium\";v=\"149\", \"Not)A;Brand\";v=\"24\"",
	"sec-ch-ua-mobile":     "?0",
	"sec-ch-ua-platform":   "\"Linux\"",
	"sec-fetch-dest":       "empty",
	"sec-fetch-mode":       "cors",
	"sec-fetch-site":       "same-origin",
	"User-Agent":           "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/149.0.0.0 Safari/537.36",
}
