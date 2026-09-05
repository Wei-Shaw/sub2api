package service

import (
	"context"
	"strings"
)

// Email alias normalization for registration dedup.
//
// Problem: registration duplicate checks compare the email verbatim (lowercase
// + trim only), so a single real inbox can spawn unlimited accounts using
// provider alias features:
//   - Plus addressing: user+tag@gmail.com is delivered to user@gmail.com
//     (supported by Gmail, Outlook/Hotmail, Yahoo, iCloud, Fastmail, and more).
//   - Gmail dot trick: u.s.e.r@gmail.com is delivered to user@gmail.com.
//   - FQDN root dot: user@gmail.com. is the absolute form of user@gmail.com,
//     passes the registration validator, and reaches the same mailbox.
//
// This lets abusers bulk-register accounts (e.g. to farm signup grants) while
// the domain whitelist and email verification see each variant as a distinct,
// deliverable address. NormalizeEmailForAliasDedup collapses these variants to
// a single "inbox identity" so the registration path can reject duplicates.
//
// Normalization rules:
//   - All domains: lowercase, trim, drop the FQDN root dot, and strip the
//     local-part "+suffix". Stripping the plus suffix on domains that do not
//     support plus addressing is harmless — those exact addresses are virtually
//     never registered.
//   - Gmail family (gmail.com / googlemail.com): additionally remove dots from
//     the local part and fold the domain to gmail.com.
//
// This only affects registration duplicate detection. It intentionally does not
// change how emails are stored, displayed, or used for login/delivery.

var gmailFamilyDomains = map[string]struct{}{
	"gmail.com":      {},
	"googlemail.com": {},
}

// NormalizeEmailForAliasDedup returns the canonical "inbox identity" of an
// email. Malformed input is returned lowercased/trimmed unchanged; format
// validation is the caller's responsibility.
func NormalizeEmailForAliasDedup(email string) string {
	local, domain, ok := splitEmailForAliasDedup(email)
	if !ok {
		return strings.ToLower(strings.TrimSpace(email))
	}
	local = stripEmailPlusSuffix(local)
	if isGmailFamilyDomain(domain) {
		local = stripEmailLocalDots(local)
		domain = "gmail.com"
	}
	return local + "@" + domain
}

// EmailAliasProbe describes a shape a stored duplicate can have, expressed on the
// dot-stripped email form used by UserRepository.ExistsByEmailAlias: Local is the
// plus-stripped and dot-stripped local part, Domain the dot-stripped candidate
// domain. Dots are removed on both sides of that comparison so one probe per
// domain also covers the Gmail dot trick and the FQDN root dot. The over-matching
// this introduces (dots stay significant outside the Gmail family) is filtered by
// re-checking every candidate with NormalizeEmailForAliasDedup.
type EmailAliasProbe struct {
	Local  string
	Domain string
}

// EmailAliasDedupProbes returns the probes covering every stored address that
// could resolve to the same inbox as email: gmail-family domains are mutual
// aliases, every other domain only collides with itself. It returns nil when
// there is nothing to probe (malformed address, or a local part made of dots
// only, which no provider delivers).
func EmailAliasDedupProbes(email string) []EmailAliasProbe {
	local, domain, ok := splitEmailForAliasDedup(email)
	if !ok {
		return nil
	}
	probeLocal := strings.ReplaceAll(stripEmailPlusSuffix(local), ".", "")
	if probeLocal == "" {
		return nil
	}
	domains := []string{domain}
	if isGmailFamilyDomain(domain) {
		domains = []string{"gmail.com", "googlemail.com"}
	}
	probes := make([]EmailAliasProbe, 0, len(domains))
	for _, candidate := range domains {
		probes = append(probes, EmailAliasProbe{
			Local:  probeLocal,
			Domain: strings.ReplaceAll(candidate, ".", ""),
		})
	}
	return probes
}

func splitEmailForAliasDedup(email string) (local string, domain string, ok bool) {
	local, domain, ok = splitEmailForPolicy(email)
	if !ok {
		return "", "", false
	}
	domain = strings.TrimRight(domain, ".")
	if domain == "" {
		return "", "", false
	}
	return local, domain, true
}

func stripEmailPlusSuffix(local string) string {
	// idx > 0 only: "+tag@host" has no local part left to keep, and folding every
	// "+x@host" into "@host" would lock unrelated senders out of that domain.
	if idx := strings.IndexByte(local, '+'); idx > 0 {
		return local[:idx]
	}
	return local
}

func stripEmailLocalDots(local string) string {
	if stripped := strings.ReplaceAll(local, ".", ""); stripped != "" {
		return stripped
	}
	return local
}

func isGmailFamilyDomain(domain string) bool {
	_, ok := gmailFamilyDomains[domain]
	return ok
}

// existsByEmailOrAlias reports whether an email — or any alias variant that
// resolves to the same inbox — is already registered.
//
// It first performs the exact ExistsByEmail check, then, only on a miss, probes
// for an alias collision. Consistent with ExistsByEmail, lookup errors are
// surfaced (fail-closed) so the registration path returns a service error instead
// of letting an attacker bypass the check by inducing errors.
func (s *AuthService) existsByEmailOrAlias(ctx context.Context, email string) (bool, error) {
	exists, err := s.userRepo.ExistsByEmail(ctx, email)
	if err != nil || exists {
		return exists, err
	}
	return s.userRepo.ExistsByEmailAlias(ctx, email)
}
