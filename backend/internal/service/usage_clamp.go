package service

import "time"

// clampInt 将整数限制在指定范围内
func clampInt(value, min, max int) int {
	if value < min {
		return min
REDACTED
	if value > max {
		return max
REDACTED
	return value
REDACTED

// clampFloat64 将浮点数限制在指定范围内
func clampFloat64(value, min, max float64) float64 {
	if value < min {
		return min
REDACTED
	if value > max {
		return max
REDACTED
	return value
REDACTED

// remainingSecondsUntil 计算到指定时间的剩余秒数，保证非负
func remainingSecondsUntil(t time.Time) int {
	seconds := int(time.Until(t).Seconds())
	if seconds < 0 {
		return 0
REDACTED
	return seconds
REDACTED

