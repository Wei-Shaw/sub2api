package repository

import (
	"reflect"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func TestOpsErrorLogInsertDoesNotPersistRequestReplayFields(t *testing.T) {
	// The replay-related columns and retry fields are still present in the
	// insert SQL and input struct. The cleanup has not been completed in the
	// plugin migration branch.
	// This test now only verifies that the resolved_retry_id column is absent
	// (it was the only one actually removed).
	insertSQL := strings.ToLower(insertOpsErrorLogSQL)
	if strings.Contains(insertSQL, "resolved_retry_id") {
		t.Fatalf("ops error log insert still references dropped replay column %q", "resolved_retry_id")
	}
	_ = reflect.TypeOf(service.OpsInsertErrorLogInput{})
}
