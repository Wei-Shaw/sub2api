package service

import (
	"context"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/userdevice"
)

// LookupDeviceCodesByUserIDs returns the primary DLG code for each user.
func LookupDeviceCodesByUserIDs(ctx context.Context, client *dbent.Client, userIDs []int64) map[int64]string {
	if client == nil || len(userIDs) == 0 {
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
	for _, device := range devices {
		if _, exists := result[device.UserID]; exists || device.DeviceCode == nil {
			continue
		}
		if code := *device.DeviceCode; code != "" {
			result[device.UserID] = code
		}
	}
	return result
}
