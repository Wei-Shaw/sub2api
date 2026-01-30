package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAnnouncementTargeting_Matches_EmptyMatchesAll(t *testing.T) {
	var targeting AnnouncementTargeting
	require.True(t, targeting.Matches(0, nil))
	require.True(t, targeting.Matches(123.45, map[int64]struct{REDACTED{1: {REDACTEDREDACTED))
REDACTED

func TestAnnouncementTargeting_NormalizeAndValidate_RejectsEmptyGroup(t *testing.T) {
	targeting := AnnouncementTargeting{
		AnyOf: []AnnouncementConditionGroup{
			{AllOf: nilREDACTED,
	REDACTED,
REDACTED
	_, err := targeting.NormalizeAndValidate()
REDACTED
	require.ErrorIs(t, err, ErrAnnouncementInvalidTarget)
REDACTED

func TestAnnouncementTargeting_NormalizeAndValidate_RejectsInvalidCondition(t *testing.T) {
	targeting := AnnouncementTargeting{
		AnyOf: []AnnouncementConditionGroup{
			{
				AllOf: []AnnouncementCondition{
					{Type: "balance", Operator: "between", Value: 10REDACTED,
			REDACTED,
		REDACTED,
	REDACTED,
REDACTED
	_, err := targeting.NormalizeAndValidate()
REDACTED
	require.ErrorIs(t, err, ErrAnnouncementInvalidTarget)
REDACTED

func TestAnnouncementTargeting_Matches_AndOrSemantics(t *testing.T) {
	targeting := AnnouncementTargeting{
		AnyOf: []AnnouncementConditionGroup{
			{
				AllOf: []AnnouncementCondition{
					{Type: AnnouncementConditionTypeBalance, Operator: AnnouncementOperatorGTE, Value: 100REDACTED,
					{Type: AnnouncementConditionTypeSubscription, Operator: AnnouncementOperatorIn, GroupIDs: []int64{10REDACTEDREDACTED,
			REDACTED,
		REDACTED,
			{
				AllOf: []AnnouncementCondition{
					{Type: AnnouncementConditionTypeBalance, Operator: AnnouncementOperatorLT, Value: 5REDACTED,
			REDACTED,
		REDACTED,
	REDACTED,
REDACTED

	// 命中第 2 组（balance < 5）
	require.True(t, targeting.Matches(4.99, nil))
	require.False(t, targeting.Matches(5, nil))

	// 命中第 1 组（balance >= 100 AND 订阅 in [10]）
	require.False(t, targeting.Matches(100, map[int64]struct{REDACTED{REDACTED))
	require.False(t, targeting.Matches(99.9, map[int64]struct{REDACTED{10: {REDACTEDREDACTED))
	require.True(t, targeting.Matches(100, map[int64]struct{REDACTED{10: {REDACTEDREDACTED))
REDACTED

