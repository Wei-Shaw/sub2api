package dto

import (
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

type CustomDomainUser struct {
	ID       int64  `json:"id"`
	Email    string `json:"email,omitempty"`
	Username string `json:"username,omitempty"`
	Role     string `json:"role,omitempty"`
}

type CustomDomain struct {
	ID                   int64              `json:"id"`
	UserID               int64              `json:"user_id,omitempty"`
	AllUsers             bool               `json:"all_users"`
	UserIDs              []int64            `json:"user_ids,omitempty"`
	Domain               string             `json:"domain"`
	Status               string             `json:"status"`
	VerificationTXTName  string             `json:"verification_txt_name"`
	VerificationTXTValue string             `json:"verification_txt_value"`
	CNAMETarget          *string            `json:"cname_target,omitempty"`
	VerifiedAt           *time.Time         `json:"verified_at,omitempty"`
	LastCheckedAt        *time.Time         `json:"last_checked_at,omitempty"`
	LastError            *string            `json:"last_error,omitempty"`
	DisabledAt           *time.Time         `json:"disabled_at,omitempty"`
	DisabledReason       *string            `json:"disabled_reason,omitempty"`
	CreatedAt            time.Time          `json:"created_at"`
	UpdatedAt            time.Time          `json:"updated_at"`
	User                 *CustomDomainUser  `json:"user,omitempty"`
	Users                []CustomDomainUser `json:"users,omitempty"`
	CanManage            bool               `json:"can_manage,omitempty"`
}

type CustomDomainConfig struct {
	Enabled     bool   `json:"enabled"`
	CNAMETarget string `json:"cname_target"`
}

func CustomDomainFromService(domain *service.CustomDomain) *CustomDomain {
	if domain == nil {
		return nil
	}
	userIDs := append([]int64(nil), domain.AuthorizedUserIDs...)
	var owner *CustomDomainUser
	if domain.User != nil {
		owner = &CustomDomainUser{
			ID:       domain.User.ID,
			Email:    domain.User.Email,
			Username: domain.User.Username,
			Role:     domain.User.Role,
		}
	}
	users := make([]CustomDomainUser, 0, len(domain.AuthorizedUsers))
	for i := range domain.AuthorizedUsers {
		user := &domain.AuthorizedUsers[i]
		users = append(users, CustomDomainUser{
			ID:       user.ID,
			Email:    user.Email,
			Username: user.Username,
			Role:     user.Role,
		})
	}
	return &CustomDomain{
		ID:                   domain.ID,
		UserID:               domain.UserID,
		AllUsers:             domain.AllUsers,
		UserIDs:              userIDs,
		Domain:               domain.Domain,
		Status:               domain.Status,
		VerificationTXTName:  domain.VerificationTXTName,
		VerificationTXTValue: domain.VerificationTXTValue,
		CNAMETarget:          domain.CNAMETarget,
		VerifiedAt:           domain.VerifiedAt,
		LastCheckedAt:        domain.LastCheckedAt,
		LastError:            domain.LastError,
		DisabledAt:           domain.DisabledAt,
		DisabledReason:       domain.DisabledReason,
		CreatedAt:            domain.CreatedAt,
		UpdatedAt:            domain.UpdatedAt,
		User:                 owner,
		Users:                users,
		CanManage:            domain.CanManage,
	}
}

func CustomDomainsFromService(domains []service.CustomDomain) []CustomDomain {
	out := make([]CustomDomain, 0, len(domains))
	for i := range domains {
		out = append(out, *CustomDomainFromService(&domains[i]))
	}
	return out
}

func CustomDomainForUserFromService(domain *service.CustomDomain, _ int64) *CustomDomain {
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

func CustomDomainsForUserFromService(domains []service.CustomDomain, userID int64) []CustomDomain {
	out := make([]CustomDomain, 0, len(domains))
	for i := range domains {
		out = append(out, *CustomDomainForUserFromService(&domains[i], userID))
	}
	return out
}
