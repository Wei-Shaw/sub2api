package service

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

// Task type constants
const (
	TaskTypeVerifyCode    = "verify_code"
	TaskTypePasswordReset = "password_reset"
)

// EmailTask 邮件发送任务
type EmailTask struct {
	Email    string
	SiteName string
	TaskType string // "verify_code" or "password_reset"
	ResetURL string // Only used for password_reset task type
	Locale   string // Optional Accept-Language locale hint
}

// EmailQueueService 异步邮件队列服务
type EmailQueueService struct {
	emailService *EmailService
	dispatcher   *NotificationEmailDispatcher
	taskChan     chan EmailTask
	wg           sync.WaitGroup
	stopChan     chan struct{}
	workers      int
}

// NewDurableEmailQueueService keeps the public auth queue API while replacing
// the process-local channel with the durable notification delivery queue.
func NewDurableEmailQueueService(emailService *EmailService, dispatcher *NotificationEmailDispatcher) *EmailQueueService {
	return &EmailQueueService{emailService: emailService, dispatcher: dispatcher}
}

// NewEmailQueueService 创建邮件队列服务
func NewEmailQueueService(emailService *EmailService, workers int) *EmailQueueService {
	if workers <= 0 {
		workers = 3 // 默认3个工作协程
	}

	service := &EmailQueueService{
		emailService: emailService,
		taskChan:     make(chan EmailTask, 100), // 缓冲100个任务
		stopChan:     make(chan struct{}),
		workers:      workers,
	}

	// 启动工作协程
	service.start()

	return service
}

// start 启动工作协程
func (s *EmailQueueService) start() {
	for i := 0; i < s.workers; i++ {
		s.wg.Add(1)
		go s.worker(i)
	}
	logger.LegacyPrintf("service.email_queue", "[EmailQueue] Started %d workers", s.workers)
}

// worker 工作协程
func (s *EmailQueueService) worker(id int) {
	defer s.wg.Done()

	for {
		select {
		case task := <-s.taskChan:
			s.processTask(id, task)
		case <-s.stopChan:
			logger.LegacyPrintf("service.email_queue", "[EmailQueue] Worker %d stopping", id)
			return
		}
	}
}

// processTask 处理任务
func (s *EmailQueueService) processTask(workerID int, task EmailTask) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	switch task.TaskType {
	case TaskTypeVerifyCode:
		if err := s.emailService.SendVerifyCode(ctx, task.Email, task.SiteName, task.Locale); err != nil {
			logger.LegacyPrintf("service.email_queue", "[EmailQueue] Worker %d failed to send verify code to %s: %v", workerID, task.Email, err)
		} else {
			logger.LegacyPrintf("service.email_queue", "[EmailQueue] Worker %d sent verify code to %s", workerID, task.Email)
		}
	case TaskTypePasswordReset:
		if err := s.emailService.SendPasswordResetEmailWithCooldown(ctx, task.Email, task.SiteName, task.ResetURL, task.Locale); err != nil {
			logger.LegacyPrintf("service.email_queue", "[EmailQueue] Worker %d failed to send password reset to %s: %v", workerID, task.Email, err)
		} else {
			logger.LegacyPrintf("service.email_queue", "[EmailQueue] Worker %d sent password reset to %s", workerID, task.Email)
		}
	default:
		logger.LegacyPrintf("service.email_queue", "[EmailQueue] Worker %d unknown task type: %s", workerID, task.TaskType)
	}
}

// EnqueueVerifyCode 将验证码发送任务加入队列
func (s *EmailQueueService) EnqueueVerifyCode(email, siteName string, locale ...string) error {
	if s != nil && s.dispatcher != nil {
		ctx, cancel := context.WithTimeout(context.Background(), notificationEmailDeliveryDBTimeout)
		defer cancel()
		return s.enqueueVerifyCodeDurable(ctx, email, firstEmailLocale(locale))
	}
	task := EmailTask{
		Email:    email,
		SiteName: siteName,
		TaskType: TaskTypeVerifyCode,
		Locale:   firstEmailLocale(locale),
	}

	select {
	case s.taskChan <- task:
		logger.LegacyPrintf("service.email_queue", "[EmailQueue] Enqueued verify code task for %s", email)
		return nil
	default:
		return fmt.Errorf("email queue is full")
	}
}

// EnqueuePasswordReset 将密码重置邮件任务加入队列
func (s *EmailQueueService) EnqueuePasswordReset(email, siteName, resetURL string, locale ...string) error {
	if s != nil && s.dispatcher != nil {
		ctx, cancel := context.WithTimeout(context.Background(), notificationEmailDeliveryDBTimeout)
		defer cancel()
		return s.enqueuePasswordResetDurable(ctx, email, resetURL, firstEmailLocale(locale))
	}
	task := EmailTask{
		Email:    email,
		SiteName: siteName,
		TaskType: TaskTypePasswordReset,
		ResetURL: resetURL,
		Locale:   firstEmailLocale(locale),
	}

	select {
	case s.taskChan <- task:
		logger.LegacyPrintf("service.email_queue", "[EmailQueue] Enqueued password reset task for %s", email)
		return nil
	default:
		return fmt.Errorf("email queue is full")
	}
}

