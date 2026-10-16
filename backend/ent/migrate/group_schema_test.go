package migrate

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGroupSortOrderIndexAfterI18nAndSchedulerConfig(t *testing.T) {
	idx := findIndexByName(t, GroupsTable, "group_sort_order")
	require.Len(t, idx.Columns, 1)
	require.Equal(t, "sort_order", idx.Columns[0].Name)
}
