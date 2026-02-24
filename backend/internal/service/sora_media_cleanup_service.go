package service

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/robfig/cron/v3"
)

var soraCleanupCronParser = cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)

// SoraMediaCleanupService 定期清理本地媒体文件
type SoraMediaCleanupService struct {
	storage *SoraMediaStorage
	cfg     *config.Config

	cron *cron.Cron

	startOnce sync.Once
	stopOnce  sync.Once
REDACTED

func NewSoraMediaCleanupService(storage *SoraMediaStorage, cfg *config.Config) *SoraMediaCleanupService {
	return &SoraMediaCleanupService{
		storage: storage,
		cfg:     cfg,
REDACTED
REDACTED

func (s *SoraMediaCleanupService) Start() {
	if s == nil || s.cfg == nil {
		return
REDACTED
	if !s.cfg.Sora.Storage.Cleanup.Enabled {
		logger.LegacyPrintf("service.sora_media_cleanup", "[SoraCleanup] not started (disabled)")
		return
REDACTED
	if s.storage == nil || !s.storage.Enabled() {
		logger.LegacyPrintf("service.sora_media_cleanup", "[SoraCleanup] not started (storage disabled)")
		return
REDACTED

	s.startOnce.Do(func() {
		schedule := strings.TrimSpace(s.cfg.Sora.Storage.Cleanup.Schedule)
		if schedule == "" {
			logger.LegacyPrintf("service.sora_media_cleanup", "[SoraCleanup] not started (empty schedule)")
			return
	REDACTED
		loc := time.Local
		if strings.TrimSpace(s.cfg.Timezone) != "" {
			if parsed, err := time.LoadLocation(strings.TrimSpace(s.cfg.Timezone)); err == nil && parsed != nil {
				loc = parsed
		REDACTED
	REDACTED
		c := cron.New(cron.WithParser(soraCleanupCronParser), cron.WithLocation(loc))
		if _, err := c.AddFunc(schedule, func() { s.runCleanup() REDACTED); err != nil {
			logger.LegacyPrintf("service.sora_media_cleanup", "[SoraCleanup] not started (invalid schedule=%q): %v", schedule, err)
			return
	REDACTED
		s.cron = c
		s.cron.Start()
		logger.LegacyPrintf("service.sora_media_cleanup", "[SoraCleanup] started (schedule=%q tz=%s)", schedule, loc.String())
REDACTED)
REDACTED

func (s *SoraMediaCleanupService) Stop() {
	if s == nil {
		return
REDACTED
	s.stopOnce.Do(func() {
		if s.cron != nil {
			ctx := s.cron.Stop()
			select {
			case <-ctx.Done():
			case <-time.After(3 * time.Second):
				logger.LegacyPrintf("service.sora_media_cleanup", "[SoraCleanup] cron stop timed out")
		REDACTED
	REDACTED
REDACTED)
REDACTED

func (s *SoraMediaCleanupService) runCleanup() {
	if s.cfg == nil || s.storage == nil {
		return
REDACTED
	retention := s.cfg.Sora.Storage.Cleanup.RetentionDays
	if retention <= 0 {
		logger.LegacyPrintf("service.sora_media_cleanup", "[SoraCleanup] skipped (retention_days=%d)", retention)
		return
REDACTED
	cutoff := time.Now().AddDate(0, 0, -retention)
	deleted := 0

	roots := []string{s.storage.ImageRoot(), s.storage.VideoRoot()REDACTED
	for _, root := range roots {
		if root == "" {
			continue
	REDACTED
		_ = filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
		REDACTED
			if info.IsDir() {
				return nil
		REDACTED
			if info.ModTime().Before(cutoff) {
				if rmErr := os.Remove(p); rmErr == nil {
					deleted++
			REDACTED
		REDACTED
			return nil
	REDACTED)
REDACTED
	logger.LegacyPrintf("service.sora_media_cleanup", "[SoraCleanup] cleanup finished, deleted=%d", deleted)
REDACTED
