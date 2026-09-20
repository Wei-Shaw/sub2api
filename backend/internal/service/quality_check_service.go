package service

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const QualityGoosePrompt = "画一个小鹅骑自行车的 SVG。请给出完整、可直接显示的 SVG，画面中要能辨认出小鹅、两个车轮、车架和骑行姿势。只输出 SVG 代码，不要解释。不许联网，不要使用外部图片或资源。"
const QualityCandyPrompt = `在一个黑色的袋子里放有三种口味的糖果，每种糖果有两种不同的形状（圆形和五角星形，不同的形状靠手感可以分辨）。现已知不同口味的糖和不同形状的数量统计如下表。参赛者需要在活动前决定摸出的糖果数目，那么，最少取出多少个糖果才能保证手中同时拥有不同形状的苹果味和桃子味的糖？（同时手中有圆形苹果味匹配五角星桃子味糖果，或者有圆形桃子味匹配五角星苹果味糖果都满足要求）
              苹果味 桃子味 西瓜味
圆形            7      9      8
五角星形        7      6      4
不许联网，自己计算。请说明摸取策略和为什么不能更少，最后一行写“答案：N颗”（N为你算出的最少数量）。`

const qualityInterval = 10 * time.Minute
const qualityCaseTimeout = 150 * time.Second

// Leave legacy empty-prompt connectivity probes unchanged. Custom text probes
// carry the actual user's question, with no browsing tools supplied.
func applyOpenAITestPrompt(payload map[string]any, prompt string) {
	if strings.TrimSpace(prompt) == "" {
		return
	}
	payload["input"] = []map[string]any{{"role": "user", "content": []map[string]any{{"type": "input_text", "text": prompt}}}}
	payload["tools"] = []any{}
}

type QualityCase struct {
	Kind       string                    `json:"kind"`
	Status     string                    `json:"status"`
	Note       string                    `json:"note"`
	Text       string                    `json:"text"`
	Model      string                    `json:"model,omitempty"`
	Answer     *int                      `json:"answer,omitempty"`
	LatencyMS  int64                     `json:"latency_ms"`
	Characters int                       `json:"characters"`
	Ticket     *OpenAICodexTicketReceipt `json:"ticket,omitempty"`
}
type QualityRun struct {
	ID         int64         `json:"id"`
	AccountID  int64         `json:"account_id"`
	Model      string        `json:"model"`
	Status     string        `json:"status"`
	Cases      []QualityCase `json:"cases"`
	StartedAt  time.Time     `json:"started_at"`
	FinishedAt *time.Time    `json:"finished_at"`
}
type QualityConfig struct {
	Enabled         bool   `json:"enabled"`
	Model           string `json:"model"`
	IntervalSeconds int    `json:"interval_seconds"`
}
type QualityAccount struct {
	ID        int64       `json:"id"`
	Name      string      `json:"name"`
	Platform  string      `json:"platform"`
	Status    string      `json:"status"`
	Eligible  bool        `json:"eligible"`
	NextRunAt *time.Time  `json:"next_run_at"`
	Latest    *QualityRun `json:"latest"`
}
type QualityOverview struct {
	Config      QualityConfig    `json:"config"`
	Accounts    []QualityAccount `json:"accounts"`
	ServerTime  time.Time        `json:"server_time"`
	GoosePrompt string           `json:"goose_prompt"`
	CandyPrompt string           `json:"candy_prompt"`
}

// QualityCheckService uses persisted leases to prevent overlap across ticks,
// manual requests, restarts and multiple server instances. Each call is pinned
// to one account via AccountTestService; no gateway account selection occurs.
type QualityCheckService struct {
	db         *sql.DB
	test       *AccountTestService
	ctx        context.Context
	cancel     context.CancelFunc
	once       sync.Once
	workers    sync.WaitGroup
	slots      chan struct{}
	dispatchMu sync.Mutex
}

