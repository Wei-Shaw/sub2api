package service

import "time"

type ApiKey struct {
	ID        int64
	UserID    int64
	Key       string
	Name      string
	GroupID   *int64
	Status    string
	CreatedAt time.Time
	UpdatedAt time.Time
	User      *User
	Group     *Group
REDACTED

func (k *ApiKey) IsActive() bool {
	return k.Status == StatusActive
REDACTED
