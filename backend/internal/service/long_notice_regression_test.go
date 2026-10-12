//go:build unit

package service

import (
 "context"
 "testing"
 "github.com/Wei-Shaw/sub2api/internal/config"
 "github.com/stretchr/testify/require"
)

func TestLongNoticePublicAndSave(t *testing.T) {
 const notice = `{"version":1,"blocks":[{"type":"feature","number":"01"}]}`
 svc := NewSettingService(&settingPublicRepoStub{values: map[string]string{SettingKeyLongNoticeContent:notice}}, &config.Config{})
 settings, err := svc.GetPublicSettings(context.Background())
 require.NoError(t,err)
 require.Equal(t,notice,settings.LongNoticeContent)
 repo := &settingUpdateRepoStub{}
 svc = NewSettingService(repo,&config.Config{})
 require.NoError(t,svc.UpdateSettings(context.Background(),&SystemSettings{LongNoticeContent:notice}))
 require.Equal(t,notice,repo.updates[SettingKeyLongNoticeContent])
 require.NoError(t,svc.UpdateSettings(context.Background(),&SystemSettings{LongNoticeContent:""}))
 require.Equal(t,"",repo.updates[SettingKeyLongNoticeContent])
}