func (s *EmailQueueService) enqueueVerifyCodeDurable(ctx context.Context, email, locale string) error {
	if s == nil || s.emailService == nil || s.emailService.cache == nil || s.dispatcher == nil {
		return errors.New("durable verification email queue is not configured")
	}
	existing, err := s.emailService.cache.GetVerificationCode(ctx, email)
	if err == nil && existing != nil && time.Since(existing.CreatedAt) < verifyCodeCooldown {
		return ErrVerifyCodeTooFrequent
	}
	code, err := s.emailService.GenerateVerifyCode()
	if err != nil {
		return fmt.Errorf("generate code: %w", err)
	}
	createdAt := time.Now().UTC()
	data := &VerificationCodeData{Code: code, CreatedAt: createdAt, ExpiresAt: createdAt.Add(verifyCodeTTL)}
	if err := s.emailService.cache.SetVerificationCode(ctx, email, data, verifyCodeTTL); err != nil {
		return fmt.Errorf("save verify code: %w", err)
	}
	_, err = s.dispatcher.Enqueue(ctx, NotificationEmailSendInput{
		Event: NotificationEmailEventAuthVerifyCode, Locale: locale,
		RecipientEmail: email, RecipientName: emailRecipientName(email),
		SourceType: "auth_verification", SourceID: notificationEmailHash(email),
		ReminderKey:        createdAt.Format(time.RFC3339Nano),
		Variables:          map[string]string{"expires_in_minutes": strconv.Itoa(int(verifyCodeTTL / time.Minute))},
		SensitiveVariables: map[string]string{"verification_code": code},
	})
	if err != nil {
		_ = s.emailService.cache.DeleteVerificationCode(context.WithoutCancel(ctx), email)
		return err
	}
	return nil
}

func (s *EmailQueueService) enqueuePasswordResetDurable(ctx context.Context, email, resetURL, locale string) error {
	if s == nil || s.emailService == nil || s.emailService.cache == nil || s.dispatcher == nil {
		return errors.New("durable password reset email queue is not configured")
	}
	if s.emailService.cache.IsPasswordResetEmailInCooldown(ctx, email) {
		return nil
	}
	existing, err := s.emailService.cache.GetPasswordResetToken(ctx, email)
	var token string
	createdToken := false
	if err == nil && existing != nil {
		token = existing.Token
	} else {
		token, err = s.emailService.GeneratePasswordResetToken()
		if err != nil {
			return fmt.Errorf("generate token: %w", err)
		}
		createdToken = true
		if err := s.emailService.cache.SetPasswordResetToken(ctx, email, &PasswordResetTokenData{Token: token, CreatedAt: time.Now()}, passwordResetTokenTTL); err != nil {
			return fmt.Errorf("save reset token: %w", err)
		}
	}
	fullResetURL := fmt.Sprintf("%s?email=%s&token=%s", resetURL, url.QueryEscape(email), url.QueryEscape(token))
	queuedAt := time.Now().UTC()
	_, err = s.dispatcher.Enqueue(ctx, NotificationEmailSendInput{
		Event: NotificationEmailEventAuthPasswordReset, Locale: locale,
		RecipientEmail: email, RecipientName: emailRecipientName(email),
		SourceType: "auth_password_reset", SourceID: notificationEmailHash(email),
		ReminderKey:        queuedAt.Format(time.RFC3339Nano),
		Variables:          map[string]string{"expires_in_minutes": strconv.Itoa(int(passwordResetTokenTTL / time.Minute))},
		SensitiveVariables: map[string]string{"reset_url": fullResetURL},
	})
	if err != nil {
		if createdToken {
			_ = s.emailService.cache.DeletePasswordResetToken(context.WithoutCancel(ctx), email)
		}
		return err
	}
	if err := s.emailService.cache.SetPasswordResetEmailCooldown(context.WithoutCancel(ctx), email, passwordResetEmailCooldown); err != nil {
		logger.LegacyPrintf("service.email_queue", "[EmailQueue] failed to set password reset cooldown for %s: %v", email, err)
	}
	return nil
}

// Stop 停止队列服务
func (s *EmailQueueService) Stop() {
	if s == nil || s.stopChan == nil {
		return
	}
	close(s.stopChan)
	s.wg.Wait()
	logger.LegacyPrintf("service.email_queue", "%s", "[EmailQueue] All workers stopped")
}
