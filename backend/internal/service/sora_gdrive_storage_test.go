//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSoraGDriveStorage_StorageType(t *testing.T) {
	s := NewSoraGDriveStorage(nil)
	assert.Equal(t, SoraStorageTypeGDrive, s.StorageType())
}

func TestSoraGDriveStorage_EnabledWithNilSettingService(t *testing.T) {
	s := NewSoraGDriveStorage(nil)
	assert.False(t, s.Enabled(context.Background()))
}

func TestSoraGDriveStorage_IsHealthyWithNilReceiver(t *testing.T) {
	var s *SoraGDriveStorage
	assert.False(t, s.IsHealthy(context.Background()))
}

func TestSoraGDriveStorage_GetServiceWithoutProfile(t *testing.T) {
	s := NewSoraGDriveStorage(nil)
	_, _, err := s.getService(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no active gdrive profile")
}

func TestSoraGDriveStorage_DeleteObjectsEmpty(t *testing.T) {
	s := NewSoraGDriveStorage(nil)
	err := s.DeleteObjects(context.Background(), []string{})
	assert.NoError(t, err)
}

func TestSoraGDriveStorage_RefreshClient(t *testing.T) {
	s := NewSoraGDriveStorage(nil)
	// 不应 panic
	s.RefreshClient()
	assert.Nil(t, s.srv)
	assert.Nil(t, s.cfg)
}

func TestSoraGDriveStorage_HasValidCredentials(t *testing.T) {
	s := NewSoraGDriveStorage(nil)

	tests := []struct {
		name    string
		profile *SoraS3Profile
		want    bool
	}{
		{
			name: "oauth2 with all fields",
			profile: &SoraS3Profile{
				AuthType:     "oauth2",
				ClientID:     "id",
				ClientSecret: "secret",
				RefreshToken: "token",
			},
			want: true,
		},
		{
			name: "oauth2 missing refresh token",
			profile: &SoraS3Profile{
				AuthType:     "oauth2",
				ClientID:     "id",
				ClientSecret: "secret",
			},
			want: false,
		},
		{
			name: "service_account with json",
			profile: &SoraS3Profile{
				AuthType:           "service_account",
				ServiceAccountJSON: `{"type":"service_account"}`,
			},
			want: true,
		},
		{
			name: "service_account without json",
			profile: &SoraS3Profile{
				AuthType: "service_account",
			},
			want: false,
		},
		{
			name: "unknown auth type",
			profile: &SoraS3Profile{
				AuthType: "unknown",
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := s.hasValidCredentials(tt.profile)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestSoraStorageRouter_DefaultsToS3(t *testing.T) {
	s3 := NewSoraS3Storage(nil)
	router := NewSoraStorageRouter(nil, s3, nil)
	// settingService 为 nil，应返回 s3Storage
	backend := router.activeBackend(context.Background())
	assert.Equal(t, s3, backend)
}

func TestSoraStorageRouter_StorageType(t *testing.T) {
	router := NewSoraStorageRouter(nil, nil, nil)
	assert.Equal(t, SoraStorageTypeS3, router.StorageType())
}

func TestSoraStorageRouter_RefreshAllNoPanic(t *testing.T) {
	router := NewSoraStorageRouter(nil, nil, nil)
	// 不应 panic
	router.RefreshAll()
}
