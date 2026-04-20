package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type AuthIdentityMigrationReport struct {
	ID         int64
	ReportType string
	ReportKey  string
	Details    map[string]any
	CreatedAt  time.Time
REDACTED

type AuthIdentityMigrationReportQuery struct {
	ReportType string
	Limit      int
	Offset     int
REDACTED

type AuthIdentityMigrationReportSummary struct {
	Total  int64
	ByType map[string]int64
REDACTED

func (r *userRepository) ListAuthIdentityMigrationReports(ctx context.Context, query AuthIdentityMigrationReportQuery) ([]AuthIdentityMigrationReport, error) {
	exec := txAwareSQLExecutor(ctx, r.sql, r.client)
	if exec == nil {
		return nil, fmt.Errorf("sql executor is not configured")
REDACTED

	limit := query.Limit
	if limit <= 0 {
		limit = 100
REDACTED
	rows, err := exec.QueryContext(ctx, `
SELECT id, report_type, report_key, details, created_at
FROM auth_identity_migration_reports
WHERE ($1 = '' OR report_type = $1)
ORDER BY created_at DESC, id DESC
LIMIT $2 OFFSET $3`,
		strings.TrimSpace(query.ReportType),
		limit,
		query.Offset,
	)
	if err != nil {
		return nil, err
REDACTED
	defer func() { _ = rows.Close() REDACTED()

	reports := make([]AuthIdentityMigrationReport, 0)
	for rows.Next() {
		report, scanErr := scanAuthIdentityMigrationReport(rows)
		if scanErr != nil {
			return nil, scanErr
	REDACTED
		reports = append(reports, report)
REDACTED
	if err := rows.Err(); err != nil {
		return nil, err
REDACTED
	return reports, nil
REDACTED

func (r *userRepository) GetAuthIdentityMigrationReport(ctx context.Context, reportType, reportKey string) (*AuthIdentityMigrationReport, error) {
	exec := txAwareSQLExecutor(ctx, r.sql, r.client)
	if exec == nil {
		return nil, fmt.Errorf("sql executor is not configured")
REDACTED

	rows, err := exec.QueryContext(ctx, `
SELECT id, report_type, report_key, details, created_at
FROM auth_identity_migration_reports
WHERE report_type = $1 AND report_key = $2
LIMIT 1`,
		strings.TrimSpace(reportType),
		strings.TrimSpace(reportKey),
	)
	if err != nil {
		return nil, err
REDACTED
	defer func() { _ = rows.Close() REDACTED()

	if !rows.Next() {
		return nil, sql.ErrNoRows
REDACTED
	report, err := scanAuthIdentityMigrationReport(rows)
	if err != nil {
		return nil, err
REDACTED
	return &report, rows.Err()
REDACTED

func (r *userRepository) SummarizeAuthIdentityMigrationReports(ctx context.Context) (*AuthIdentityMigrationReportSummary, error) {
	exec := txAwareSQLExecutor(ctx, r.sql, r.client)
	if exec == nil {
		return nil, fmt.Errorf("sql executor is not configured")
REDACTED

	rows, err := exec.QueryContext(ctx, `
SELECT report_type, COUNT(*)
FROM auth_identity_migration_reports
GROUP BY report_type
ORDER BY report_type ASC`)
	if err != nil {
		return nil, err
REDACTED
	defer func() { _ = rows.Close() REDACTED()

	summary := &AuthIdentityMigrationReportSummary{
		ByType: make(map[string]int64),
REDACTED
	for rows.Next() {
		var reportType string
		var count int64
		if err := rows.Scan(&reportType, &count); err != nil {
			return nil, err
	REDACTED
		summary.ByType[reportType] = count
		summary.Total += count
REDACTED
	if err := rows.Err(); err != nil {
		return nil, err
REDACTED
	return summary, nil
REDACTED

func scanAuthIdentityMigrationReport(scanner interface{ Scan(dest ...any) error REDACTED) (AuthIdentityMigrationReport, error) {
	var (
		report  AuthIdentityMigrationReport
		details []byte
	)
	if err := scanner.Scan(&report.ID, &report.ReportType, &report.ReportKey, &details, &report.CreatedAt); err != nil {
		return AuthIdentityMigrationReport{REDACTED, err
REDACTED
	report.Details = map[string]any{REDACTED
	if len(details) > 0 {
		if err := json.Unmarshal(details, &report.Details); err != nil {
			return AuthIdentityMigrationReport{REDACTED, err
	REDACTED
REDACTED
	return report, nil
REDACTED
