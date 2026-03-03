package repository

import (
	"fmt"
	"strings"
)

type latencyHistogramBucket struct {
	upperMs int
	label   string
REDACTED

var latencyHistogramBuckets = []latencyHistogramBucket{
	{upperMs: 100, label: "0-100ms"REDACTED,
	{upperMs: 200, label: "100-200ms"REDACTED,
	{upperMs: 500, label: "200-500ms"REDACTED,
	{upperMs: 1000, label: "500-1000ms"REDACTED,
	{upperMs: 2000, label: "1000-2000ms"REDACTED,
	{upperMs: 0, label: "2000ms+"REDACTED, // default bucket
REDACTED

var latencyHistogramOrderedRanges = func() []string {
	out := make([]string, 0, len(latencyHistogramBuckets))
	for _, b := range latencyHistogramBuckets {
		out = append(out, b.label)
REDACTED
	return out
REDACTED()

func latencyHistogramRangeCaseExpr(column string) string {
	var sb strings.Builder
	_, _ = sb.WriteString("CASE\n")

	for _, b := range latencyHistogramBuckets {
		if b.upperMs <= 0 {
			continue
	REDACTED
		fmt.Fprintf(&sb, "\tWHEN %s < %d THEN '%s'\n", column, b.upperMs, b.label)
REDACTED

	// Default bucket.
	last := latencyHistogramBuckets[len(latencyHistogramBuckets)-1]
	fmt.Fprintf(&sb, "\tELSE '%s'\n", last.label)
	_, _ = sb.WriteString("END")
	return sb.String()
REDACTED

func latencyHistogramRangeOrderCaseExpr(column string) string {
	var sb strings.Builder
	_, _ = sb.WriteString("CASE\n")

	order := 1
	for _, b := range latencyHistogramBuckets {
		if b.upperMs <= 0 {
			continue
	REDACTED
		fmt.Fprintf(&sb, "\tWHEN %s < %d THEN %d\n", column, b.upperMs, order)
		order++
REDACTED

	fmt.Fprintf(&sb, "\tELSE %d\n", order)
	_, _ = sb.WriteString("END")
	return sb.String()
REDACTED
