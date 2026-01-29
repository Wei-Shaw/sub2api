package service

import (
	"context"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

const (
	soraCacheCleanupInterval = time.Hour
	soraCacheCleanupBatch    = 200
)

// SoraCacheCleanupService 负责清理 Sora 视频缓存文件。
type SoraCacheCleanupService struct {
	cacheRepo      SoraCacheFileRepository
	settingService *SettingService
	cfg            *config.Config
	stopCh         chan struct{REDACTED
	stopOnce       sync.Once
REDACTED

func NewSoraCacheCleanupService(cacheRepo SoraCacheFileRepository, settingService *SettingService, cfg *config.Config) *SoraCacheCleanupService {
	return &SoraCacheCleanupService{
		cacheRepo:      cacheRepo,
		settingService: settingService,
		cfg:            cfg,
		stopCh:         make(chan struct{REDACTED),
REDACTED
REDACTED

func (s *SoraCacheCleanupService) Start() {
	if s == nil || s.cacheRepo == nil {
		return
REDACTED
	go s.cleanupLoop()
REDACTED

func (s *SoraCacheCleanupService) Stop() {
	if s == nil {
		return
REDACTED
	s.stopOnce.Do(func() {
		close(s.stopCh)
REDACTED)
REDACTED

func (s *SoraCacheCleanupService) cleanupLoop() {
	ticker := time.NewTicker(soraCacheCleanupInterval)
	defer ticker.Stop()

	s.cleanupOnce()
	for {
		select {
		case <-ticker.C:
			s.cleanupOnce()
		case <-s.stopCh:
			return
	REDACTED
REDACTED
REDACTED

func (s *SoraCacheCleanupService) cleanupOnce() {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	if s.cacheRepo == nil {
		return
REDACTED

	cfg := s.getSoraConfig(ctx)
	videoDir := strings.TrimSpace(cfg.Cache.VideoDir)
	if videoDir == "" {
		return
REDACTED
	maxBytes := cfg.Cache.MaxBytes
	if maxBytes <= 0 {
		return
REDACTED

	size, err := dirSize(videoDir)
	if err != nil {
		log.Printf("[SoraCacheCleanup] 计算目录大小失败: %v", err)
		return
REDACTED
	if size <= maxBytes {
		return
REDACTED

	for size > maxBytes {
		entries, err := s.cacheRepo.ListOldest(ctx, soraCacheCleanupBatch)
		if err != nil {
			log.Printf("[SoraCacheCleanup] 读取缓存记录失败: %v", err)
			return
	REDACTED
		if len(entries) == 0 {
			log.Printf("[SoraCacheCleanup] 无缓存记录但目录仍超限: size=%d max=%d", size, maxBytes)
			return
	REDACTED

		ids := make([]int64, 0, len(entries))
		for _, entry := range entries {
			if entry == nil {
				continue
		REDACTED
			removedSize := entry.SizeBytes
			if entry.CachePath != "" {
				if info, err := os.Stat(entry.CachePath); err == nil {
					if removedSize <= 0 {
						removedSize = info.Size()
				REDACTED
			REDACTED
				if err := os.Remove(entry.CachePath); err != nil && !os.IsNotExist(err) {
					log.Printf("[SoraCacheCleanup] 删除缓存文件失败: path=%s err=%v", entry.CachePath, err)
			REDACTED
		REDACTED

			if entry.ID > 0 {
				ids = append(ids, entry.ID)
		REDACTED
			if removedSize > 0 {
				size -= removedSize
				if size < 0 {
					size = 0
			REDACTED
		REDACTED
	REDACTED

		if len(ids) > 0 {
			if err := s.cacheRepo.DeleteByIDs(ctx, ids); err != nil {
				log.Printf("[SoraCacheCleanup] 删除缓存记录失败: %v", err)
		REDACTED
	REDACTED

		if size > maxBytes {
			if refreshed, err := dirSize(videoDir); err == nil {
				size = refreshed
		REDACTED
	REDACTED
REDACTED
REDACTED

func (s *SoraCacheCleanupService) getSoraConfig(ctx context.Context) config.SoraConfig {
	if s.settingService != nil {
		return s.settingService.GetSoraConfig(ctx)
REDACTED
	if s.cfg != nil {
		return s.cfg.Sora
REDACTED
	return config.SoraConfig{REDACTED
REDACTED
