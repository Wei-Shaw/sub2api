package dto

import (
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

type CustomDomain struct {
	ID                   int64              `json:"id"`
	UserID               int64              `json:"user_id"`
	AllUsers             bool               `json:"all_users"`
	UserIDs              []int64            `json:"user_ids"`
	Domain               string             `json:"domain"`
	Status               string             `json:"status"`
	VerificationTXTName  string             `json:"verification_txt_name"`
	VerificationTXTValue string             `json:"verification_txt_value"`
	CNAMETarget          *string            `json:"cname_target,omitempty"`
	LastError            *string            `json:"last_error,omitempty"`
	VerifiedAt           *time.Time         `json:"verified_at,omitempty"`
	LastCheckedAt        *time.Time         `json:"last_checked_at,omitempty"`
	DisabledAt           *time.Time         `json:"disabled_at,omitempty"`
	DisabledReason       *string            `json:"disabled_reason,omitempty"`
	CreatedAt            time.Time          `json:"created_at"`
	UpdatedAt            time.Time          `json:"updated_at"`
	User                 *CustomDomainUser  `json:"user,omitempty"`
	Users                []CustomDomainUser `json:"users,omitempty"`
	CanManage            bool               `json:"can_manage"`
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
		AllUsers:             domain.AllUsers,
		UserIDs:              append([]int64(nil), domain.UserIDs...),
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
		CanManage:            domain.CanManage,
	}
	if domain.User != nil {
		out.User = &CustomDomainUser{
			ID:       domain.User.ID,
			Email:    domain.User.Email,
			Username: domain.User.Username,
			Role:     domain.User.Role,
		}
	}
	if len(domain.Users) > 0 {
		out.Users = make([]CustomDomainUser, 0, len(domain.Users))
		for i := range domain.Users {
			out.Users = append(out.Users, CustomDomainUser{
				ID:       domain.Users[i].ID,
				Email:    domain.Users[i].Email,
				Username: domain.Users[i].Username,
				Role:     domain.Users[i].Role,
			})
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

func CustomDomainForUserFromService(domain *service.CustomDomain) *CustomDomain {
	out := CustomDomainFromService(domain)
	if out == nil || out.CanManage {
		return out
	}
	out.UserID = 0
	out.AllUsers = false
	out.UserIDs = nil
	out.VerificationTXTName = ""
	out.VerificationTXTValue = ""
	out.User = nil
	out.Users = nil
	return out
}

func CustomDomainsForUserFromService(domains []service.CustomDomain) []CustomDomain {
	out := make([]CustomDomain, 0, len(domains))
	for i := range domains {
		if converted := CustomDomainForUserFromService(&domains[i]); converted != nil {
			out = append(out, *converted)
		}
	}
	return out
}
