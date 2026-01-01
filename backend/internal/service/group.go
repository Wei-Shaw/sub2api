package service

import "time"

type Group struct {
	ID             int64
	Name           string
	Description    string
	Platform       string
	RateMultiplier float64
	IsExclusive    bool
	Status         string

	SubscriptionType    string
	DailyLimitUSD       *float64
	WeeklyLimitUSD      *float64
	MonthlyLimitUSD     *float64
	DefaultValidityDays int

	CreatedAt time.Time
	UpdatedAt time.Time

	AccountGroups []AccountGroup
	AccountCount  int64
REDACTED

func (g *Group) IsActive() bool {
	return g.Status == StatusActive
REDACTED

func (g *Group) IsSubscriptionType() bool {
	return g.SubscriptionType == SubscriptionTypeSubscription
REDACTED

func (g *Group) IsFreeSubscription() bool {
	return g.IsSubscriptionType() && g.RateMultiplier == 0
REDACTED

func (g *Group) HasDailyLimit() bool {
	return g.DailyLimitUSD != nil && *g.DailyLimitUSD > 0
REDACTED

func (g *Group) HasWeeklyLimit() bool {
	return g.WeeklyLimitUSD != nil && *g.WeeklyLimitUSD > 0
REDACTED

func (g *Group) HasMonthlyLimit() bool {
	return g.MonthlyLimitUSD != nil && *g.MonthlyLimitUSD > 0
REDACTED
