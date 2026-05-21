package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

type playgroundRepository struct {
	db *sql.DB
}

func NewPlaygroundRepository(db *sql.DB) service.PlaygroundRepository {
	return &playgroundRepository{db: db}
}

func (r *playgroundRepository) ListChatSessions(ctx context.Context, userID int64, page, pageSize int, _ service.PlaygroundChatSessionListFilters) ([]service.PlaygroundChatSession, int64, error) {
	var total int64
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM playground_chat_sessions WHERE user_id=$1 AND deleted_at IS NULL`, userID).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT id,user_id,title,model,api_key_id,system_prompt,use_context,metadata,last_message_at,created_at,updated_at
		FROM playground_chat_sessions
		WHERE user_id=$1 AND deleted_at IS NULL
		ORDER BY last_message_at DESC, id DESC
		LIMIT $2 OFFSET $3`, userID, pageSize, (page-1)*pageSize)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := make([]service.PlaygroundChatSession, 0)
	for rows.Next() {
		item, err := scanPlaygroundChatSession(rows)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, item)
	}
	return items, total, rows.Err()
}

func (r *playgroundRepository) CreateChatSession(ctx context.Context, userID int64, req service.CreatePlaygroundChatSessionRequest) (*service.PlaygroundChatSession, error) {
	metadata := jsonOrDefault(req.Metadata, map[string]any{})
	row := r.db.QueryRowContext(ctx, `
		INSERT INTO playground_chat_sessions (user_id,title,model,api_key_id,system_prompt,use_context,metadata,last_message_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,NOW())
		RETURNING id,user_id,title,model,api_key_id,system_prompt,use_context,metadata,last_message_at,created_at,updated_at`,
		userID, req.Title, req.Model, nullableInt64(req.APIKeyID), req.SystemPrompt, req.UseContext, metadata)
	item, err := scanPlaygroundChatSession(row)
	return &item, err
}

