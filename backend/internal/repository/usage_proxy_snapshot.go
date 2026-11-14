package repository

import (
	"database/sql"
	"encoding/json"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func usageProxySnapshotJSON(snapshot *service.UsageProxySnapshot) any {
	if snapshot == nil {
		return nil
	}
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		return nil
	}
	return string(encoded)
}

func decodeUsageProxySnapshot(value sql.NullString) *service.UsageProxySnapshot {
	if !value.Valid {
		return nil
	}
	var snapshot *service.UsageProxySnapshot
	if json.Unmarshal([]byte(value.String), &snapshot) != nil {
		return nil
	}
	return snapshot
}
