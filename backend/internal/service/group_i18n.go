package service

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

type GroupI18n = domain.GroupI18n
type GroupLocaleText = domain.GroupLocaleText

// groupI18nNameMaxRunes 与 groups.name 的长度上限一致。
const groupI18nNameMaxRunes = 100

// 语言代码只校验 BCP 47 形态（en、zh、zh-TW）；具体提供哪些语言由前端语言包决定。
var groupI18nLocalePattern = regexp.MustCompile(`^[A-Za-z]{2,8}(-[A-Za-z0-9]{1,8})*$`)

// normalizeGroupI18n 去除译文首尾空白，并丢弃名称与描述都为空的语言。
// 返回值始终非 nil，落库为 JSON 对象。
func normalizeGroupI18n(in GroupI18n) (GroupI18n, error) {
	out := make(GroupI18n, len(in))
	for locale, text := range in {
		if !groupI18nLocalePattern.MatchString(locale) {
			return nil, infraerrors.BadRequest("INVALID_GROUP_I18N", fmt.Sprintf("invalid locale %q in i18n", locale))
		}
		name := strings.TrimSpace(text.Name)
		description := strings.TrimSpace(text.Description)
		if utf8.RuneCountInString(name) > groupI18nNameMaxRunes {
			return nil, infraerrors.BadRequest("INVALID_GROUP_I18N", fmt.Sprintf("i18n name for locale %q must be at most %d characters", locale, groupI18nNameMaxRunes))
		}
		if name == "" && description == "" {
			continue
		}
		out[locale] = GroupLocaleText{Name: name, Description: description}
	}
	return out, nil
}

// cloneGroupI18nForDuplicate 复制译文中的描述。名称译文不随复制带走：
// 副本的 name 已改写，沿用源分组的名称译文会让两个分组在该语言下同名。
func cloneGroupI18nForDuplicate(in GroupI18n) GroupI18n {
	out := make(GroupI18n, len(in))
	for locale, text := range in {
		if text.Description == "" {
			continue
		}
		out[locale] = GroupLocaleText{Description: text.Description}
	}
	return out
}