func (r *playgroundRepository) GetChatSession(ctx context.Context, userID, sessionID int64) (*service.PlaygroundChatSession, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id,user_id,title,model,api_key_id,system_prompt,use_context,metadata,last_message_at,created_at,updated_at
		FROM playground_chat_sessions
		WHERE id=$1 AND user_id=$2 AND deleted_at IS NULL`, sessionID, userID)
	item, err := scanPlaygroundChatSession(row)
	if err != nil {
		return nil, translateSQLNotFound(err, service.ErrPlaygroundRecordNotFound)
	}
	return &item, nil
}

func (r *playgroundRepository) UpdateChatSession(ctx context.Context, userID, sessionID int64, req service.UpdatePlaygroundChatSessionRequest) (*service.PlaygroundChatSession, error) {
	existing, err := r.GetChatSession(ctx, userID, sessionID)
	if err != nil {
		return nil, err
	}
	title := existing.Title
	if req.Title != nil {
		title = *req.Title
	}
	model := existing.Model
	if req.Model != nil {
		model = *req.Model
	}
	systemPrompt := existing.SystemPrompt
	if req.SystemPrompt != nil {
		systemPrompt = *req.SystemPrompt
	}
	useContext := existing.UseContext
	if req.UseContext != nil {
		useContext = *req.UseContext
	}
	apiKeyID := existing.APIKeyID
	if req.APIKeyID != nil {
		apiKeyID = req.APIKeyID
	}
	metadata := existing.Metadata
	if req.Metadata != nil {
		metadata = req.Metadata
	}
	row := r.db.QueryRowContext(ctx, `
		UPDATE playground_chat_sessions
		SET title=$1, model=$2, api_key_id=$3, system_prompt=$4, use_context=$5, metadata=$6, updated_at=NOW()
		WHERE id=$7 AND user_id=$8 AND deleted_at IS NULL
		RETURNING id,user_id,title,model,api_key_id,system_prompt,use_context,metadata,last_message_at,created_at,updated_at`,
		title, model, nullableInt64(apiKeyID), systemPrompt, useContext, jsonOrDefault(metadata, map[string]any{}), sessionID, userID)
	item, err := scanPlaygroundChatSession(row)
	if err != nil {
		return nil, translateSQLNotFound(err, service.ErrPlaygroundRecordNotFound)
	}
	return &item, nil
}

func (r *playgroundRepository) DeleteChatSession(ctx context.Context, userID, sessionID int64) error {
	res, err := r.db.ExecContext(ctx, `UPDATE playground_chat_sessions SET deleted_at=NOW(), updated_at=NOW() WHERE id=$1 AND user_id=$2 AND deleted_at IS NULL`, sessionID, userID)
	return ensureRowsAffected(res, err, service.ErrPlaygroundRecordNotFound)
}

func (r *playgroundRepository) ListChatMessages(ctx context.Context, userID, sessionID int64, page, pageSize int) ([]service.PlaygroundChatMessage, int64, error) {
	if _, err := r.GetChatSession(ctx, userID, sessionID); err != nil {
		return nil, 0, err
	}
	var total int64
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM playground_chat_messages WHERE user_id=$1 AND session_id=$2`, userID, sessionID).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT id,session_id,user_id,api_key_id,role,model,content,content_json,images,usage,status,error,duration_ms,metadata,created_at,updated_at
		FROM playground_chat_messages
		WHERE user_id=$1 AND session_id=$2
		ORDER BY created_at ASC, id ASC
		LIMIT $3 OFFSET $4`, userID, sessionID, pageSize, (page-1)*pageSize)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := make([]service.PlaygroundChatMessage, 0)
	for rows.Next() {
		item, err := scanPlaygroundChatMessage(rows)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, item)
	}
	return items, total, rows.Err()
}

func (r *playgroundRepository) CreateChatMessage(ctx context.Context, userID, sessionID int64, req service.CreatePlaygroundChatMessageRequest) (*service.PlaygroundChatMessage, error) {
	if _, err := r.GetChatSession(ctx, userID, sessionID); err != nil {
		return nil, err
	}
	row := r.db.QueryRowContext(ctx, `
		INSERT INTO playground_chat_messages (session_id,user_id,api_key_id,role,model,content,content_json,images,usage,status,error,duration_ms,metadata)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
		RETURNING id,session_id,user_id,api_key_id,role,model,content,content_json,images,usage,status,error,duration_ms,metadata,created_at,updated_at`,
		sessionID, userID, nullableInt64(req.APIKeyID), req.Role, req.Model, req.Content,
		jsonOrDefault(req.ContentJSON, map[string]any{}), jsonOrDefault(req.Images, []map[string]any{}), jsonOrDefault(req.Usage, map[string]any{}),
		req.Status, req.Error, nullableInt(req.DurationMS), jsonOrDefault(req.Metadata, map[string]any{}))
	item, err := scanPlaygroundChatMessage(row)
	if err != nil {
		return nil, err
	}
	if req.TouchSession {
		_, _ = r.db.ExecContext(ctx, `UPDATE playground_chat_sessions SET last_message_at=$1, updated_at=NOW() WHERE id=$2 AND user_id=$3`, item.CreatedAt, sessionID, userID)
	}
	return &item, nil
}

func (r *playgroundRepository) ListImageTasks(ctx context.Context, userID int64, page, pageSize int, filters service.PlaygroundImageTaskListFilters) ([]service.PlaygroundImageTask, int64, error) {
	where := `user_id=$1 AND deleted_at IS NULL`
	args := []any{userID}
	if strings.TrimSpace(filters.Status) != "" {
		where += ` AND status=$2`
		args = append(args, strings.TrimSpace(filters.Status))
	}
	var total int64
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM playground_image_tasks WHERE `+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, pageSize, (page-1)*pageSize)
	rows, err := r.db.QueryContext(ctx, `
		SELECT id,user_id,api_key_id,model,prompt,quality,size,n,endpoint,status,request,reference_images,result_images,response,error,usage,cost,duration_ms,metadata,created_at,updated_at
		FROM playground_image_tasks
		WHERE `+where+`
		ORDER BY created_at DESC, id DESC
		LIMIT $`+fmt.Sprint(len(args)-1)+` OFFSET $`+fmt.Sprint(len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := make([]service.PlaygroundImageTask, 0)
	for rows.Next() {
		item, err := scanPlaygroundImageTask(rows)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, item)
	}
	return items, total, rows.Err()
}

