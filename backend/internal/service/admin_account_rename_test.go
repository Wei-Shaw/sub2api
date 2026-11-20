package service

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type renameRepoStub struct {
	names   []AccountName
	applied []AccountRenameItem
	err     error
}

func (r *renameRepoStub) ListAccountNamesForRename(context.Context, []int64, string, string) ([]AccountName, error) {
	return r.names, r.err
}
func (r *renameRepoStub) ApplyAccountRenames(_ context.Context, items []AccountRenameItem) error {
	r.applied = items
	return r.err
}

func TestAccountRenamePreviewStableSelectionAndContinuation(t *testing.T) {
	repo := &renameRepoStub{names: []AccountName{{ID: 9, Name: "second@example.test"}, {ID: 2, Name: "first@example.test"}, {ID: 11, Name: "GPT-Free-07"}, {ID: 12, Name: "GPT-Free-11"}, {ID: 15, Name: "GPT-Free-not-a-number"}}}
	svc := &adminServiceImpl{accountNameRepo: repo}
	input := &BulkAccountRenameInput{AccountIDs: []int64{9, 2}, Prefix: "GPT-Free-", StartNumber: 1, Padding: 2, ContinueSequence: true}
	got, err := svc.PreviewAccountRenames(context.Background(), input)
	require.NoError(t, err)
	require.Equal(t, []AccountRenameItem{{AccountID: 2, OldName: "first@example.test", NewName: "GPT-Free-12"}, {AccountID: 9, OldName: "second@example.test", NewName: "GPT-Free-13"}}, got.Items)
	require.Equal(t, []int64{9, 2}, input.AccountIDs)
	require.Empty(t, repo.applied, "preview must not write")
}

func TestAccountRenameValidationAndConflicts(t *testing.T) {
	tests := []struct {
		name  string
		input BulkAccountRenameInput
		rows  []AccountName
		want  error
	}{
		{"duplicate IDs", BulkAccountRenameInput{AccountIDs: []int64{1, 1}, Padding: 2}, nil, ErrAccountRenameInvalid},
		{"missing", BulkAccountRenameInput{AccountIDs: []int64{1}, Padding: 2}, nil, ErrAccountNotFound},
		{"collision", BulkAccountRenameInput{AccountIDs: []int64{1}, Prefix: "A", StartNumber: 1, Padding: 2}, []AccountName{{1, "old"}, {2, "A01"}}, ErrAccountRenameConflict},
		{"negative", BulkAccountRenameInput{AccountIDs: []int64{1}, StartNumber: -1, Padding: 2}, nil, ErrAccountRenameInvalid},
		{"bad padding", BulkAccountRenameInput{AccountIDs: []int64{1}, Padding: 17}, nil, ErrAccountRenameInvalid},
		{"overflow", BulkAccountRenameInput{AccountIDs: []int64{1, 2}, StartNumber: maxAccountRenameNumber, Padding: 2}, []AccountName{{1, "a"}, {2, "b"}}, ErrAccountRenameInvalid},
		{"too long", BulkAccountRenameInput{AccountIDs: []int64{1}, Prefix: strings.Repeat("号", 99), Padding: 2}, []AccountName{{1, "a"}}, ErrAccountRenameInvalid},
		{"null", BulkAccountRenameInput{AccountIDs: []int64{1}, Prefix: "x\x00", Padding: 2}, nil, ErrAccountRenameInvalid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &adminServiceImpl{accountNameRepo: &renameRepoStub{names: tt.rows}}
			_, err := svc.PreviewAccountRenames(context.Background(), &tt.input)
			require.ErrorIs(t, err, tt.want)
		})
	}
	svc := &adminServiceImpl{accountNameRepo: &renameRepoStub{names: []AccountName{{1, "old"}}}}
	got, err := svc.PreviewAccountRenames(context.Background(), &BulkAccountRenameInput{AccountIDs: []int64{1}, Prefix: strings.Repeat("号", 98), StartNumber: 1, Padding: 2})
	require.NoError(t, err)
	require.Equal(t, strings.Repeat("号", 98)+"01", got.Items[0].NewName)
}

func TestAccountRenameApplyValidatesWholePlanBeforeWriting(t *testing.T) {
	repo := &renameRepoStub{}
	svc := &adminServiceImpl{accountNameRepo: repo}
	for _, items := range [][]AccountRenameItem{nil, {{1, "old", ""}}, {{1, "a", "new"}, {2, "b", "new"}}, {{1, "a", "A"}, {1, "a", "B"}}} {
		_, err := svc.ApplyAccountRenames(context.Background(), items)
		require.ErrorIs(t, err, ErrAccountRenameInvalid)
		require.Empty(t, repo.applied)
	}
	input := []AccountRenameItem{{9, "b", "B"}, {2, "a", "A"}}
	got, err := svc.ApplyAccountRenames(context.Background(), input)
	require.NoError(t, err)
	require.Equal(t, 2, got.Applied)
	require.Equal(t, int64(2), repo.applied[0].AccountID)
	require.Equal(t, int64(9), input[0].AccountID)
}
