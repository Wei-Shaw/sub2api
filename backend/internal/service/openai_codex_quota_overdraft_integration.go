package service

func (s *OpenAIGatewayService) SetCodexQuotaOverdraftCoordinator(coordinator *CodexQuotaOverdraftCoordinator) {
	if s != nil {
		s.codexQuotaOverdraft = coordinator
	}
}

func (s *AccountUsageService) SetCodexQuotaOverdraftCoordinator(coordinator *CodexQuotaOverdraftCoordinator) {
	if s != nil {
		s.codexQuotaOverdraft = coordinator
	}
}

// codexQuotaOverdraftCoordinator returns the single coordinator owned by the
// OpenAI gateway. Keeping construction here avoids adding a fork-only provider
// to the generated Wire graph.
func (s *OpenAIGatewayService) codexQuotaOverdraftCoordinator(
	tlsFPProfileService *TLSFingerprintProfileService,
) *CodexQuotaOverdraftCoordinator {
	if s == nil {
		return nil
	}
	s.codexQuotaOverdraftOnce.Do(func() {
		if s.codexQuotaOverdraft != nil {
			return
		}
		var tempUnschedCache TempUnschedCache
		if s.rateLimitService != nil {
			tempUnschedCache = s.rateLimitService.tempUnschedCache
		}
		s.codexQuotaOverdraft = NewCodexQuotaOverdraftCoordinator(
			s.accountRepo,
			s.httpUpstream,
			s.openAITokenProvider,
			tlsFPProfileService,
			s.cfg,
			tempUnschedCache,
			s,
			s.rateLimitService,
		)
	})
	return s.codexQuotaOverdraft
}
