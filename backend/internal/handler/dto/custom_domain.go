package dto

import (
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

type CustomDomain struct {
	ID                   int64             `json:"id"`
	UserID               int64             `json:"user_id"`
	Domain               string            `json:"domain"`
	Status               string            `json:"status"`
	VerificationTXTName  string            `json:"verification_txt_name"`
	VerificationTXTValue string            `json:"verification_txt_value"`
	CNAMETarget          *string           `json:"cname_target,omitempty"`
	LastError            *string           `json:"last_error,omitempty"`
	VerifiedAt           *time.Time        `json:"verified_at,omitempty"`
	LastCheckedAt        *time.Time        `json:"last_checked_at,omitempty"`
	DisabledAt           *time.Time        `json:"disabled_at,omitempty"`
	DisabledReason       *string           `json:"disabled_reason,omitempty"`
	CreatedAt            time.Time         `json:"created_at"`
	UpdatedAt            time.Time         `json:"updated_at"`
	User                 *CustomDomainUser `json:"user,omitempty"`
}

type CustomDomainUser struct {
	ID       int64  `json:"id"`
	Email    string `json:"email"`
	Username string `json:"username,omitempty"`
	Role     string `json:"role,omitempty"`
}

type CustomDomainConfig struct {
	Enabled     bool   `json:"enabled"`
	CNAMETarget string `json:"cname_target"`
}

func CustomDomainFromService(domain *service.CustomDomain) *CustomDomain {
	if domain == nil {
		return nil
	}
	out := &CustomDomain{
		ID:                   domain.ID,
		UserID:               domain.UserID,
		Domain:               domain.Domain,
		Status:               domain.Status,
		VerificationTXTName:  domain.VerificationTXTName,
		VerificationTXTValue: domain.VerificationTXTValue,
		CNAMETarget:          domain.CNAMETarget,
		LastError:            domain.LastError,
		VerifiedAt:           domain.VerifiedAt,
		LastCheckedAt:        domain.LastCheckedAt,
		DisabledAt:           domain.DisabledAt,
		DisabledReason:       domain.DisabledReason,
		CreatedAt:            domain.CreatedAt,
		UpdatedAt:            domain.UpdatedAt,
	}
	if domain.User != nil {
		out.User = &CustomDomainUser{
			ID:       domain.User.ID,
			Email:    domain.User.Email,
			Username: domain.User.Username,
			Role:     domain.User.Role,
		}
	}
	return out
}

func CustomDomainsFromService(domains []service.CustomDomain) []CustomDomain {
	out := make([]CustomDomain, 0, len(domains))
	for i := range domains {
		if converted := CustomDomainFromService(&domains[i]); converted != nil {
			out = append(out, *converted)
		}
	}
	return out
}
