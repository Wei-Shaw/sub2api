package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"net/mail"
	"net/textproto"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/google/uuid"
)

const (
	NotificationEmailDeliveryStatusPending    = "pending"
	NotificationEmailDeliveryStatusProcessing = "processing"
	NotificationEmailDeliveryStatusRetryWait  = "retry_wait"
	NotificationEmailDeliveryStatusSent       = "sent"
	NotificationEmailDeliveryStatusFailed     = "failed"
	NotificationEmailDeliveryStatusSuppressed = "suppressed"

	notificationEmailDeliveryBatchSize       = 4
	notificationEmailDeliveryPollInterval    = time.Second
	notificationEmailDeliveryLease           = 2 * time.Minute
	notificationEmailDeliverySendTimeout     = 30 * time.Second
	notificationEmailDeliveryDBTimeout       = 5 * time.Second
	notificationEmailDeliveryConcurrency     = 4
	notificationEmailDeliveryMaxAttempts     = 5
	notificationEmailDeliveryCleanupEvery    = 6 * time.Hour
	notificationEmailDeliverySentRetention   = 30 * 24 * time.Hour
	notificationEmailDeliveryFailedRetention = 90 * 24 * time.Hour
	notificationEmailDeliveryCleanupBatch    = 1000
	notificationEmailVariablesMaxBytes       = 64 * 1024
	notificationEmailRawHTMLMaxBytes         = 512 * 1024
)

var notificationEmailAddressInErrorPattern = regexp.MustCompile(`(?i)[a-z0-9.!#$%&'*+/=?^_` + "`" + `{|}~-]+@[a-z0-9.-]+\.[a-z]{2,}`)

// NotificationEmailDelivery is the durable event envelope shared by business
// notifications today and Ops incidents in the next iteration.
type NotificationEmailDelivery struct {
	ID                           int64
	DedupKey                     string
	Event                        string
	Channel                      string
	RecipientEmail               string
	RecipientHash                string
	RecipientName                string
	UserID                       int64
	SourceType                   string
	SourceID                     string
	ReminderKey                  string
	Locale                       string
	Variables                    map[string]string
	RawHTMLVariables             map[string]string
	SensitiveVariablesCiphertext string
	Status                       string
	AttemptCount                 int
	MaxAttempts                  int
	NextAttemptAt                time.Time
	LastErrorCategory            string
	LastError                    string
	CreatedAt                    time.Time
	UpdatedAt                    time.Time
	SentAt                       *time.Time
}

type NotificationEmailDeliveryListFilter struct {
	Page          int
	PageSize      int
	Event         string
	Status        string
	SourceType    string
	SourceID      string
	RecipientHash string
	ReminderKey   string
	CreatedAfter  *time.Time
}

type NotificationEmailDeliveryListResult struct {
	Items []NotificationEmailDelivery
	Total int64
}

type NotificationEmailDeliveryRepository interface {
	Enqueue(ctx context.Context, delivery NotificationEmailDelivery) (id int64, created bool, err error)
	Claim(ctx context.Context, workerID string, limit int, lease time.Duration) ([]NotificationEmailDelivery, error)
	MarkSent(ctx context.Context, id int64, workerID string) error
	MarkSuppressed(ctx context.Context, id int64, workerID, category, detail string) error
	MarkFailed(ctx context.Context, id int64, workerID, category, detail string, retryAt *time.Time) error
	List(ctx context.Context, filter NotificationEmailDeliveryListFilter) (NotificationEmailDeliveryListResult, error)
	Retry(ctx context.Context, id int64) (bool, error)
	Stats(ctx context.Context) (NotificationEmailDeliveryStats, error)
	DeleteTerminalBefore(ctx context.Context, successfulBefore, failedBefore time.Time, limit int) (int64, error)
}

type NotificationEmailEnqueueResult struct {
	ID      int64 `json:"id"`
	Created bool  `json:"created"`
}