func NewQualityCheckService(db *sql.DB, test *AccountTestService) *QualityCheckService {
	ctx, cancel := context.WithCancel(context.Background())
	return &QualityCheckService{db: db, test: test, ctx: ctx, cancel: cancel, slots: make(chan struct{}, 4)}
}
func (s *AccountTestService) QualityChecks() *QualityCheckService { return s.qualityChecks }

func (s *QualityCheckService) Start() {
	if s == nil {
		return
	}
	s.once.Do(func() {
		s.workers.Add(1)
		go func() {
			defer s.workers.Done()
			ticker := time.NewTicker(5 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-s.ctx.Done():
					return
				case <-ticker.C:
					ctx, cancel := context.WithTimeout(s.ctx, 8*time.Second)
					_, err := s.Dispatch(ctx, false, 0)
					cancel()
					if err != nil && s.ctx.Err() == nil {
						slog.Warn("quality check scheduler", "error", err)
					}
				}
			}
		}()
	})
}
func (s *QualityCheckService) Stop() {
	if s == nil {
		return
	}
	s.cancel()
	s.dispatchMu.Lock()
	s.dispatchMu.Unlock()
	done := make(chan struct{})
	go func() { s.workers.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(4 * time.Second):
	}
}
func (s *QualityCheckService) Config(ctx context.Context) (QualityConfig, error) {
	c := QualityConfig{IntervalSeconds: 600}
	err := s.db.QueryRowContext(ctx, "SELECT enabled,model FROM account_quality_settings WHERE id=1").Scan(&c.Enabled, &c.Model)
	return c, err
}
func (s *QualityCheckService) Configure(ctx context.Context, c QualityConfig) error {
	c.Model = strings.TrimSpace(c.Model)
	if c.Model == "" || len(c.Model) > 200 || strings.ContainsAny(c.Model, "\r\n") || isOpenAIImageModel(c.Model) {
		return errors.New("请填写用于检查的文本模型名称")
	}
	_, err := s.db.ExecContext(ctx, "UPDATE account_quality_settings SET enabled=$1,model=$2,updated_at=NOW() WHERE id=1", c.Enabled, c.Model)
	return err
}
func (s *QualityCheckService) enroll(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO account_quality_schedule(account_id)
 SELECT id FROM accounts WHERE deleted_at IS NULL AND platform='openai'
 ON CONFLICT DO NOTHING`)
	return err
}

func (s *QualityCheckService) Dispatch(ctx context.Context, manual bool, accountID int64) (int, error) {
	s.dispatchMu.Lock()
	defer s.dispatchMu.Unlock()
	if s.ctx.Err() != nil {
		return 0, errors.New("检查服务正在停止")
	}
	cfg, err := s.Config(ctx)
	if err != nil {
		return 0, err
	}
	if !manual && !cfg.Enabled {
		return 0, nil
	}
	if err = s.enroll(ctx); err != nil {
		return 0, err
	}
	// Expired leases are the only runs that can be recovered after a crash;
	// healthy work on another instance is left untouched.
	_, err = s.db.ExecContext(ctx, `UPDATE account_quality_runs r SET status='interrupted',finished_at=NOW()
 FROM account_quality_schedule q WHERE r.account_id=q.account_id AND r.status='running' AND q.lease_until<NOW()`)
	if err != nil {
		return 0, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT a.id FROM accounts a JOIN account_quality_schedule q ON q.account_id=a.id
 WHERE a.deleted_at IS NULL AND a.platform='openai' AND a.status<>'disabled'
 AND ($1::bigint=0 OR a.id=$1) AND ($2::boolean OR q.next_run_at<=NOW())
 AND (q.lease_until IS NULL OR q.lease_until<NOW()) ORDER BY q.next_run_at,a.id LIMIT 200`, accountID, manual)
	if err != nil {
		return 0, err
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err = rows.Scan(&id); err != nil {
			_ = rows.Close()
			return 0, err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	if closeErr := rows.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return 0, err
	}
	launched := 0
	for _, id := range ids {
		select {
		case s.slots <- struct{}{}:
		default:
			return launched, nil
		}
		token := uuid.NewString()
		tx, e := s.db.BeginTx(ctx, nil)
		if e != nil {
			<-s.slots
			return launched, e
		}
		claimed, e := tx.ExecContext(ctx, `UPDATE account_quality_schedule SET lease_token=$2,lease_until=NOW()+INTERVAL '6 minutes',next_run_at=NOW()+INTERVAL '10 minutes'
   WHERE account_id=$1 AND (lease_until IS NULL OR lease_until<NOW()) AND ($3::boolean OR next_run_at<=NOW())`, id, token, manual)
		if e != nil {
			_ = tx.Rollback()
			<-s.slots
			return launched, e
		}
		n, e := claimed.RowsAffected()
		if e != nil || n == 0 {
			_ = tx.Rollback()
			<-s.slots
			continue
		}
		run := QualityRun{AccountID: id, Model: cfg.Model, Status: "running", Cases: []QualityCase{}}
		e = tx.QueryRowContext(ctx, "INSERT INTO account_quality_runs(account_id,model) VALUES ($1,$2) RETURNING id,started_at", id, cfg.Model).Scan(&run.ID, &run.StartedAt)
		if e != nil {
			_ = tx.Rollback()
			<-s.slots
			return launched, e
		}
		if e = tx.Commit(); e != nil {
			<-s.slots
			return launched, e
		}
		launched++
		s.workers.Add(1)
		go func(r QualityRun, lease string) {
			defer s.workers.Done()
			defer func() { <-s.slots }()
			s.run(r, lease)
		}(run, token)
	}
	return launched, nil
}

