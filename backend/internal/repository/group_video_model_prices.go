package repository

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

func loadGroupVideoModelPrices(ctx context.Context, sqlq sqlExecutor, groupIDs []int64) (map[int64]map[string]map[string]float64, error) {
	out := make(map[int64]map[string]map[string]float64, len(groupIDs))
	if sqlq == nil || len(groupIDs) == 0 {
		return out, nil
REDACTED

	rows, err := sqlq.QueryContext(ctx, `
		SELECT id, video_model_prices
		FROM groups
		WHERE id = ANY($1) AND deleted_at IS NULL
	`, pq.Array(groupIDs))
	if err != nil {
		return nil, err
REDACTED
	defer func() { _ = rows.Close() REDACTED()

	for rows.Next() {
		var (
			groupID int64
			raw     []byte
		)
		if err := rows.Scan(&groupID, &raw); err != nil {
			return nil, err
	REDACTED
		prices, err := decodeVideoModelPrices(raw)
		if err != nil {
			return nil, err
	REDACTED
		if prices != nil {
			out[groupID] = prices
	REDACTED
REDACTED
	if err := rows.Err(); err != nil {
		return nil, err
REDACTED
	return out, nil
REDACTED

func saveGroupVideoModelPrices(ctx context.Context, sqlq sqlExecutor, groupID int64, prices map[string]map[string]float64) error {
	if sqlq == nil || groupID <= 0 {
		return nil
REDACTED
	normalized := service.NormalizeVideoModelPrices(prices)
	if len(normalized) == 0 {
		_, err := sqlq.ExecContext(ctx, `
			UPDATE groups
			SET video_model_prices = NULL
			WHERE id = $1
		`, groupID)
		return err
REDACTED
	payload, err := json.Marshal(normalized)
	if err != nil {
		return err
REDACTED
	_, err = sqlq.ExecContext(ctx, `
		UPDATE groups
		SET video_model_prices = $1::jsonb
		WHERE id = $2
	`, string(payload), groupID)
	return err
REDACTED

func applyVideoModelPricesToGroups(groups []service.Group, pricesByID map[int64]map[string]map[string]float64) {
	for i := range groups {
		if prices, ok := pricesByID[groups[i].ID]; ok {
			groups[i].VideoModelPrices = service.NormalizeVideoModelPrices(prices)
			continue
	REDACTED
		groups[i].VideoModelPrices = service.NormalizeVideoModelPrices(groups[i].VideoModelPrices)
REDACTED
REDACTED

func applyVideoModelPricesToGroup(group *service.Group, pricesByID map[int64]map[string]map[string]float64) {
	if group == nil {
		return
REDACTED
	if prices, ok := pricesByID[group.ID]; ok {
		group.VideoModelPrices = service.NormalizeVideoModelPrices(prices)
		return
REDACTED
	group.VideoModelPrices = service.NormalizeVideoModelPrices(group.VideoModelPrices)
REDACTED

func decodeVideoModelPrices(raw []byte) (map[string]map[string]float64, error) {
	if len(raw) == 0 {
		return nil, nil
REDACTED
	// Driver may return NULL as nil slice; treat empty JSON as nil.
	trimmed := string(raw)
	if trimmed == "" || trimmed == "null" {
		return nil, nil
REDACTED
	var parsed map[string]map[string]float64
	if err := json.Unmarshal(raw, &parsed); err != nil {
		// Some drivers surface NULL via sql.NullString paths; tolerate empty object.
		if err == sql.ErrNoRows {
			return nil, nil
	REDACTED
		return nil, err
REDACTED
	return service.NormalizeVideoModelPrices(parsed), nil
REDACTED
