package service

// Image jobs outlive a disconnected HTTP client. A durable claim is written
// before contacting the image provider, so a retry cannot start another job,
// including after a process restart. Completed images are retained for 24h.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/chatgptwebimage"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const webImageJobRetention = 24 * time.Hour
const webImageJobMaxDiskBytes int64 = 512 << 20
const webImageJobMaxFileBytes int64 = 32 << 20

type webImageJobRecord struct {
	BillingID string                     `json:"billing_id"`
	Billed    bool                       `json:"billed,omitempty"`
	State     string                     `json:"state"`
	Result    chatgptwebimage.Result     `json:"result,omitempty"`
	Error     *OpenAIImagesUpstreamError `json:"error,omitempty"`
}

type webImageRunningJob struct {
	done   chan struct{}
	record webImageJobRecord
}

type webImageJobStore struct {
	billingMu sync.Mutex
	dir       string
	mu        sync.Mutex
	active    map[string]*webImageRunningJob
}

var webImageJobStores = struct {
	sync.Mutex
	byPath map[string]*webImageJobStore
}{byPath: make(map[string]*webImageJobStore)}

func webImageJobUnavailable(code, message string) *OpenAIImagesUpstreamError {
	return &OpenAIImagesUpstreamError{StatusCode: 400, ErrorType: "image_generation_error", Code: code, Message: message}
}

func webImageJobUnknown() *OpenAIImagesUpstreamError {
	return webImageJobUnavailable("image_generation_outcome_unknown", "This image request may already have started; it will not be submitted again. Retry the same turn to retrieve a completed result")
}

func webImageJobKey(c *gin.Context, body []byte) (string, error) {
	key := getAPIKeyFromContext(c)
	if key == nil || key.ID <= 0 {
		return "", errors.New("image job requires an authenticated API key")
	}
	var request any
	if err := json.Unmarshal(body, &request); err != nil {
		return "", err
	}
	canonical, err := json.Marshal(request)
	if err != nil {
		return "", err
	}
	// Scope to the caller and conversation, not the selected pool account or
	// the planner's regenerated prompt. A failover must find the same job.
	session := ""
	for _, header := range []string{"thread-id", "conversation_id", "session-id", "session_id"} {
		if session = c.GetHeader(header); session != "" {
			break
		}
	}
	identity := fmt.Sprintf("v1\x00%d\x00%s\x00%s\x00", key.ID, session, c.Request.URL.Path)
	if id := c.GetHeader("Idempotency-Key"); id != "" {
		identity += "explicit:" + id
		canonical = nil
	}
	h := sha256.New()
	_, _ = h.Write([]byte(identity))
	_, _ = h.Write(canonical)
	return hex.EncodeToString(h.Sum(nil)), nil
}

func configuredWebImageJobStore() (*webImageJobStore, error) {
	dir := strings.TrimSpace(os.Getenv("SUB2API_WEB_IMAGE_STATE_DIR"))
	if dir == "" {
		base := strings.TrimSpace(os.Getenv("DATA_DIR"))
		if base == "" {
			base = "/app/data"
		}
		var err error
		dir, err = filepath.Abs(filepath.Join(base, "web-image-jobs"))
		if err != nil {
			return nil, fmt.Errorf("resolve web image state directory: %w", err)
		}
	}
	if !filepath.IsAbs(dir) {
		return nil, errors.New("web image state directory must be absolute")
	}
	dir = filepath.Clean(dir)
	webImageJobStores.Lock()
	defer webImageJobStores.Unlock()
	if s := webImageJobStores.byPath[dir]; s != nil {
		return s, nil
	}
	//nolint:gosec // G703: dir is a trusted operator process setting, never request input; an absolute path is required above.
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	s := &webImageJobStore{dir: dir, active: make(map[string]*webImageRunningJob)}
	webImageJobStores.byPath[dir] = s
	return s, nil
}

