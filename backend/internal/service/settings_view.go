package service

type SystemSettings struct {
	RegistrationEnabled bool
	EmailVerifyEnabled  bool

	SmtpHost     string
	SmtpPort     int
	SmtpUsername string
	SmtpPassword string
	SmtpFrom     string
	SmtpFromName string
	SmtpUseTLS   bool

	TurnstileEnabled   bool
	TurnstileSiteKey   string
	TurnstileSecretKey string

	SiteName     string
	SiteLogo     string
	SiteSubtitle string
	ApiBaseUrl   string
	ContactInfo  string
	DocUrl       string

	DefaultConcurrency int
	DefaultBalance     float64

	SimpleMode bool // 简单模式
REDACTED

type PublicSettings struct {
	RegistrationEnabled bool
	EmailVerifyEnabled  bool
	TurnstileEnabled    bool
	TurnstileSiteKey    string
	SiteName            string
	SiteLogo            string
	SiteSubtitle        string
	ApiBaseUrl          string
	ContactInfo         string
	DocUrl              string
	Version             string
	SimpleMode          bool // 简单模式
REDACTED
