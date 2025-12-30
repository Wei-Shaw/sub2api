package service

import (
	"time"

	"golang.org/x/crypto/bcrypt"
)

type User struct {
	ID            int64
	Email         string
	Username      string
	Wechat        string
	Notes         string
	PasswordHash  string
	Role          string
	Balance       float64
	Concurrency   int
	Status        string
	AllowedGroups []int64
	TokenVersion  int64 // Incremented on password change to invalidate existing tokens
	CreatedAt     time.Time
	UpdatedAt     time.Time

	ApiKeys       []ApiKey
	Subscriptions []UserSubscription
REDACTED

func (u *User) IsAdmin() bool {
	return u.Role == RoleAdmin
REDACTED

func (u *User) IsActive() bool {
	return u.Status == StatusActive
REDACTED

// CanBindGroup checks whether a user can bind to a given group.
// For standard groups:
// - If AllowedGroups is non-empty, only allow binding to IDs in that list.
// - If AllowedGroups is empty (nil or length 0), allow binding to any non-exclusive group.
func (u *User) CanBindGroup(groupID int64, isExclusive bool) bool {
	if len(u.AllowedGroups) > 0 {
		for _, id := range u.AllowedGroups {
			if id == groupID {
				return true
		REDACTED
	REDACTED
		return false
REDACTED
	return !isExclusive
REDACTED

func (u *User) SetPassword(password string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
REDACTED
	u.PasswordHash = string(hash)
	return nil
REDACTED

func (u *User) CheckPassword(password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)) == nil
REDACTED