func (r *playgroundRepository) CreateImageTask(ctx context.Context, userID int64, req service.CreatePlaygroundImageTaskRequest) (*service.PlaygroundImageTask, error) {
	row := r.db.QueryRowContext(ctx, `
		INSERT INTO playground_image_tasks (user_id,api_key_id,model,prompt,quality,size,n,endpoint,status,request,reference_images,result_images,response,error,usage,cost,duration_ms,metadata)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)
		RETURNING id,user_id,api_key_id,model,prompt,quality,size,n,endpoint,status,request,reference_images,result_images,response,error,usage,cost,duration_ms,metadata,created_at,updated_at`,
		userID, nullableInt64(req.APIKeyID), req.Model, req.Prompt, req.Quality, req.Size, req.N, req.Endpoint, req.Status,
		jsonOrDefault(req.Request, map[string]any{}), jsonOrDefault(req.ReferenceImages, []map[string]any{}), jsonOrDefault(req.ResultImages, []map[string]any{}),
		jsonOrDefault(req.Response, map[string]any{}), req.Error, jsonOrDefault(req.Usage, map[string]any{}), req.Cost, nullableInt(req.DurationMS), jsonOrDefault(req.Metadata, map[string]any{}))
	item, err := scanPlaygroundImageTask(row)
	return &item, err
}

