package service

import (
	"fmt"
	"strings"
	"unicode/utf8"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// AccountImportNaming applies only to newly created accounts, in import order.
// It is independent of bulk renaming so imports never rename existing accounts.
type AccountImportNaming struct {
	Prefix      string `json:"prefix"`
	Suffix      string `json:"suffix"`
	StartNumber int64  `json:"start_number"`
	Padding     int    `json:"padding"`
}

const maxAccountImportSequence int64 = 9007199254740991

func (n *AccountImportNaming) Name(index int64) (string, error) {
	if n == nil {
		return "", nil
	}
	if n.StartNumber < 0 || n.StartNumber > maxAccountImportSequence || index < 0 || index > maxAccountImportSequence-n.StartNumber || n.Padding < 1 || n.Padding > 16 {
		return "", infraerrors.BadRequest("IMPORT_NAME_INVALID", "invalid import name sequence: use a nonnegative safe integer and padding from 1 to 16")
	}
	name := n.Prefix + fmt.Sprintf("%0*d", n.Padding, n.StartNumber+index) + n.Suffix
	if !utf8.ValidString(name) || strings.TrimSpace(name) == "" || utf8.RuneCountInString(name) > 100 || strings.ContainsRune(name, '\x00') {
		return "", infraerrors.BadRequest("IMPORT_NAME_INVALID", "imported account names must contain 1 to 100 valid characters and no NUL")
	}
	return name, nil
}

// AccountImportPoolID leaves unsupported accounts outside pools during mixed imports.
func AccountImportPoolID(poolID *int64, platform, accountType string) *int64 {
	if platform == PlatformOpenAI && accountType == AccountTypeOAuth {
		return poolID
	}
	return nil
}