func (s *QualityCheckService) run(run QualityRun, token string) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("quality check panic", "account_id", run.AccountID)
			run.Status = "interrupted"
		}
		if run.Status == "running" {
			run.Status = "completed"
		}
		if s.ctx.Err() != nil {
			run.Status = "interrupted"
		}
		now := time.Now()
		run.FinishedAt = &now
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.save(ctx, run); err != nil {
			slog.Error("quality check save failed", "account_id", run.AccountID, "error", err)
		}
		_, _ = s.db.ExecContext(ctx, "UPDATE account_quality_schedule SET lease_until=NULL,lease_token=NULL WHERE account_id=$1 AND lease_token=$2", run.AccountID, token)
		_, _ = s.db.ExecContext(ctx, `DELETE FROM account_quality_runs WHERE account_id=$1 AND status<>'running' AND id NOT IN
   (SELECT id FROM account_quality_runs WHERE account_id=$1 ORDER BY id DESC LIMIT 36)`, run.AccountID)
	}()
	for _, task := range []struct{ kind, prompt string }{{"goose_svg", QualityGoosePrompt}, {"candy", QualityCandyPrompt}} {
		if s.ctx.Err() != nil {
			return
		}
		ctx, cancel := context.WithTimeout(s.ctx, qualityCaseTimeout)
		result := s.test.runQualityPrompt(ctx, run.AccountID, run.Model, task.kind, task.prompt)
		cancel()
		run.Cases = append(run.Cases, result)
		ctx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
		err := s.save(ctx, run)
		cancel()
		if err != nil {
			slog.Warn("quality check partial save failed", "account_id", run.AccountID, "error", err)
		}
	}
}
func (s *QualityCheckService) save(ctx context.Context, r QualityRun) error {
	data, err := json.Marshal(r.Cases)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, "UPDATE account_quality_runs SET status=$2,cases=$3,finished_at=$4 WHERE id=$1", r.ID, r.Status, string(data), r.FinishedAt)
	return err
}
func (s *QualityCheckService) Overview(ctx context.Context) (*QualityOverview, error) {
	cfg, err := s.Config(ctx)
	if err != nil {
		return nil, err
	}
	out := &QualityOverview{Config: cfg, Accounts: []QualityAccount{}, ServerTime: time.Now(), GoosePrompt: QualityGoosePrompt, CandyPrompt: QualityCandyPrompt}
	rows, err := s.db.QueryContext(ctx, `SELECT a.id,a.name,a.platform,a.status,q.next_run_at,COALESCE(to_jsonb(r),'null'::jsonb)
 FROM accounts a LEFT JOIN account_quality_schedule q ON q.account_id=a.id
 LEFT JOIN LATERAL (SELECT id,account_id,model,status,cases,started_at,finished_at FROM account_quality_runs WHERE account_id=a.id ORDER BY id DESC LIMIT 1) r ON TRUE
 WHERE a.deleted_at IS NULL ORDER BY a.id`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var a QualityAccount
		var raw []byte
		if err = rows.Scan(&a.ID, &a.Name, &a.Platform, &a.Status, &a.NextRunAt, &raw); err != nil {
			return nil, err
		}
		a.Eligible = a.Platform == "openai" && a.Status != "disabled"
		if err = json.Unmarshal(raw, &a.Latest); err != nil {
			return nil, err
		}
		out.Accounts = append(out.Accounts, a)
	}
	return out, rows.Err()
}
func (s *QualityCheckService) History(ctx context.Context, id int64) ([]QualityRun, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,account_id,model,status,started_at,finished_at FROM account_quality_runs WHERE account_id=$1 ORDER BY id DESC LIMIT 36`, id)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []QualityRun{}
	for rows.Next() {
		var r QualityRun
		r.Cases = []QualityCase{}
		if err = rows.Scan(&r.ID, &r.AccountID, &r.Model, &r.Status, &r.StartedAt, &r.FinishedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
func (s *QualityCheckService) GetRun(ctx context.Context, id int64) (*QualityRun, error) {
	var r QualityRun
	var raw []byte
	err := s.db.QueryRowContext(ctx, "SELECT id,account_id,model,status,cases,started_at,finished_at FROM account_quality_runs WHERE id=$1", id).Scan(&r.ID, &r.AccountID, &r.Model, &r.Status, &raw, &r.StartedAt, &r.FinishedAt)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(raw, &r.Cases)
	return &r, err
}

// A capped recorder avoids retaining unbounded streamed model output.
type qualityRecorder struct {
	header   http.Header
	body     bytes.Buffer
	cancel   context.CancelFunc
	overflow bool
}

func (w *qualityRecorder) Header() http.Header { return w.header }
func (w *qualityRecorder) WriteHeader(int)     {}
func (w *qualityRecorder) Flush()              {}
func (w *qualityRecorder) Write(p []byte) (int, error) {
	if w.body.Len()+len(p) > 2<<20 {
		w.overflow = true
		w.cancel()
		return 0, errors.New("检查输出超过限制")
	}
	return w.body.Write(p)
}

func (s *AccountTestService) runQualityPrompt(parent context.Context, accountID int64, model, kind, prompt string) QualityCase {
	start := time.Now()
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	receipt := &OpenAICodexTicketReceipt{Status: "unverified"}
	ctx = context.WithValue(ctx, qualityTicketReceiptKey{}, receipt)
	w := &qualityRecorder{header: make(http.Header), cancel: cancel}
	c, _ := gin.CreateTestContext(w)
	c.Request = (&http.Request{Header: make(http.Header)}).WithContext(ctx)
	err := s.TestAccountConnection(c, accountID, model, prompt, AccountTestModeDefault)
	result := parseQualityOutput(w.body.String(), kind)
	result.Ticket = receipt
	result.LatencyMS = time.Since(start).Milliseconds()
	if err != nil || w.overflow || parent.Err() != nil {
		result.Status = "error"
		if parent.Err() != nil {
			result.Note = "检查超时或服务停止，本次不能判断回答质量"
		} else if w.overflow {
			result.Note = "输出超过记录限制"
		} else if result.Note == "" || result.Status == "error" {
			result.Note = qualityClip(err.Error(), 800)
		}
	}
	if qualityTicketBlocked(receipt.Status) {
		result.Status = "waiting_ticket"
		result.Note = "当前账号和模型没有有效票据，本次未请求模型；同步到有效票据后在下轮检查"
	}
	return result
}
func qualityClip(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}

func parseQualityOutput(body, kind string) QualityCase {
	r := QualityCase{Kind: kind, Status: "error"}
	var text strings.Builder
	complete := false
	failure := ""
	for _, line := range strings.Split(body, "\n") {
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		var e TestEvent
		if json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(line, "data:"))), &e) != nil {
			continue
		}
		switch e.Type {
		case "test_start":
			r.Model = e.Model
		case "content":
			text.WriteString(e.Text)
		case "error":
			failure = e.Error
		case "test_complete":
			complete = e.Success
		}
	}
	r.Text = text.String()
	r.Characters = utf8.RuneCountInString(r.Text)
	if len(r.Text) > 128<<10 {
		r.Text = qualityClip(r.Text, 32000)
		r.Note = "输出过长，记录已截断"
		return r
	}
	if failure != "" {
		r.Note = qualityClip(failure, 800)
		return r
	}
	if !complete {
		r.Note = "未收到成功完成事件"
		return r
	}
	if strings.TrimSpace(r.Text) == "" {
		r.Note = "模型未输出内容"
		return r
	}
	if kind == "goose_svg" {
		if qualitySVGValid(r.Text) {
			r.Status = "review"
			r.Note = "SVG 结构完整 · 点击目测小鹅与骑行姿势"
		} else {
			r.Status = "mismatch"
			r.Note = "未生成完整、有效的 SVG"
		}
		return r
	}
	answer := qualityCandyAnswer(r.Text)
	r.Answer = answer
	if answer == nil {
		r.Status = "review"
		r.Note = "未识别最终答案，请展开查看推导"
	} else if *answer == 21 {
		r.Status = "pass"
		r.Note = "答案 21 颗符合基准 · 推导可展开核验"
	} else if *answer == 29 {
		r.Status = "mismatch"
		r.Note = "29 是盲取答案；题目允许用手感分辨形状，需复核"
	} else {
		r.Status = "mismatch"
		r.Note = fmt.Sprintf("答案 %d 颗与基准 21 颗不一致，需复核", *answer)
	}
	return r
}

var qualityAnswerPattern = regexp.MustCompile(`(?m)(?:答案|最少(?:需要)?|至少(?:需要)?)[^0-9\n]{0,18}([0-9]{1,3})\s*(?:颗|个)?`)

func qualityCandyAnswer(text string) *int {
	matches := qualityAnswerPattern.FindAllStringSubmatch(text, -1)
	if len(matches) == 0 {
		return nil
	}
	var n int
	if _, err := fmt.Sscan(matches[len(matches)-1][1], &n); err != nil {
		return nil
	}
	return &n
}
func qualitySVGValid(text string) bool {
	lower := strings.ToLower(text)
	begin := strings.Index(lower, "<svg")
	end := strings.LastIndex(lower, "</svg>")
	if begin < 0 || end < begin {
		return false
	}
	decoder := xml.NewDecoder(strings.NewReader(text[begin : end+6]))
	depth, shapes := 0, 0
	root := false
	for {
		tok, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return false
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if depth == 0 {
				if root || t.Name.Local != "svg" {
					return false
				}
				root = true
			}
			depth++
			switch t.Name.Local {
			case "path", "circle", "ellipse", "rect", "polygon", "polyline", "line":
				shapes++
			}
		case xml.EndElement:
			depth--
		}
	}
	return root && depth == 0 && shapes >= 3
}
