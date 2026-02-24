//go:build unit

package service

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// ---------- 辅助解析函数（复制生产代码中的 gjson 解析逻辑，用于单元测试） ----------

// testParseUploadOrCreateTaskID 模拟 UploadImage / CreateImageTask / CreateVideoTask 中
// 用 gjson.GetBytes(respBody, "id") 提取 id 的逻辑。
func testParseUploadOrCreateTaskID(respBody []byte) (string, error) {
	id := strings.TrimSpace(gjson.GetBytes(respBody, "id").String())
	if id == "" {
		return "", assert.AnError // 占位错误，表示 "missing id"
REDACTED
	return id, nil
REDACTED

// testParseFetchRecentImageTask 模拟 fetchRecentImageTask 中的 gjson.ForEach 解析逻辑。
func testParseFetchRecentImageTask(respBody []byte, taskID string) (*SoraImageTaskStatus, bool) {
	var found *SoraImageTaskStatus
	gjson.GetBytes(respBody, "task_responses").ForEach(func(_, item gjson.Result) bool {
		if item.Get("id").String() != taskID {
			return true // continue
	REDACTED
		status := strings.TrimSpace(item.Get("status").String())
		progress := item.Get("progress_pct").Float()
		var urls []string
		item.Get("generations").ForEach(func(_, gen gjson.Result) bool {
			if u := strings.TrimSpace(gen.Get("url").String()); u != "" {
				urls = append(urls, u)
		REDACTED
			return true
	REDACTED)
		found = &SoraImageTaskStatus{
			ID:          taskID,
			Status:      status,
			ProgressPct: progress,
			URLs:        urls,
	REDACTED
		return false // break
REDACTED)
	if found != nil {
		return found, true
REDACTED
	return &SoraImageTaskStatus{ID: taskID, Status: "processing"REDACTED, false
REDACTED

// testParseGetVideoTaskPending 模拟 GetVideoTask 中解析 pending 列表的逻辑。
func testParseGetVideoTaskPending(respBody []byte, taskID string) (*SoraVideoTaskStatus, bool) {
	pendingResult := gjson.ParseBytes(respBody)
	if !pendingResult.IsArray() {
		return nil, false
REDACTED
	var pendingFound *SoraVideoTaskStatus
	pendingResult.ForEach(func(_, task gjson.Result) bool {
		if task.Get("id").String() != taskID {
			return true
	REDACTED
		progress := 0
		if v := task.Get("progress_pct"); v.Exists() {
			progress = int(v.Float() * 100)
	REDACTED
		status := strings.TrimSpace(task.Get("status").String())
		pendingFound = &SoraVideoTaskStatus{
			ID:          taskID,
			Status:      status,
			ProgressPct: progress,
	REDACTED
		return false
REDACTED)
	if pendingFound != nil {
		return pendingFound, true
REDACTED
	return nil, false
REDACTED

// testParseGetVideoTaskDrafts 模拟 GetVideoTask 中解析 drafts 列表的逻辑。
func testParseGetVideoTaskDrafts(respBody []byte, taskID string) (*SoraVideoTaskStatus, bool) {
	var draftFound *SoraVideoTaskStatus
	gjson.GetBytes(respBody, "items").ForEach(func(_, draft gjson.Result) bool {
		if draft.Get("task_id").String() != taskID {
			return true
	REDACTED
		kind := strings.TrimSpace(draft.Get("kind").String())
		reason := strings.TrimSpace(draft.Get("reason_str").String())
		if reason == "" {
			reason = strings.TrimSpace(draft.Get("markdown_reason_str").String())
	REDACTED
		urlStr := strings.TrimSpace(draft.Get("downloadable_url").String())
		if urlStr == "" {
			urlStr = strings.TrimSpace(draft.Get("url").String())
	REDACTED

		if kind == "sora_content_violation" || reason != "" || urlStr == "" {
			msg := reason
			if msg == "" {
				msg = "Content violates guardrails"
		REDACTED
			draftFound = &SoraVideoTaskStatus{
				ID:       taskID,
				Status:   "failed",
				ErrorMsg: msg,
		REDACTED
	REDACTED else {
			draftFound = &SoraVideoTaskStatus{
				ID:     taskID,
				Status: "completed",
				URLs:   []string{urlStrREDACTED,
		REDACTED
	REDACTED
		return false
REDACTED)
	if draftFound != nil {
		return draftFound, true
REDACTED
	return nil, false
REDACTED

// ===================== Test 1: TestSoraParseUploadResponse =====================

func TestSoraParseUploadResponse(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantID  string
		wantErr bool
REDACTED{
		{
			name:   "正常 id",
			body:   `{"id":"file-abc123","status":"uploaded"REDACTED`,
			wantID: "file-abc123",
	REDACTED,
		{
			name:    "空 id",
			body:    `{"id":"","status":"uploaded"REDACTED`,
			wantErr: true,
	REDACTED,
		{
			name:    "无 id 字段",
			body:    `{"status":"uploaded"REDACTED`,
			wantErr: true,
	REDACTED,
		{
			name:    "id 全为空白",
			body:    `{"id":"   ","status":"uploaded"REDACTED`,
			wantErr: true,
	REDACTED,
		{
			name:   "id 前后有空白",
			body:   `{"id":"  file-trimmed  ","status":"uploaded"REDACTED`,
			wantID: "file-trimmed",
	REDACTED,
		{
			name:    "空 JSON 对象",
			body:    `{REDACTED`,
			wantErr: true,
	REDACTED,
REDACTED
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, err := testParseUploadOrCreateTaskID([]byte(tt.body))
			if tt.wantErr {
				require.Error(t, err, "应返回错误")
				return
		REDACTED
		REDACTED
			require.Equal(t, tt.wantID, id)
	REDACTED)
REDACTED
REDACTED

// ===================== Test 2: TestSoraParseCreateTaskResponse =====================

func TestSoraParseCreateTaskResponse(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantID  string
		wantErr bool
REDACTED{
		{
			name:   "正常任务 id",
			body:   `{"id":"task-123"REDACTED`,
			wantID: "task-123",
	REDACTED,
		{
			name:    "缺失 id",
			body:    `{"status":"created"REDACTED`,
			wantErr: true,
	REDACTED,
		{
			name:    "空 id",
			body:    `{"id":"  "REDACTED`,
			wantErr: true,
	REDACTED,
		{
			name:   "id 为数字（gjson 转字符串）",
			body:   `{"id":123REDACTED`,
			wantID: "123",
	REDACTED,
		{
			name:   "id 含特殊字符",
			body:   `{"id":"task-abc-def-456-ghi"REDACTED`,
			wantID: "task-abc-def-456-ghi",
	REDACTED,
		{
			name:   "额外字段不影响解析",
			body:   `{"id":"task-999","type":"image_gen","extra":"data"REDACTED`,
			wantID: "task-999",
	REDACTED,
REDACTED
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, err := testParseUploadOrCreateTaskID([]byte(tt.body))
			if tt.wantErr {
				require.Error(t, err, "应返回错误")
				return
		REDACTED
		REDACTED
			require.Equal(t, tt.wantID, id)
	REDACTED)
REDACTED
REDACTED

// ===================== Test 3: TestSoraParseFetchRecentImageTask =====================

func TestSoraParseFetchRecentImageTask(t *testing.T) {
	tests := []struct {
		name         string
		body         string
		taskID       string
		wantFound    bool
		wantStatus   string
		wantProgress float64
		wantURLs     []string
REDACTED{
		{
			name:         "匹配已完成任务",
			body:         `{"task_responses":[{"id":"task-1","status":"completed","progress_pct":1.0,"generations":[{"url":"https://example.com/img.png"REDACTED]REDACTED]REDACTED`,
			taskID:       "task-1",
			wantFound:    true,
			wantStatus:   "completed",
			wantProgress: 1.0,
			wantURLs:     []string{"https://example.com/img.png"REDACTED,
	REDACTED,
		{
			name:         "匹配处理中任务",
			body:         `{"task_responses":[{"id":"task-2","status":"processing","progress_pct":0.5,"generations":[]REDACTED]REDACTED`,
			taskID:       "task-2",
			wantFound:    true,
			wantStatus:   "processing",
			wantProgress: 0.5,
			wantURLs:     nil,
	REDACTED,
		{
			name:       "无匹配任务",
			body:       `{"task_responses":[{"id":"other","status":"completed"REDACTED]REDACTED`,
			taskID:     "task-1",
			wantFound:  false,
			wantStatus: "processing",
	REDACTED,
		{
			name:       "空 task_responses",
			body:       `{"task_responses":[]REDACTED`,
			taskID:     "task-1",
			wantFound:  false,
			wantStatus: "processing",
	REDACTED,
		{
			name:       "缺少 task_responses 字段",
			body:       `{"other":"data"REDACTED`,
			taskID:     "task-1",
			wantFound:  false,
			wantStatus: "processing",
	REDACTED,
		{
			name:         "多个任务中精准匹配",
			body:         `{"task_responses":[{"id":"task-a","status":"completed","progress_pct":1.0,"generations":[{"url":"https://a.com/1.png"REDACTED]REDACTED,{"id":"task-b","status":"processing","progress_pct":0.3,"generations":[]REDACTED,{"id":"task-c","status":"failed","progress_pct":0REDACTED]REDACTED`,
			taskID:       "task-b",
			wantFound:    true,
			wantStatus:   "processing",
			wantProgress: 0.3,
			wantURLs:     nil,
	REDACTED,
		{
			name:         "多个 generations",
			body:         `{"task_responses":[{"id":"task-m","status":"completed","progress_pct":1.0,"generations":[{"url":"https://a.com/1.png"REDACTED,{"url":"https://a.com/2.png"REDACTED,{"url":""REDACTED]REDACTED]REDACTED`,
			taskID:       "task-m",
			wantFound:    true,
			wantStatus:   "completed",
			wantProgress: 1.0,
			wantURLs:     []string{"https://a.com/1.png", "https://a.com/2.png"REDACTED,
	REDACTED,
REDACTED
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, found := testParseFetchRecentImageTask([]byte(tt.body), tt.taskID)
			require.Equal(t, tt.wantFound, found, "found 不匹配")
			require.NotNil(t, status)
			require.Equal(t, tt.taskID, status.ID)
			require.Equal(t, tt.wantStatus, status.Status)
			if tt.wantFound {
				require.InDelta(t, tt.wantProgress, status.ProgressPct, 0.001, "进度不匹配")
				require.Equal(t, tt.wantURLs, status.URLs)
		REDACTED
	REDACTED)
REDACTED
REDACTED

// ===================== Test 4: TestSoraParseGetVideoTaskPending =====================

func TestSoraParseGetVideoTaskPending(t *testing.T) {
	tests := []struct {
		name         string
		body         string
		taskID       string
		wantFound    bool
		wantStatus   string
		wantProgress int
REDACTED{
		{
			name:         "匹配 pending 任务",
			body:         `[{"id":"task-1","status":"processing","progress_pct":0.5REDACTED]`,
			taskID:       "task-1",
			wantFound:    true,
			wantStatus:   "processing",
			wantProgress: 50,
	REDACTED,
		{
			name:         "进度为 0",
			body:         `[{"id":"task-2","status":"queued","progress_pct":0REDACTED]`,
			taskID:       "task-2",
			wantFound:    true,
			wantStatus:   "queued",
			wantProgress: 0,
	REDACTED,
		{
			name:         "进度为 1（100%）",
			body:         `[{"id":"task-3","status":"completing","progress_pct":1.0REDACTED]`,
			taskID:       "task-3",
			wantFound:    true,
			wantStatus:   "completing",
			wantProgress: 100,
	REDACTED,
		{
			name:      "空数组",
			body:      `[]`,
			taskID:    "task-1",
			wantFound: false,
	REDACTED,
		{
			name:      "无匹配 id",
			body:      `[{"id":"task-other","status":"processing","progress_pct":0.3REDACTED]`,
			taskID:    "task-1",
			wantFound: false,
	REDACTED,
		{
			name:         "多个任务精准匹配",
			body:         `[{"id":"task-a","status":"processing","progress_pct":0.2REDACTED,{"id":"task-b","status":"queued","progress_pct":0REDACTED,{"id":"task-c","status":"processing","progress_pct":0.8REDACTED]`,
			taskID:       "task-c",
			wantFound:    true,
			wantStatus:   "processing",
			wantProgress: 80,
	REDACTED,
		{
			name:      "非数组 JSON",
			body:      `{"id":"task-1","status":"processing"REDACTED`,
			taskID:    "task-1",
			wantFound: false,
	REDACTED,
		{
			name:         "无 progress_pct 字段",
			body:         `[{"id":"task-4","status":"pending"REDACTED]`,
			taskID:       "task-4",
			wantFound:    true,
			wantStatus:   "pending",
			wantProgress: 0,
	REDACTED,
REDACTED
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, found := testParseGetVideoTaskPending([]byte(tt.body), tt.taskID)
			require.Equal(t, tt.wantFound, found, "found 不匹配")
			if tt.wantFound {
				require.NotNil(t, status)
				require.Equal(t, tt.taskID, status.ID)
				require.Equal(t, tt.wantStatus, status.Status)
				require.Equal(t, tt.wantProgress, status.ProgressPct)
		REDACTED
	REDACTED)
REDACTED
REDACTED

// ===================== Test 5: TestSoraParseGetVideoTaskDrafts =====================

func TestSoraParseGetVideoTaskDrafts(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		taskID     string
		wantFound  bool
		wantStatus string
		wantURLs   []string
		wantErr    string
REDACTED{
		{
			name:       "正常完成的视频",
			body:       `{"items":[{"task_id":"task-1","kind":"video","downloadable_url":"https://example.com/video.mp4"REDACTED]REDACTED`,
			taskID:     "task-1",
			wantFound:  true,
			wantStatus: "completed",
			wantURLs:   []string{"https://example.com/video.mp4"REDACTED,
	REDACTED,
		{
			name:       "使用 url 字段回退",
			body:       `{"items":[{"task_id":"task-2","kind":"video","url":"https://example.com/fallback.mp4"REDACTED]REDACTED`,
			taskID:     "task-2",
			wantFound:  true,
			wantStatus: "completed",
			wantURLs:   []string{"https://example.com/fallback.mp4"REDACTED,
	REDACTED,
		{
			name:       "内容违规",
			body:       `{"items":[{"task_id":"task-3","kind":"sora_content_violation","reason_str":"Content policy violation"REDACTED]REDACTED`,
			taskID:     "task-3",
			wantFound:  true,
			wantStatus: "failed",
			wantErr:    "Content policy violation",
	REDACTED,
		{
			name:       "内容违规 - markdown_reason_str 回退",
			body:       `{"items":[{"task_id":"task-4","kind":"sora_content_violation","markdown_reason_str":"Markdown reason"REDACTED]REDACTED`,
			taskID:     "task-4",
			wantFound:  true,
			wantStatus: "failed",
			wantErr:    "Markdown reason",
	REDACTED,
		{
			name:       "内容违规 - 无 reason 使用默认消息",
			body:       `{"items":[{"task_id":"task-5","kind":"sora_content_violation"REDACTED]REDACTED`,
			taskID:     "task-5",
			wantFound:  true,
			wantStatus: "failed",
			wantErr:    "Content violates guardrails",
	REDACTED,
		{
			name:       "有 reason_str 但非 violation kind（仍判定失败）",
			body:       `{"items":[{"task_id":"task-6","kind":"video","reason_str":"Some error occurred"REDACTED]REDACTED`,
			taskID:     "task-6",
			wantFound:  true,
			wantStatus: "failed",
			wantErr:    "Some error occurred",
	REDACTED,
		{
			name:       "空 URL 判定为失败",
			body:       `{"items":[{"task_id":"task-7","kind":"video","downloadable_url":"","url":""REDACTED]REDACTED`,
			taskID:     "task-7",
			wantFound:  true,
			wantStatus: "failed",
			wantErr:    "Content violates guardrails",
	REDACTED,
		{
			name:      "无匹配 task_id",
			body:      `{"items":[{"task_id":"task-other","kind":"video","downloadable_url":"https://example.com/video.mp4"REDACTED]REDACTED`,
			taskID:    "task-1",
			wantFound: false,
	REDACTED,
		{
			name:      "空 items",
			body:      `{"items":[]REDACTED`,
			taskID:    "task-1",
			wantFound: false,
	REDACTED,
		{
			name:      "缺少 items 字段",
			body:      `{"other":"data"REDACTED`,
			taskID:    "task-1",
			wantFound: false,
	REDACTED,
		{
			name:       "多个 items 精准匹配",
			body:       `{"items":[{"task_id":"task-a","kind":"video","downloadable_url":"https://a.com/a.mp4"REDACTED,{"task_id":"task-b","kind":"sora_content_violation","reason_str":"Bad content"REDACTED,{"task_id":"task-c","kind":"video","downloadable_url":"https://c.com/c.mp4"REDACTED]REDACTED`,
			taskID:     "task-b",
			wantFound:  true,
			wantStatus: "failed",
			wantErr:    "Bad content",
	REDACTED,
REDACTED
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, found := testParseGetVideoTaskDrafts([]byte(tt.body), tt.taskID)
			require.Equal(t, tt.wantFound, found, "found 不匹配")
			if !tt.wantFound {
				return
		REDACTED
			require.NotNil(t, status)
			require.Equal(t, tt.taskID, status.ID)
			require.Equal(t, tt.wantStatus, status.Status)
			if tt.wantErr != "" {
				require.Equal(t, tt.wantErr, status.ErrorMsg)
		REDACTED
			if tt.wantURLs != nil {
				require.Equal(t, tt.wantURLs, status.URLs)
		REDACTED
	REDACTED)
REDACTED
REDACTED
