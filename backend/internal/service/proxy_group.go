package service

import "time"

const (
	ProxyGroupStatusActive   = "active"
	ProxyGroupStatusInactive = "inactive"
)

type ProxyGroup struct {
	ID                   int64     `json:"id"`
	Name                 string    `json:"name"`
	Description          *string   `json:"description,omitempty"`
	Status               string    `json:"status"`
	ProxyIDs             []int64   `json:"proxy_ids"`
	MemberCount          int       `json:"member_count"`
	AvailableMemberCount int       `json:"available_member_count"`
	AccountCount         int64     `json:"account_count"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}

type ProxyGroupInput struct {
	Name        string
	Description *string
	Status      string
	ProxyIDs    []int64
}
