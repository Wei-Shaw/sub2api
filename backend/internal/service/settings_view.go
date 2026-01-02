package service

type SystemSettings struct {
	RegistrationEnabled bool
	EmailVerifyEnabled  bool

	SMTPHost     string
	SMTPPort     int
	SMTPUsername string
	SMTPPassword string
	SMTPFrom     string
	SMTPFromName string
	SMTPUseTLS   bool

	TurnstileEnabled   bool
	TurnstileSiteKey   string
	TurnstileSecretKey string

	SiteName     string
	SiteLogo     string
	SiteSubtitle string
	APIBaseURL   string
	ContactInfo  string
	DocURL       string

	DefaultConcurrency int
	DefaultBalance     float64
REDACTED

type PublicSettings struct {
	RegistrationEnabled bool
	EmailVerifyEnabled  bool
	TurnstileEnabled    bool
	TurnstileSiteKey    string
	SiteName            string
	SiteLogo            string
	SiteSubtitle        string
	APIBaseURL          string
	ContactInfo         string
	DocURL              string
	Version             string
REDACTED
