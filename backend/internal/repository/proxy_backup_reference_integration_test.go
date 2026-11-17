//go:build integration

package repository

import "github.com/Wei-Shaw/sub2api/internal/service"

func (s *ProxyExpirySuite) TestBackupReferencesAreDirectedAndShareable() {
	backupID := s.mkProxy("shared-backup", service.FallbackModeNone, nil, nil)
	firstID := s.mkProxy("first-primary", service.FallbackModeProxy, nil, &backupID)
	backup, err := s.repo.GetByID(s.ctx, backupID)
	s.Require().NoError(err)
	s.Require().Nil(backup.BackupProxyID, "assigning a backup must not overwrite its own configuration")

	secondID := s.mkProxy("second-primary", service.FallbackModeProxy, nil, &backupID)
	// Updating the shared backup must not clear its incoming references.
	backup.Name = "renamed-shared-backup"
	s.Require().NoError(s.repo.Update(s.ctx, backup))
	for _, id := range []int64{firstID, secondID} {
		primary, err := s.repo.GetByID(s.ctx, id)
		s.Require().NoError(err)
		s.Require().Equal(&backupID, primary.BackupProxyID)
	}
	backup, err = s.repo.GetByID(s.ctx, backupID)
	s.Require().NoError(err)
	s.Require().Nil(backup.BackupProxyID)
}

func (s *ProxyExpirySuite) TestBackupUpdateAndClearLeaveOtherReferencesIntact() {
	backupID := s.mkProxy("old-backup", service.FallbackModeNone, nil, nil)
	replacementID := s.mkProxy("new-backup", service.FallbackModeNone, nil, nil)
	firstID := s.mkProxy("primary-to-change", service.FallbackModeProxy, nil, &backupID)
	secondID := s.mkProxy("primary-to-keep", service.FallbackModeProxy, nil, &backupID)
	first, err := s.repo.GetByID(s.ctx, firstID)
	s.Require().NoError(err)
	first.BackupProxyID = &replacementID
	s.Require().NoError(s.repo.Update(s.ctx, first))
	first, err = s.repo.GetByID(s.ctx, firstID)
	s.Require().NoError(err)
	s.Require().Equal(&replacementID, first.BackupProxyID)
	first.BackupProxyID = nil
	first.FallbackMode = service.FallbackModeNone
	s.Require().NoError(s.repo.Update(s.ctx, first))
	first, err = s.repo.GetByID(s.ctx, firstID)
	s.Require().NoError(err)
	s.Require().Nil(first.BackupProxyID)
	second, err := s.repo.GetByID(s.ctx, secondID)
	s.Require().NoError(err)
	s.Require().Equal(&backupID, second.BackupProxyID)
	for _, id := range []int64{backupID, replacementID} {
		backup, err := s.repo.GetByID(s.ctx, id)
		s.Require().NoError(err)
		s.Require().Nil(backup.BackupProxyID)
	}
}

func (s *ProxyExpirySuite) TestBackupReferencesSupportDirectedChains() {
	lastID := s.mkProxy("chain-last", service.FallbackModeNone, nil, nil)
	middleID := s.mkProxy("chain-middle", service.FallbackModeProxy, nil, &lastID)
	firstID := s.mkProxy("chain-first", service.FallbackModeProxy, nil, &middleID)
	for _, pair := range [][2]int64{{firstID, middleID}, {middleID, lastID}} {
		primary, err := s.repo.GetByID(s.ctx, pair[0])
		s.Require().NoError(err)
		s.Require().Equal(&pair[1], primary.BackupProxyID)
	}
	last, err := s.repo.GetByID(s.ctx, lastID)
	s.Require().NoError(err)
	s.Require().Nil(last.BackupProxyID)
}