func (r *playgroundRepository) GetImageTask(ctx context.Context, userID, taskID int64) (*service.PlaygroundImageTask, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id,user_id,api_key_id,model,prompt,quality,size,n,endpoint,status,request,reference_images,result_images,response,error,usage,cost,duration_ms,metadata,created_at,updated_at
		FROM playground_image_tasks
		WHERE id=$1 AND user_id=$2 AND deleted_at IS NULL`, taskID, userID)
	item, err := scanPlaygroundImageTask(row)
	if err != nil {
		return nil, translateSQLNotFound(err, service.ErrPlaygroundRecordNotFound)
	}
	return &item, nil
}

func (r *playgroundRepository) UpdateImageTask(ctx context.Context, userID, taskID int64, req service.UpdatePlaygroundImageTaskRequest) (*service.PlaygroundImageTask, error) {
	existing, err := r.GetImageTask(ctx, userID, taskID)
	if err != nil {
		return nil, err
	}
	status := existing.Status
	if req.Status != nil {
		status = *req.Status
	}
	errorText := existing.Error
	if req.Error != nil {
		errorText = *req.Error
	}
	cost := existing.Cost
	if req.Cost != nil {
		cost = *req.Cost
	}
	durationMS := existing.DurationMS
	if req.DurationMS != nil {
		durationMS = req.DurationMS
	}
	resultImages := existing.ResultImages
	if req.ResultImages != nil {
		resultImages = req.ResultImages
	}
	responseBody := existing.Response
	if req.Response != nil {
		responseBody = req.Response
	}
	usage := existing.Usage
	if req.Usage != nil {
		usage = req.Usage
	}
	metadata := existing.Metadata
	if req.Metadata != nil {
		metadata = req.Metadata
	}
	row := r.db.QueryRowContext(ctx, `
		UPDATE playground_image_tasks
		SET status=$1,result_images=$2,response=$3,error=$4,usage=$5,cost=$6,duration_ms=$7,metadata=$8,updated_at=NOW()
		WHERE id=$9 AND user_id=$10 AND deleted_at IS NULL
		RETURNING id,user_id,api_key_id,model,prompt,quality,size,n,endpoint,status,request,reference_images,result_images,response,error,usage,cost,duration_ms,metadata,created_at,updated_at`,
		status, jsonOrDefault(resultImages, []map[string]any{}), jsonOrDefault(responseBody, map[string]any{}), errorText, jsonOrDefault(usage, map[string]any{}), cost,
		nullableInt(durationMS), jsonOrDefault(metadata, map[string]any{}), taskID, userID)
	item, err := scanPlaygroundImageTask(row)
	if err != nil {
		return nil, translateSQLNotFound(err, service.ErrPlaygroundRecordNotFound)
	}
	return &item, nil
}

func (r *playgroundRepository) DeleteImageTask(ctx context.Context, userID, taskID int64) error {
	res, err := r.db.ExecContext(ctx, `UPDATE playground_image_tasks SET deleted_at=NOW(), updated_at=NOW() WHERE id=$1 AND user_id=$2 AND deleted_at IS NULL`, taskID, userID)
	return ensureRowsAffected(res, err, service.ErrPlaygroundRecordNotFound)
}

type scanner interface{ Scan(dest ...any) error }

func scanPlaygroundChatSession(row scanner) (service.PlaygroundChatSession, error) {
	var item service.PlaygroundChatSession
	var apiKeyID sql.NullInt64
	var metadata []byte
	err := row.Scan(&item.ID, &item.UserID, &item.Title, &item.Model, &apiKeyID, &item.SystemPrompt, &item.UseContext, &metadata, &item.LastMessageAt, &item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		return item, err
	}
	item.APIKeyID = ptrFromNullInt64(apiKeyID)
	item.Metadata = decodeMap(metadata)
	return item, nil
}

func scanPlaygroundChatMessage(row scanner) (service.PlaygroundChatMessage, error) {
	var item service.PlaygroundChatMessage
	var apiKeyID sql.NullInt64
	var contentJSON, images, usage, metadata []byte
	var durationMS sql.NullInt64
	err := row.Scan(&item.ID, &item.SessionID, &item.UserID, &apiKeyID, &item.Role, &item.Model, &item.Content, &contentJSON, &images, &usage, &item.Status, &item.Error, &durationMS, &metadata, &item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		return item, err
	}
	item.APIKeyID = ptrFromNullInt64(apiKeyID)
	item.DurationMS = ptrIntFromNullInt64(durationMS)
	item.ContentJSON = decodeMap(contentJSON)
	item.Images = decodeSliceMap(images)
	item.Usage = decodeMap(usage)
	item.Metadata = decodeMap(metadata)
	return item, nil
}

func scanPlaygroundImageTask(row scanner) (service.PlaygroundImageTask, error) {
	var item service.PlaygroundImageTask
	var apiKeyID sql.NullInt64
	var request, referenceImages, resultImages, responseBody, usage, metadata []byte
	var durationMS sql.NullInt64
	err := row.Scan(&item.ID, &item.UserID, &apiKeyID, &item.Model, &item.Prompt, &item.Quality, &item.Size, &item.N, &item.Endpoint, &item.Status, &request, &referenceImages, &resultImages, &responseBody, &item.Error, &usage, &item.Cost, &durationMS, &metadata, &item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		return item, err
	}
	item.APIKeyID = ptrFromNullInt64(apiKeyID)
	item.DurationMS = ptrIntFromNullInt64(durationMS)
	item.Request = decodeMap(request)
	item.ReferenceImages = decodeSliceMap(referenceImages)
	item.ResultImages = decodeSliceMap(resultImages)
	item.Response = decodeMap(responseBody)
	item.Usage = decodeMap(usage)
	item.Metadata = decodeMap(metadata)
	return item, nil
}

func jsonOrDefault(v any, fallback any) []byte {
	if v == nil {
		v = fallback
	}
	b, err := json.Marshal(v)
	if err != nil {
		b, _ = json.Marshal(fallback)
	}
	return b
}

func decodeMap(b []byte) map[string]any {
	out := map[string]any{}
	if len(b) > 0 {
		_ = json.Unmarshal(b, &out)
	}
	return out
}

func decodeSliceMap(b []byte) []map[string]any {
	out := []map[string]any{}
	if len(b) > 0 {
		_ = json.Unmarshal(b, &out)
	}
	return out
}

func nullableInt64(v *int64) any {
	if v == nil || *v == 0 {
		return nil
	}
	return *v
}

func nullableInt(v *int) any {
	if v == nil {
		return nil
	}
	return *v
}

func ptrFromNullInt64(v sql.NullInt64) *int64 {
	if !v.Valid {
		return nil
	}
	out := v.Int64
	return &out
}

func ptrIntFromNullInt64(v sql.NullInt64) *int {
	if !v.Valid {
		return nil
	}
	out := int(v.Int64)
	return &out
}

func translateSQLNotFound(err error, notFound error) error {
	if err == sql.ErrNoRows {
		return notFound
	}
	return err
}

func ensureRowsAffected(res sql.Result, err error, notFound error) error {
	if err != nil {
		return err
	}
	if res == nil {
		return nil
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return notFound
	}
	return nil
}
