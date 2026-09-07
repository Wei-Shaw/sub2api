package kiro

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// profileArnPattern 是 AWS 自己给出的 profileArn 校验规则——真实账号测试时
// 直接从 AWS 的 400 ValidationException 里原样抄出来的（不是猜的）：
//
//	arn:[-.a-z0-9]{1,63}:(codewhisperer|transform):[-.a-z0-9]{1,63}:\d{12}:profile/([a-zA-Z0-9]){12}
var profileArnPattern = regexp.MustCompile(`^arn:[-.a-z0-9]{1,63}:(codewhisperer|transform):[-.a-z0-9]{1,63}:\d{12}:profile/[a-zA-Z0-9]{12}$`)

// IsValidProfileArn 判断一个字符串是否是合法形态的 Kiro profile ARN。
func IsValidProfileArn(arn string) bool {
	return profileArnPattern.MatchString(strings.TrimSpace(arn))
}

// ListProfilesHostFor 返回 ListAvailableProfiles 该打去哪个 host。
//
// CodeWhisperer 的 REST host 只在 us-east-1 存在；其它区域用区域化的
// Amazon Q host（q.<region>.amazonaws.com）提供同一个操作——参考实现
// Kiro-Go（proxy/kiro_api.go 的 regionalizeURLForRegion）证实了这一点，
// 不是我们自己猜的映射关系。
func ListProfilesHostFor(region string) string {
	region = strings.ToLower(strings.TrimSpace(region))
	if region == "" || region == "us-east-1" {
		return "https://codewhisperer.us-east-1.amazonaws.com"
	}
	return fmt.Sprintf("https://q.%s.amazonaws.com", region)
}

// BuildListProfilesURL 拼出完整的 ListAvailableProfiles 地址。
func BuildListProfilesURL(region string) string {
	return ListProfilesHostFor(region) + "/ListAvailableProfiles"
}

// BuildListProfilesRequestBody 构造分页请求体。
//
// 不带 maxResults 字段——真实账号测试证实带了这个字段（不管是 JSON 数字
// 还是字符串）会被 AWS 拒成 400 REQUEST_BODY_INVALID/"Improperly formed
// request."，参考实现 Kiro-Go 的 map[string]interface{}{"maxResults":
// pageSize} 这个细节在我们的真实账号上是错的（或者 AWS 之后收紧了校验）。
// 只有 nextToken 时才带这个字段（分页第二页起），首页请求体是空对象 {}。
func BuildListProfilesRequestBody(nextToken string) ([]byte, error) {
	body := map[string]any{}
	if nextToken != "" {
		body["nextToken"] = nextToken
	}
	return json.Marshal(body)
}

// KiroProfile 是 ListAvailableProfiles 返回的一条可选 profile。
type KiroProfile struct {
	ARN  string
	Name string
}

// ListProfilesResponse 是 ListAvailableProfiles 一页的解析结果。
type ListProfilesResponse struct {
	Profiles  []KiroProfile
	NextToken string
}

// ParseListProfilesResponse 解析 ListAvailableProfiles 响应。
//
// 格式不对的单条 profile（ARN 不满足 profileArnPattern）直接丢弃，不让
// 一条脏数据打断整批发现——上游返回什么由 AWS 决定，本地只做防御性过滤。
func ParseListProfilesResponse(raw []byte) (*ListProfilesResponse, error) {
	var r struct {
		Profiles []struct {
			ARN  string `json:"arn"`
			Name string `json:"profileName"`
		} `json:"profiles"`
		NextToken string `json:"nextToken"`
	}
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, fmt.Errorf("kiro: decode ListAvailableProfiles response: %w", err)
	}

	out := &ListProfilesResponse{NextToken: strings.TrimSpace(r.NextToken)}
	for _, p := range r.Profiles {
		arn := strings.TrimSpace(p.ARN)
		if !IsValidProfileArn(arn) {
			continue
		}
		out.Profiles = append(out.Profiles, KiroProfile{ARN: arn, Name: strings.TrimSpace(p.Name)})
	}
	return out, nil
}
