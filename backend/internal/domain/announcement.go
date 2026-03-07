package domain

import (
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	AnnouncementStatusDraft    = "draft"
	AnnouncementStatusActive   = "active"
	AnnouncementStatusArchived = "archived"
)

const (
	AnnouncementNotifyModeSilent = "silent"
	AnnouncementNotifyModePopup  = "popup"
)

const (
	AnnouncementConditionTypeSubscription = "subscription"
	AnnouncementConditionTypeBalance      = "balance"
)

const (
	AnnouncementOperatorIn  = "in"
	AnnouncementOperatorGT  = "gt"
	AnnouncementOperatorGTE = "gte"
	AnnouncementOperatorLT  = "lt"
	AnnouncementOperatorLTE = "lte"
	AnnouncementOperatorEQ  = "eq"
)

var (
	ErrAnnouncementNotFound      = infraerrors.NotFound("ANNOUNCEMENT_NOT_FOUND", "announcement not found")
	ErrAnnouncementInvalidTarget = infraerrors.BadRequest("ANNOUNCEMENT_INVALID_TARGET", "invalid announcement targeting rules")
)

type AnnouncementTargeting struct {
	// AnyOf 表示 OR：任意一个条件组满足即可展示。
	AnyOf []AnnouncementConditionGroup `json:"any_of,omitempty"`
REDACTED

type AnnouncementConditionGroup struct {
	// AllOf 表示 AND：组内所有条件都满足才算命中该组。
	AllOf []AnnouncementCondition `json:"all_of,omitempty"`
REDACTED

type AnnouncementCondition struct {
	// Type: subscription | balance
	Type string `json:"type"`

	// Operator:
	// - subscription: in
	// - balance: gt/gte/lt/lte/eq
	Operator string `json:"operator"`

	// subscription 条件：匹配的订阅套餐（group_id）
	GroupIDs []int64 `json:"group_ids,omitempty"`

	// balance 条件：比较阈值
	Value float64 `json:"value,omitempty"`
REDACTED

func (t AnnouncementTargeting) Matches(balance float64, activeSubscriptionGroupIDs map[int64]struct{REDACTED) bool {
	// 空规则：展示给所有用户
	if len(t.AnyOf) == 0 {
		return true
REDACTED

	for _, group := range t.AnyOf {
		if len(group.AllOf) == 0 {
			// 空条件组不命中（避免 OR 中出现无条件 “全命中”）
			continue
	REDACTED
		allMatched := true
		for _, cond := range group.AllOf {
			if !cond.Matches(balance, activeSubscriptionGroupIDs) {
				allMatched = false
				break
		REDACTED
	REDACTED
		if allMatched {
			return true
	REDACTED
REDACTED

	return false
REDACTED

func (c AnnouncementCondition) Matches(balance float64, activeSubscriptionGroupIDs map[int64]struct{REDACTED) bool {
	switch c.Type {
	case AnnouncementConditionTypeSubscription:
		if c.Operator != AnnouncementOperatorIn {
			return false
	REDACTED
		if len(c.GroupIDs) == 0 {
			return false
	REDACTED
		if len(activeSubscriptionGroupIDs) == 0 {
			return false
	REDACTED
		for _, gid := range c.GroupIDs {
			if _, ok := activeSubscriptionGroupIDs[gid]; ok {
				return true
		REDACTED
	REDACTED
		return false

	case AnnouncementConditionTypeBalance:
		switch c.Operator {
		case AnnouncementOperatorGT:
			return balance > c.Value
		case AnnouncementOperatorGTE:
			return balance >= c.Value
		case AnnouncementOperatorLT:
			return balance < c.Value
		case AnnouncementOperatorLTE:
			return balance <= c.Value
		case AnnouncementOperatorEQ:
			return balance == c.Value
		default:
			return false
	REDACTED

	default:
		return false
REDACTED
REDACTED

func (t AnnouncementTargeting) NormalizeAndValidate() (AnnouncementTargeting, error) {
	normalized := AnnouncementTargeting{AnyOf: make([]AnnouncementConditionGroup, 0, len(t.AnyOf))REDACTED

	// 允许空 targeting（展示给所有用户）
	if len(t.AnyOf) == 0 {
		return normalized, nil
REDACTED

	if len(t.AnyOf) > 50 {
		return AnnouncementTargeting{REDACTED, ErrAnnouncementInvalidTarget
REDACTED

	for _, g := range t.AnyOf {
		if len(g.AllOf) == 0 {
			return AnnouncementTargeting{REDACTED, ErrAnnouncementInvalidTarget
	REDACTED
		if len(g.AllOf) > 50 {
			return AnnouncementTargeting{REDACTED, ErrAnnouncementInvalidTarget
	REDACTED

		group := AnnouncementConditionGroup{AllOf: make([]AnnouncementCondition, 0, len(g.AllOf))REDACTED
		for _, c := range g.AllOf {
			cond := AnnouncementCondition{
				Type:     strings.TrimSpace(c.Type),
				Operator: strings.TrimSpace(c.Operator),
				Value:    c.Value,
		REDACTED
			for _, gid := range c.GroupIDs {
				if gid <= 0 {
					return AnnouncementTargeting{REDACTED, ErrAnnouncementInvalidTarget
			REDACTED
				cond.GroupIDs = append(cond.GroupIDs, gid)
		REDACTED

			if err := cond.validate(); err != nil {
				return AnnouncementTargeting{REDACTED, err
		REDACTED
			group.AllOf = append(group.AllOf, cond)
	REDACTED

		normalized.AnyOf = append(normalized.AnyOf, group)
REDACTED

	return normalized, nil
REDACTED

func (c AnnouncementCondition) validate() error {
	switch c.Type {
	case AnnouncementConditionTypeSubscription:
		if c.Operator != AnnouncementOperatorIn {
			return ErrAnnouncementInvalidTarget
	REDACTED
		if len(c.GroupIDs) == 0 {
			return ErrAnnouncementInvalidTarget
	REDACTED
		return nil

	case AnnouncementConditionTypeBalance:
		switch c.Operator {
		case AnnouncementOperatorGT, AnnouncementOperatorGTE, AnnouncementOperatorLT, AnnouncementOperatorLTE, AnnouncementOperatorEQ:
			return nil
		default:
			return ErrAnnouncementInvalidTarget
	REDACTED

	default:
		return ErrAnnouncementInvalidTarget
REDACTED
REDACTED

type Announcement struct {
	ID         int64
	Title      string
	Content    string
	Status     string
	NotifyMode string
	Targeting  AnnouncementTargeting
	StartsAt   *time.Time
	EndsAt     *time.Time
	CreatedBy  *int64
	UpdatedBy  *int64
	CreatedAt  time.Time
	UpdatedAt  time.Time
REDACTED

func (a *Announcement) IsActiveAt(now time.Time) bool {
	if a == nil {
		return false
REDACTED
	if a.Status != AnnouncementStatusActive {
		return false
REDACTED
	if a.StartsAt != nil && now.Before(*a.StartsAt) {
		return false
REDACTED
	if a.EndsAt != nil && !now.Before(*a.EndsAt) {
		// ends_at 语义：到点即下线
		return false
REDACTED
	return true
REDACTED
