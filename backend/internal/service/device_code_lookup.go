package service

import (
	"context"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/userdevice"
)

// LookupDeviceCodesByUserIDs returns a map of userID -> device_code for the given user IDs.
// It picks the primary device (most recently logged in) for each user.
// This is a shared helper used by multiple services/handlers.
func LookupDeviceCodesByUserIDs(ctx context.Context, client *dbent.Client, userIDs []int64) map[int64]string {
	if len(userIDs) == 0 {
		return nil
	}
	devices, err := client.UserDevice.Query().
		Where(userdevice.UserIDIn(userIDs...)).
		Order(dbent.Desc(userdevice.FieldLastLoginAt), dbent.Desc(userdevice.FieldCreatedAt)).
		All(ctx)
	if err != nil || len(devices) == 0 {
		return nil
	}
	result := make(map[int64]string, len(userIDs))
	for _, d := range devices {
		if _, exists := result[d.UserID]; exists {
			continue
		}
		if d.DeviceCode != nil && *d.DeviceCode != "" {
			result[d.UserID] = *d.DeviceCode
		}
	}
	return result
}
