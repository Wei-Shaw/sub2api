package service

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const MaxBulkRenameAccounts = 1000
const maxAccountRenameNumber = int64(9007199254740991) // Largest exact JSON number in JavaScript.

var (
	ErrAccountRenameInvalid  = infraerrors.BadRequest("ACCOUNT_RENAME_INVALID", "invalid account rename rule or selection")
	ErrAccountRenameConflict = infraerrors.New(http.StatusConflict, "ACCOUNT_RENAME_CONFLICT", "account names changed or a generated name is already in use; preview again")
)

type AccountName struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type AccountRenameItem struct {
	AccountID int64  `json:"account_id"`
	OldName   string `json:"old_name"`
	NewName   string `json:"new_name"`
}

type AccountNameRepository interface {
	// ListAccountNamesForRename returns selected rows and names sharing the literal affixes.
	// It must never load credentials or other account configuration.
	ListAccountNamesForRename(context.Context, []int64, string, string) ([]AccountName, error)
	// ApplyAccountRenames locks the selected rows and applies the entire plan atomically.
	ApplyAccountRenames(context.Context, []AccountRenameItem) error
}

type BulkAccountRenameInput struct {
	AccountIDs       []int64 `json:"account_ids"`
	Prefix           string  `json:"prefix"`
	Suffix           string  `json:"suffix"`
	StartNumber      int64   `json:"start_number"`
	Padding          int     `json:"padding"`
	ContinueSequence bool    `json:"continue_sequence"`
}

type BulkAccountRenamePreview struct {
	Items []AccountRenameItem `json:"items"`
}

type BulkAccountRenameResult struct {
	Applied int `json:"applied"`
}

func validateAccountRenameIDs(ids []int64) error {
	if len(ids) == 0 || len(ids) > MaxBulkRenameAccounts {
		return ErrAccountRenameInvalid.WithMetadata(map[string]string{"max_accounts": strconv.Itoa(MaxBulkRenameAccounts)})
	}
	seen := make(map[int64]bool, len(ids))
	for _, id := range ids {
		if id <= 0 || seen[id] {
			return ErrAccountRenameInvalid
		}
		seen[id] = true
	}
	return nil
}

func validAccountRename(name string) bool {
	return utf8.ValidString(name) && strings.TrimSpace(name) != "" && utf8.RuneCountInString(name) <= 100 && !strings.ContainsRune(name, '\x00')
}

func (s *adminServiceImpl) PreviewAccountRenames(ctx context.Context, input *BulkAccountRenameInput) (*BulkAccountRenamePreview, error) {
	if input == nil {
		return nil, ErrAccountRenameInvalid
	}
	if err := validateAccountRenameIDs(input.AccountIDs); err != nil {
		return nil, err
	}
	if input.StartNumber < 0 || input.StartNumber > maxAccountRenameNumber || input.Padding < 1 || input.Padding > 16 || !utf8.ValidString(input.Prefix+input.Suffix) || utf8.RuneCountInString(input.Prefix+input.Suffix) >= 100 || strings.ContainsRune(input.Prefix+input.Suffix, '\x00') {
		return nil, ErrAccountRenameInvalid
	}
	names, err := s.accountNameRepo.ListAccountNamesForRename(ctx, input.AccountIDs, input.Prefix, input.Suffix)
	if err != nil {
		return nil, err
	}
	return buildAccountRenamePreview(input, names)
}

func buildAccountRenamePreview(input *BulkAccountRenameInput, names []AccountName) (*BulkAccountRenamePreview, error) {
	selected := make(map[int64]bool, len(input.AccountIDs))
	for _, id := range input.AccountIDs {
		selected[id] = true
	}
	current := make(map[int64]string, len(input.AccountIDs))
	occupied := make(map[string]bool, len(names))
	start := input.StartNumber
	for _, row := range names {
		if selected[row.ID] {
			current[row.ID] = row.Name
		} else {
			occupied[row.Name] = true
		}
		if input.ContinueSequence && strings.HasPrefix(row.Name, input.Prefix) && strings.HasSuffix(row.Name, input.Suffix) && len(row.Name) >= len(input.Prefix)+len(input.Suffix) {
			number := row.Name[len(input.Prefix) : len(row.Name)-len(input.Suffix)]
			if number == "" || strings.IndexFunc(number, func(r rune) bool { return r < '0' || r > '9' }) >= 0 {
				continue
			}
			n, err := strconv.ParseInt(number, 10, 64)
			if err != nil || n >= maxAccountRenameNumber {
				return nil, ErrAccountRenameInvalid
			}
			if n >= start {
				start = n + 1
			}
		}
	}
	if len(current) != len(input.AccountIDs) {
		return nil, ErrAccountNotFound
	}
	if start > maxAccountRenameNumber-int64(len(input.AccountIDs)-1) {
		return nil, ErrAccountRenameInvalid
	}
	ids := append([]int64(nil), input.AccountIDs...)
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	result := &BulkAccountRenamePreview{Items: make([]AccountRenameItem, 0, len(ids))}
	for i, id := range ids {
		name := input.Prefix + fmt.Sprintf("%0*d", input.Padding, start+int64(i)) + input.Suffix
		if !validAccountRename(name) {
			return nil, ErrAccountRenameInvalid
		}
		if occupied[name] {
			return nil, ErrAccountRenameConflict
		}
		result.Items = append(result.Items, AccountRenameItem{AccountID: id, OldName: current[id], NewName: name})
	}
	return result, nil
}

func (s *adminServiceImpl) ApplyAccountRenames(ctx context.Context, items []AccountRenameItem) (*BulkAccountRenameResult, error) {
	ids := make([]int64, 0, len(items))
	names := make(map[string]bool, len(items))
	for _, item := range items {
		if !validAccountRename(item.NewName) || !utf8.ValidString(item.OldName) || utf8.RuneCountInString(item.OldName) > 100 || names[item.NewName] {
			return nil, ErrAccountRenameInvalid
		}
		names[item.NewName] = true
		ids = append(ids, item.AccountID)
	}
	if err := validateAccountRenameIDs(ids); err != nil {
		return nil, err
	}
	// Stable lock order prevents concurrent selections with opposite UI orders deadlocking.
	plan := append([]AccountRenameItem(nil), items...)
	sort.Slice(plan, func(i, j int) bool { return plan[i].AccountID < plan[j].AccountID })
	if err := s.accountNameRepo.ApplyAccountRenames(ctx, plan); err != nil {
		return nil, err
	}
	return &BulkAccountRenameResult{Applied: len(plan)}, nil
}