func (s *OpenAIGatewayService) generateWebImageOnce(ctx context.Context, c *gin.Context, account *Account, body []byte, prompt, imageModel string) (chatgptwebimage.Result, error) {
	if _, err := resolveChatGPTWebImageModel(account, imageModel); err != nil {
		return chatgptwebimage.Result{}, err
	}
	key, err := webImageJobKey(c, body)
	if err != nil {
		return chatgptwebimage.Result{}, webImageJobUnavailable("image_job_unavailable", "Could not identify the authenticated image request")
	}
	store, err := configuredWebImageJobStore()
	if err != nil {
		return chatgptwebimage.Result{}, webImageJobUnavailable("image_job_unavailable", "Image job storage is unavailable; no image was requested")
	}
	result, err := store.run(ctx, key, func(jobCtx context.Context) (chatgptwebimage.Result, error) {
		return s.generateWebImage(jobCtx, account, prompt)
	})
	if err != nil {
		return result, err
	}
	ref, err := store.billingRef(key)
	if err != nil {
		return chatgptwebimage.Result{}, webImageJobUnavailable("image_job_unavailable", "Could not recover the completed image job identity; no new image was requested")
	}
	c.Set(webImageJobBillingContextKey, ref)
	return result, nil
}

func (s *webImageJobStore) run(ctx context.Context, key string, generate func(context.Context) (chatgptwebimage.Result, error)) (chatgptwebimage.Result, error) {
	if err := ctx.Err(); err != nil {
		return chatgptwebimage.Result{}, err
	}
	if len(key) != 64 || strings.Trim(key, "0123456789abcdef") != "" {
		return chatgptwebimage.Result{}, errors.New("invalid image job key")
	}
	s.mu.Lock()
	if job := s.active[key]; job != nil {
		s.mu.Unlock()
		releaseWebImageRequestSlot(ctx)
		return waitWebImageJob(ctx, job)
	}
	path := filepath.Join(s.dir, key+".json")
	if info, err := os.Stat(path); err == nil && time.Since(info.ModTime()) <= webImageJobRetention {
		record, readErr := readWebImageJob(path)
		s.mu.Unlock()
		releaseWebImageRequestSlot(ctx)
		if readErr != nil || record.State != "complete" {
			return chatgptwebimage.Result{}, webImageJobUnknown()
		}
		return webImageJobResult(record)
	}
	// Register before waiting for capacity so identical requests join this job
	// without occupying another image slot. Never wait under the store lock:
	// the job holding the last slot may need that lock to finish.
	job := &webImageRunningJob{done: make(chan struct{})}
	s.active[key] = job
	s.mu.Unlock()
	release, err := takeWebImageGenerationSlot(ctx)
	if err != nil {
		s.finish(key, job, webImageJobRecord{Error: chatGPTWebImagePublicError(err)})
		return chatgptwebimage.Result{}, err
	}
	// Before a durable claim exists, a cancelled request must not start work.
	if err := ctx.Err(); err != nil {
		release()
		s.finish(key, job, webImageJobRecord{Error: chatGPTWebImagePublicError(err)})
		return chatgptwebimage.Result{}, err
	}
	billingID, err := s.claim(key)
	if err != nil {
		release()
		s.finish(key, job, webImageJobRecord{Error: chatGPTWebImagePublicError(err)})
		return chatgptwebimage.Result{}, err
	}
	jobCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Minute)
	go func() {
		defer cancel()
		defer release()
		record := webImageJobRecord{State: "complete", BillingID: billingID}
		// A panic leaves the durable pending claim intact and safely prevents
		// regeneration; it must not crash the whole gateway goroutine.
		defer func() {
			if recover() != nil {
				record = webImageJobRecord{State: "complete", BillingID: billingID, Error: webImageJobUnknown()}
			}
			var persistErr error
			if record.State == "retryable" {
				// Remove only our own claim, before publishing the result to
				// waiters. A second process can safely claim the request after
				// removal because this attempt provably did not generate.
				persistErr = os.Remove(path)
				if persistErr == nil {
					persistErr = syncWebImageJobDir(s.dir)
				}
			} else {
				persistErr = writeWebImageJob(path, record)
			}
			if persistErr != nil {
				record = webImageJobRecord{State: "complete", BillingID: billingID, Error: webImageJobUnknown()}
			}
			s.finish(key, job, record)
		}()
		result, err := generate(jobCtx)
		if err != nil {
			// Only a typed provider error can prove that generation never
			// started. Release this claim while this job still owns it so the
			// next request can retry a failed bootstrap/prepare or definitive
			// rejection. Never infer this from a generic HTTP 502: a timeout
			// after sending generation may already have consumed image quota.
			var providerErr *chatgptwebimage.Error
			if errors.As(err, &providerErr) && !providerErr.Accepted {
				record.State = "retryable"
			}
			public := chatGPTWebImagePublicError(err)
			// Persist only the public fields, never an upstream response body.
			record.Error = &OpenAIImagesUpstreamError{StatusCode: public.StatusCode, ErrorType: public.ErrorType, Code: public.Code, Message: public.Message, Param: public.Param}
		} else {
			record.Result = result
		}
	}()
	return waitWebImageJob(ctx, job)
}