type NotificationEmailDeliveryView struct {
	ID                int64      `json:"id"`
	Event             string     `json:"event"`
	Channel           string     `json:"channel"`
	Recipient         string     `json:"recipient"`
	SourceType        string     `json:"source_type"`
	SourceID          string     `json:"source_id"`
	Status            string     `json:"status"`
	AttemptCount      int        `json:"attempt_count"`
	MaxAttempts       int        `json:"max_attempts"`
	NextAttemptAt     time.Time  `json:"next_attempt_at"`
	LastErrorCategory string     `json:"last_error_category,omitempty"`
	LastError         string     `json:"last_error,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
	SentAt            *time.Time `json:"sent_at,omitempty"`
}

type NotificationEmailDeliveryPage struct {
	Items    []NotificationEmailDeliveryView `json:"items"`
	Total    int64                           `json:"total"`
	Page     int                             `json:"page"`
	PageSize int                             `json:"page_size"`
}

type NotificationEmailDeliveryStats struct {
	Pending         int64
	OldestCreatedAt *time.Time
	MaxAttempts     int
	LastError       string
}

type NotificationEmailDeliveryHealth struct {
	Running     bool          `json:"running"`
	Processed   uint64        `json:"processed"`
	Failures    uint64        `json:"failures"`
	Pending     int64         `json:"pending"`
	OldestLag   time.Duration `json:"oldest_lag"`
	LastError   string        `json:"last_error,omitempty"`
	StatsError  string        `json:"stats_error,omitempty"`
	MaxAttempts int           `json:"max_attempts"`
}

// NotificationEmailDispatcher is the single async delivery boundary. Business
// and Ops code submit stable events; only the worker owns SMTP delivery.
type NotificationEmailDispatcher struct {
	repo         NotificationEmailDeliveryRepository
	emailService *NotificationEmailService
	encryptor    SecretEncryptor
}

func NewNotificationEmailDispatcher(repo NotificationEmailDeliveryRepository, emailService *NotificationEmailService) *NotificationEmailDispatcher {
	return &NotificationEmailDispatcher{repo: repo, emailService: emailService}
}

func ProvideNotificationEmailDispatcher(repo NotificationEmailDeliveryRepository, emailService *NotificationEmailService, encryptor SecretEncryptor, cfg *config.Config) *NotificationEmailDispatcher {
	dispatcher := NewNotificationEmailDispatcher(repo, emailService)
	if cfg != nil && cfg.Totp.EncryptionKeyConfigured {
		dispatcher.encryptor = encryptor
	}
	return dispatcher
}

func (d *NotificationEmailDispatcher) Enqueue(ctx context.Context, input NotificationEmailSendInput) (NotificationEmailEnqueueResult, error) {
	if d == nil || d.repo == nil || d.emailService == nil {
		return NotificationEmailEnqueueResult{}, errors.New("notification email dispatcher is not configured")
	}
	_, event, err := d.emailService.eventInfo(input.Event)
	if err != nil {
		return NotificationEmailEnqueueResult{}, err
	}
	channel, ok := notificationEmailEventChannels[event]
	if !ok {
		return NotificationEmailEnqueueResult{}, fmt.Errorf("notification email event has no channel policy: %s", event)
	}
	if strings.TrimSpace(input.SourceType) == "" || strings.TrimSpace(input.SourceID) == "" {
		return NotificationEmailEnqueueResult{}, errors.New("notification email source_type and source_id are required")
	}
	if len(strings.TrimSpace(input.SourceType)) > 100 || len(strings.TrimSpace(input.SourceID)) > 200 || len(strings.TrimSpace(input.ReminderKey)) > 200 {
		return NotificationEmailEnqueueResult{}, errors.New("notification email source identity is too long")
	}
	if len(strings.TrimSpace(input.Locale)) > 16 || len(strings.TrimSpace(input.RecipientName)) > 200 {
		return NotificationEmailEnqueueResult{}, errors.New("notification email recipient metadata is too long")
	}
	recipient := strings.ToLower(strings.TrimSpace(input.RecipientEmail))
	if recipient == "" {
		return NotificationEmailEnqueueResult{}, errors.New("notification email recipient is required")
	}
	parsed, err := mail.ParseAddress(recipient)
	if err != nil || !strings.EqualFold(strings.TrimSpace(parsed.Address), recipient) {
		return NotificationEmailEnqueueResult{}, errors.New("notification email recipient is invalid")
	}
	if len(recipient) > 320 {
		return NotificationEmailEnqueueResult{}, errors.New("notification email recipient is too long")
	}
	if err := validateNotificationEmailQueuePayload(event, input.Variables, input.SensitiveVariables, input.RawHTMLVariables); err != nil {
		return NotificationEmailEnqueueResult{}, err
	}
	if err := d.emailService.requireEventChannelEnabled(ctx, event); err != nil {
		return NotificationEmailEnqueueResult{}, err
	}

	var sensitiveCiphertext string
	if len(input.SensitiveVariables) > 0 {
		if !notificationEmailAllowsSensitiveVariables(event) {
			return NotificationEmailEnqueueResult{}, fmt.Errorf("notification email event does not allow sensitive variables: %s", event)
		}
		if d.encryptor == nil {
			return NotificationEmailEnqueueResult{}, errors.New("notification email sensitive payload encryption is not configured")
		}
		encoded, encodeErr := json.Marshal(input.SensitiveVariables)
		if encodeErr != nil {
			return NotificationEmailEnqueueResult{}, fmt.Errorf("encode notification email sensitive variables: %w", encodeErr)
		}
		sensitiveCiphertext, err = d.encryptor.Encrypt(string(encoded))
		if err != nil {
			return NotificationEmailEnqueueResult{}, fmt.Errorf("encrypt notification email sensitive variables: %w", err)
		}
	}

	delivery := NotificationEmailDelivery{
		DedupKey:                     notificationEmailQueueDedupKey(event, input.SourceType, input.SourceID, recipient, input.ReminderKey),
		Event:                        event,
		Channel:                      channel,
		RecipientEmail:               recipient,
		RecipientHash:                notificationEmailHash(recipient),
		RecipientName:                strings.TrimSpace(input.RecipientName),
		UserID:                       input.UserID,
		SourceType:                   strings.TrimSpace(input.SourceType),
		SourceID:                     strings.TrimSpace(input.SourceID),
		ReminderKey:                  strings.TrimSpace(input.ReminderKey),
		Locale:                       strings.TrimSpace(input.Locale),
		Variables:                    cloneNotificationEmailVariables(input.Variables),
		SensitiveVariablesCiphertext: sensitiveCiphertext,
		RawHTMLVariables:             cloneNotificationEmailVariables(input.RawHTMLVariables),
		Status:                       NotificationEmailDeliveryStatusPending,
		MaxAttempts:                  notificationEmailDeliveryMaxAttempts,
	}
	id, created, err := d.repo.Enqueue(ctx, delivery)
	return NotificationEmailEnqueueResult{ID: id, Created: created}, err
}

func notificationEmailAllowsSensitiveVariables(event string) bool {
	switch strings.TrimSpace(event) {
	case NotificationEmailEventAuthVerifyCode,
		NotificationEmailEventAuthPasswordReset,
		NotificationEmailEventNotificationEmailVerifyCode:
		return true
	default:
		return false
	}
}

// EnqueueGroup resolves the configured group before creating one durable row
// per recipient. It is the intended entry point for Ops incidents and reports.
func (d *NotificationEmailDispatcher) EnqueueGroup(ctx context.Context, input NotificationEmailSendInput) ([]NotificationEmailEnqueueResult, error) {
	if d == nil || d.emailService == nil {
		return nil, errors.New("notification email dispatcher is not configured")
	}
	_, event, err := d.emailService.eventInfo(input.Event)
	if err != nil {
		return nil, err
	}
	channel, ok := notificationEmailEventChannels[event]
	if !ok {
		return nil, fmt.Errorf("notification email event has no channel policy: %s", event)
	}
	recipients, err := d.emailService.ResolveGroupRecipients(ctx, channel)
	if err != nil {
		return nil, err
	}
	results := make([]NotificationEmailEnqueueResult, 0, len(recipients))
	for _, recipient := range recipients {
		item := input
		item.RecipientEmail = recipient
		item.RecipientName = emailRecipientName(recipient)
		result, enqueueErr := d.Enqueue(ctx, item)
		if enqueueErr != nil {
			return results, enqueueErr
		}
		results = append(results, result)
	}
	return results, nil
}

func (d *NotificationEmailDispatcher) List(ctx context.Context, filter NotificationEmailDeliveryListFilter) (NotificationEmailDeliveryPage, error) {
	if d == nil || d.repo == nil {
		return NotificationEmailDeliveryPage{}, errors.New("notification email dispatcher is not configured")
	}
	if filter.Page < 1 {
		filter.Page = 1
	}
	if filter.PageSize < 1 {
		filter.PageSize = 20
	}
	if filter.PageSize > 100 {
		filter.PageSize = 100
	}
	result, err := d.repo.List(ctx, filter)
	if err != nil {
		return NotificationEmailDeliveryPage{}, err
	}
	items := make([]NotificationEmailDeliveryView, 0, len(result.Items))
	for _, item := range result.Items {
		items = append(items, NotificationEmailDeliveryView{
			ID: item.ID, Event: item.Event, Channel: item.Channel,
			Recipient: maskNotificationEmail(item.RecipientEmail), SourceType: item.SourceType,
			SourceID: item.SourceID, Status: item.Status, AttemptCount: item.AttemptCount,
			MaxAttempts: item.MaxAttempts, NextAttemptAt: item.NextAttemptAt,
			LastErrorCategory: item.LastErrorCategory, LastError: item.LastError,
			CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt, SentAt: item.SentAt,
		})
	}
	return NotificationEmailDeliveryPage{Items: items, Total: result.Total, Page: filter.Page, PageSize: filter.PageSize}, nil
}

func (d *NotificationEmailDispatcher) Retry(ctx context.Context, id int64) (bool, error) {
	if d == nil || d.repo == nil {
		return false, errors.New("notification email dispatcher is not configured")
	}
	if id <= 0 {
		return false, errors.New("invalid notification email delivery id")
	}
	return d.repo.Retry(ctx, id)
}

type NotificationEmailDeliveryWorker struct {
	repo      NotificationEmailDeliveryRepository
	deliver   func(context.Context, NotificationEmailSendInput, bool) (bool, error)
	workerID  string
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	start     sync.Once
	stop      sync.Once
	running   atomic.Bool
	processed atomic.Uint64
	failures  atomic.Uint64
	lastError atomic.Value
	encryptor SecretEncryptor
}

func NewNotificationEmailDeliveryWorker(repo NotificationEmailDeliveryRepository, email *NotificationEmailService) *NotificationEmailDeliveryWorker {
	ctx, cancel := context.WithCancel(context.Background())
	var deliver func(context.Context, NotificationEmailSendInput, bool) (bool, error)
	if email != nil {
		deliver = email.deliver
	}
	w := &NotificationEmailDeliveryWorker{repo: repo, deliver: deliver, workerID: uuid.NewString(), ctx: ctx, cancel: cancel}
	w.lastError.Store("")
	return w
}

func (w *NotificationEmailDeliveryWorker) SetEncryptor(encryptor SecretEncryptor) {
	if w != nil {
		w.encryptor = encryptor
	}
}

func (w *NotificationEmailDeliveryWorker) Start() {
	if w == nil || w.repo == nil || w.deliver == nil {
		return
	}
	w.start.Do(func() {
		w.running.Store(true)
		w.wg.Add(1)
		go w.run()
	})
}

func (w *NotificationEmailDeliveryWorker) Stop() {
	if w == nil {
		return
	}
	w.stop.Do(func() {
		w.cancel()
		w.wg.Wait()
		w.running.Store(false)
	})
}

func (w *NotificationEmailDeliveryWorker) run() {
	defer w.wg.Done()
	defer w.running.Store(false)
	ticker := time.NewTicker(notificationEmailDeliveryPollInterval)
	defer ticker.Stop()
	cleanupTicker := time.NewTicker(notificationEmailDeliveryCleanupEvery)
	defer cleanupTicker.Stop()
	for {
		if err := w.processBatch(w.ctx); err != nil && w.ctx.Err() == nil {
			w.recordFailure(err)
		}
		select {
		case <-w.ctx.Done():
			return
		case <-ticker.C:
		case <-cleanupTicker.C:
			if _, err := w.cleanupTerminalDeliveries(w.ctx, time.Now().UTC()); err != nil && w.ctx.Err() == nil {
				w.recordFailure(err)
			}
		}
	}
}

func (w *NotificationEmailDeliveryWorker) processBatch(ctx context.Context) error {
	deliveries, err := w.repo.Claim(ctx, w.workerID, notificationEmailDeliveryBatchSize, notificationEmailDeliveryLease)
	if err != nil {
		return fmt.Errorf("claim notification emails: %w", err)
	}
	semaphore := make(chan struct{}, notificationEmailDeliveryConcurrency)
	var wg sync.WaitGroup
	for i := range deliveries {
		select {
		case <-ctx.Done():
			wg.Wait()
			return ctx.Err()
		case semaphore <- struct{}{}:
		}
		wg.Add(1)
		go func(delivery NotificationEmailDelivery) {
			defer wg.Done()
			defer func() { <-semaphore }()
			w.processDelivery(delivery)
		}(deliveries[i])
	}
	wg.Wait()
	return nil
}

func (w *NotificationEmailDeliveryWorker) processDelivery(delivery NotificationEmailDelivery) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err := fmt.Errorf("panic while delivering notification email: %v", recovered)
			w.recordFailure(err)
			w.releaseFailedDelivery(delivery, "internal", err, true)
		}
	}()

	variables := cloneNotificationEmailVariables(delivery.Variables)
	if delivery.SensitiveVariablesCiphertext != "" {
		if w.encryptor == nil {
			err := errors.New("notification email sensitive payload decryption is not configured")
			w.recordFailure(err)
			w.releaseFailedDelivery(delivery, "configuration", err, true)
			return
		}
		plaintext, err := w.encryptor.Decrypt(delivery.SensitiveVariablesCiphertext)
		if err != nil {
			err = fmt.Errorf("decrypt notification email sensitive variables: %w", err)
			w.recordFailure(err)
			w.releaseFailedDelivery(delivery, "configuration", err, false)
			return
		}
		var sensitive map[string]string
		if err := json.Unmarshal([]byte(plaintext), &sensitive); err != nil {
			err = fmt.Errorf("decode notification email sensitive variables: %w", err)
			w.recordFailure(err)
			w.releaseFailedDelivery(delivery, "internal", err, false)
			return
		}
		for key, value := range sensitive {
			variables[key] = value
		}
	}

	ctx, cancel := context.WithTimeout(w.ctx, notificationEmailDeliverySendTimeout)
	suppressed, err := w.deliver(ctx, NotificationEmailSendInput{
		Event: delivery.Event, Locale: delivery.Locale, RecipientEmail: delivery.RecipientEmail,
		RecipientName: delivery.RecipientName, UserID: delivery.UserID, SourceType: delivery.SourceType,
		SourceID: delivery.SourceID, ReminderKey: delivery.ReminderKey,
		Variables: variables, RawHTMLVariables: delivery.RawHTMLVariables,
	}, false)
	cancel()

	if err != nil {
		if errors.Is(err, ErrNotificationEmailChannelDisabled) {
			ctx, cancel = context.WithTimeout(context.Background(), notificationEmailDeliveryDBTimeout)
			err = w.repo.MarkSuppressed(ctx, delivery.ID, w.workerID, "policy", "suppressed because the channel is disabled")
			cancel()
			if err != nil {
				w.recordFailure(fmt.Errorf("suppress notification email %d: %w", delivery.ID, err))
			}
			return
		}
		category, retryable := notificationEmailFailureCategory(err)
		w.recordFailure(err)
		w.releaseFailedDelivery(delivery, category, err, retryable)
		return
	}
	ctx, cancel = context.WithTimeout(context.Background(), notificationEmailDeliveryDBTimeout)
	if suppressed {
		err = w.repo.MarkSuppressed(ctx, delivery.ID, w.workerID, "policy", "suppressed by current channel or recipient preference")
	} else {
		err = w.repo.MarkSent(ctx, delivery.ID, w.workerID)
	}
	cancel()
	if err != nil {
		w.recordFailure(fmt.Errorf("ack notification email %d: %w", delivery.ID, err))
		return
	}
	w.processed.Add(1)
	w.lastError.Store("")
}

func (w *NotificationEmailDeliveryWorker) cleanupTerminalDeliveries(ctx context.Context, now time.Time) (int64, error) {
	if w == nil || w.repo == nil {
		return 0, nil
	}
	var total int64
	for {
		deleted, err := w.repo.DeleteTerminalBefore(
			ctx,
			now.Add(-notificationEmailDeliverySentRetention),
			now.Add(-notificationEmailDeliveryFailedRetention),
			notificationEmailDeliveryCleanupBatch,
		)
		if err != nil {
			return total, fmt.Errorf("clean notification email deliveries: %w", err)
		}
		total += deleted
		if deleted < notificationEmailDeliveryCleanupBatch {
			return total, nil
		}
	}
}

func (w *NotificationEmailDeliveryWorker) releaseFailedDelivery(delivery NotificationEmailDelivery, category string, deliveryErr error, retryable bool) {
	var retryAt *time.Time
	if retryable && delivery.AttemptCount < delivery.MaxAttempts {
		value := time.Now().UTC().Add(notificationEmailRetryDelay(delivery.AttemptCount))
		retryAt = &value
	}
	ctx, cancel := context.WithTimeout(context.Background(), notificationEmailDeliveryDBTimeout)
	err := w.repo.MarkFailed(ctx, delivery.ID, w.workerID, category, boundedNotificationEmailError(deliveryErr), retryAt)
	cancel()
	if err != nil {
		w.recordFailure(fmt.Errorf("release notification email %d: %w", delivery.ID, err))
	}
}

func (w *NotificationEmailDeliveryWorker) Health(ctx context.Context) NotificationEmailDeliveryHealth {
	health := NotificationEmailDeliveryHealth{}
	if w == nil {
		return health
	}
	health.Running = w.running.Load()
	health.Processed = w.processed.Load()
	health.Failures = w.failures.Load()
	if value := w.lastError.Load(); value != nil {
		health.LastError, _ = value.(string)
	}
	if w.repo == nil {
		return health
	}
	stats, err := w.repo.Stats(ctx)
	if err != nil {
		health.StatsError = boundedNotificationEmailError(err)
		return health
	}
	health.Pending = stats.Pending
	health.MaxAttempts = stats.MaxAttempts
	if health.LastError == "" {
		health.LastError = stats.LastError
	}
	if stats.OldestCreatedAt != nil {
		health.OldestLag = time.Since(*stats.OldestCreatedAt)
		if health.OldestLag < 0 {
			health.OldestLag = 0
		}
	}
	return health
}

// GetNotificationEmailDeliveryHealth is the stable Ops boundary for the next
// alerting/dashboard iteration. It intentionally exposes aggregate health only.
func (s *OpsService) GetNotificationEmailDeliveryHealth(ctx context.Context) NotificationEmailDeliveryHealth {
	if s == nil || s.notificationEmailWorker == nil {
		return NotificationEmailDeliveryHealth{}
	}
	return s.notificationEmailWorker.Health(ctx)
}

func (s *OpsService) SetNotificationEmailDeliveryWorker(worker *NotificationEmailDeliveryWorker) {
	if s != nil {
		s.notificationEmailWorker = worker
	}
}

func (w *NotificationEmailDeliveryWorker) recordFailure(err error) {
	if err == nil {
		return
	}
	w.failures.Add(1)
	w.lastError.Store(boundedNotificationEmailError(err))
	slog.Warn("notification email delivery failed", "error", err)
}

func ProvideNotificationEmailDeliveryWorker(repo NotificationEmailDeliveryRepository, email *NotificationEmailService, encryptor SecretEncryptor, cfg *config.Config) *NotificationEmailDeliveryWorker {
	worker := NewNotificationEmailDeliveryWorker(repo, email)
	if cfg != nil && cfg.Totp.EncryptionKeyConfigured {
		worker.SetEncryptor(encryptor)
	}
	startProcessBackground("notification_email_delivery", worker.Start)
	return worker
}

func notificationEmailQueueDedupKey(event, sourceType, sourceID, recipient, reminderKey string) string {
	identity := strings.Join([]string{
		strings.ToLower(strings.TrimSpace(event)),
		strings.ToLower(strings.TrimSpace(sourceType)),
		strings.TrimSpace(sourceID),
		strings.ToLower(strings.TrimSpace(recipient)),
		strings.TrimSpace(reminderKey),
	}, "\x00")
	sum := sha256.Sum256([]byte(identity))
	return hex.EncodeToString(sum[:])
}

func notificationEmailRetryDelay(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	if attempt > 8 {
		attempt = 8
	}
	base := 15 * time.Second * time.Duration(1<<(attempt-1))
	return time.Duration(float64(base) * (0.8 + rand.Float64()*0.4))
}

func notificationEmailFailureCategory(err error) (string, bool) {
	if err == nil {
		return "", false
	}
	if errors.Is(err, ErrNotificationEmailChannelDisabled) {
		return "policy", false
	}
	if isNotificationEmailDeliveryError(err) {
		var smtpErr *textproto.Error
		if errors.As(err, &smtpErr) && smtpErr.Code >= 500 && smtpErr.Code < 600 {
			return "transport_permanent", false
		}
		return "transport", true
	}
	var templateErr notificationEmailTemplateError
	if errors.As(err, &templateErr) {
		return "template", false
	}
	var configErr notificationEmailConfigError
	if errors.As(err, &configErr) {
		return "configuration", false
	}
	return "internal", true
}

func boundedNotificationEmailError(err error) string {
	if err == nil {
		return ""
	}
	message := strings.Join(strings.Fields(err.Error()), " ")
	message = notificationEmailAddressInErrorPattern.ReplaceAllString(message, "***@***")
	message = strings.ToValidUTF8(message, "\uFFFD")
	if len(message) > 512 {
		message = strings.ToValidUTF8(message[:512], "")
	}
	return message
}

func validateNotificationEmailQueuePayload(event string, variables, sensitiveVariables, rawHTMLVariables map[string]string) error {
	allowed := notificationEmailAllowedPlaceholderSet(event)
	variableBytes := 0
	for key, value := range variables {
		if _, ok := allowed[key]; !ok {
			return fmt.Errorf("unsupported notification email variable for %s: %s", event, key)
		}
		variableBytes += len(key) + len(value)
		if variableBytes > notificationEmailVariablesMaxBytes {
			return errors.New("notification email variables exceed size limit")
		}
	}
	for key, value := range sensitiveVariables {
		if _, exists := variables[key]; exists {
			return fmt.Errorf("notification email variable %s cannot be both public and sensitive", key)
		}
		if _, ok := allowed[key]; !ok {
			return fmt.Errorf("unsupported sensitive notification email variable for %s: %s", event, key)
		}
		variableBytes += len(key) + len(value)
		if variableBytes > notificationEmailVariablesMaxBytes {
			return errors.New("notification email variables exceed size limit")
		}
	}
	rawHTMLBytes := 0
	for key, value := range rawHTMLVariables {
		if _, ok := allowed[key]; !ok || !notificationEmailRawHTMLAllowed(event, key) {
			return fmt.Errorf("unsupported raw HTML notification email variable for %s: %s", event, key)
		}
		rawHTMLBytes += len(key) + len(value)
		if rawHTMLBytes > notificationEmailRawHTMLMaxBytes {
			return errors.New("notification email raw HTML variables exceed size limit")
		}
	}
	return nil
}

func maskNotificationEmail(email string) string {
	trimmed := strings.TrimSpace(email)
	parts := strings.Split(trimmed, "@")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "***"
	}
	local := parts[0]
	if len(local) == 1 {
		local = local[:1] + "***"
	} else {
		local = local[:1] + "***" + local[len(local)-1:]
	}
	domain := parts[1]
	domainParts := strings.SplitN(domain, ".", 2)
	if domainParts[0] == "" {
		return local + "@***"
	}
	maskedDomain := domainParts[0][:1] + "***"
	if len(domainParts) == 2 {
		maskedDomain += "." + domainParts[1]
	}
	return local + "@" + maskedDomain
}

func cloneNotificationEmailVariables(values map[string]string) map[string]string {
	if len(values) == 0 {
		return map[string]string{}
	}
	cloned := make(map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}
