package service

import "context"

const SettingKeyLoginCaptchaEnabled = "login_captcha_enabled"

func (s *SettingService) loginCaptchaEnabled(settings map[string]string) bool {
	// An explicit production security requirement cannot be disabled by a UI setting.
	if s.cfg != nil && s.cfg.Server.Mode == "release" && s.cfg.Turnstile.Required {
		return true
	}
	return settings[SettingKeyLoginCaptchaEnabled] != "false"
}

func (s *AuthService) VerifyCaptchaForLogin(ctx context.Context, proof CaptchaProof, remoteIP string) error {
	if s.settingService == nil {
		return s.VerifyCaptcha(ctx, proof, remoteIP)
	}
	settings, err := s.settingService.settingRepo.GetMultiple(ctx, []string{SettingKeyLoginCaptchaEnabled})
	if err != nil {
		return ErrServiceUnavailable
	}
	if !s.settingService.loginCaptchaEnabled(settings) {
		return nil
	}
	return s.VerifyCaptcha(ctx, proof, remoteIP)
}