func (s *webImageJobStore) finish(key string, job *webImageRunningJob, record webImageJobRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()
	job.record = record
	close(job.done)
	delete(s.active, key)
}

func (s *webImageJobStore) claim(key string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.makeRoom(); err != nil {
		return "", webImageJobUnavailable("image_job_storage_full", "Image job storage is unavailable or full; no image was requested")
	}
	// O_EXCL also protects against a second service process using this volume.
	path := filepath.Join(s.dir, key+".json")
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return "", webImageJobUnknown()
	}
	billingID := "web-image:" + uuid.NewString()
	claim, _ := json.Marshal(webImageJobRecord{State: "pending", BillingID: billingID})
	_, writeErr := f.Write(claim)
	if writeErr == nil {
		writeErr = f.Sync()
	}
	closeErr := f.Close()
	if writeErr != nil || closeErr != nil {
		return "", webImageJobUnavailable("image_job_unavailable", "Could not persist the image job; no image was requested")
	}
	// Persist the directory entry too before performing the external action.
	if err := syncWebImageJobDir(s.dir); err != nil {
		return "", webImageJobUnavailable("image_job_unavailable", "Could not persist the image job; no image was requested")
	}
	return billingID, nil
}

func waitWebImageJob(ctx context.Context, job *webImageRunningJob) (chatgptwebimage.Result, error) {
	select {
	case <-job.done:
		return webImageJobResult(job.record)
	case <-ctx.Done():
		return chatgptwebimage.Result{}, webImageJobUnknown()
	}
}

func webImageJobResult(record webImageJobRecord) (chatgptwebimage.Result, error) {
	if record.Error != nil {
		return chatgptwebimage.Result{}, record.Error
	}
	return record.Result, nil
}

func readWebImageJob(path string) (webImageJobRecord, error) {
	var record webImageJobRecord
	//nolint:gosec // G703: callers join the operator-owned store with a server SHA-256 key or direct ReadDir basename, never a client path.
	info, err := os.Stat(path)
	if err != nil {
		return record, err
	}
	if info.Size() > webImageJobMaxFileBytes {
		return record, errors.New("image job exceeds storage limit")
	}
	//nolint:gosec // G703: the path has the same server-owned origin as the Stat call above; requests cannot supply a filename.
	b, err := os.ReadFile(path)
	if err != nil {
		return record, err
	}
	err = json.Unmarshal(b, &record)
	return record, err
}

func writeWebImageJob(path string, record webImageJobRecord) error {
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	if int64(len(data)) > webImageJobMaxFileBytes {
		return errors.New("image job exceeds storage limit")
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".image-job-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(f.Name()) }()
	if _, err = f.Write(data); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err := os.Rename(f.Name(), path); err != nil {
		return err
	}
	return syncWebImageJobDir(filepath.Dir(path))
}

func syncWebImageJobDir(dir string) error {
	f, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	return f.Sync()
}

func (s *webImageJobStore) makeRoom() error {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return err
	}
	var used int64
	for _, entry := range entries {
		if entry.IsDir() || (!strings.HasSuffix(entry.Name(), ".json") && !strings.HasPrefix(entry.Name(), ".image-job-")) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue // A known-not-started job released its claim during the scan.
			}
			return err
		}
		if time.Since(info.ModTime()) > webImageJobRetention {
			if err := os.Remove(filepath.Join(s.dir, entry.Name())); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
			continue
		}
		if info.Size() < 1024 && strings.HasSuffix(entry.Name(), ".json") {
			// Claims now include the durable billing identity. Inspect their
			// state instead of assuming a short fixed-size pending document.
			record, readErr := readWebImageJob(filepath.Join(s.dir, entry.Name()))
			if errors.Is(readErr, os.ErrNotExist) {
				continue
			}
			if readErr != nil || record.State != "complete" {
				used += webImageJobMaxFileBytes
				continue
			}
		}
		used += info.Size()
	}
	if used+webImageJobMaxFileBytes > webImageJobMaxDiskBytes {
		return errors.New("image job storage full")
	}
	return nil
}

const webImageJobBillingContextKey = "sub2api.internal.web_image_job_billing"

type webImageJobBillingRef struct {
	store     *webImageJobStore
	key       string
	billingID string
}

func isWebImageBillingRequestID(id string) bool {
	if !strings.HasPrefix(id, "web-image:") {
		return false
	}
	_, err := uuid.Parse(strings.TrimPrefix(id, "web-image:"))
	return err == nil
}

func webImageBillingFingerprint(id string, apiKeyID int64) string {
	if !isWebImageBillingRequestID(id) {
		return ""
	}
	return fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprintf("%d\x00%s", apiKeyID, id))))
}

func (s *webImageJobStore) billingRef(key string) (*webImageJobBillingRef, error) {
	record, err := readWebImageJob(filepath.Join(s.dir, key+".json"))
	if err != nil {
		return nil, err
	}
	if record.State != "complete" || record.Error != nil || !isWebImageBillingRequestID(record.BillingID) {
		return nil, errors.New("completed image job has no valid billing identity")
	}
	return &webImageJobBillingRef{store: s, key: key, billingID: record.BillingID}, nil
}

// The billing ref is internal server state. Request headers cannot opt into
// suppressing usage recording or supply a durable image billing identity.
func applyChatGPTWebImageJobBilling(c *gin.Context, result *OpenAIForwardResult) {
	if result == nil {
		return
	}
	value, exists := c.Get(webImageJobBillingContextKey)
	ref, ok := value.(*webImageJobBillingRef)
	if !exists || !ok || ref == nil {
		return
	}
	result.RequestID = ref.billingID
	result.webImageJobBilling = ref
}

// Billing starts on the first successful response's normal RecordUsage path,
// not when a background generation completes after its client disconnects.
// Cache hits bypass the entire usage path after billing succeeds. The existing
// SQL ledger additionally uses billingID as its durable identity, so a crash
// between the ledger commit and this local marker cannot debit a second time.
func (s *webImageJobStore) billOnce(ref *webImageJobBillingRef, bill func() error) error {
	s.billingMu.Lock()
	defer s.billingMu.Unlock()
	path := filepath.Join(s.dir, ref.key+".json")
	record, err := readWebImageJob(path)
	if err != nil {
		return err
	}
	if record.State != "complete" || record.Error != nil || record.BillingID != ref.billingID {
		return errors.New("image job billing identity is no longer available")
	}
	if record.Billed {
		return nil
	}
	if err := bill(); err != nil {
		return err
	}
	record.Billed = true
	return writeWebImageJob(path, record)
}

func webImageUsageBillingID(result *OpenAIForwardResult) string {
	if result == nil || result.webImageJobBilling == nil {
		return ""
	}
	return result.webImageJobBilling.billingID
}
