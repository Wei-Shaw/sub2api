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
	opsInstanceHeartbeatInterval = 15 * time.Second
	opsInstanceHeartbeatTimeout  = 3 * time.Second
)

type OpsInstanceHeartbeatService struct {
	opsRepo OpsRepository
	cfg     *config.Config

	settingRepo SettingRepository

	instanceID                  string
	role                        string
	hostname                    string
	autonomousBackgroundEnabled bool
	startedAt                   time.Time

	stopCh    chan struct{}
	startOnce sync.Once
	stopOnce  sync.Once
}

func NewOpsInstanceHeartbeatService(opsRepo OpsRepository, settingRepo SettingRepository, cfg *config.Config) *OpsInstanceHeartbeatService {
	hostname := strings.TrimSpace(os.Getenv("HOSTNAME"))
	if hostname == "" {
		if value, err := os.Hostname(); err == nil {
			hostname = strings.TrimSpace(value)
		}
	}

	role := config.DeploymentRoleStandalone
	instanceID := ""
	if cfg != nil {
		role = config.NormalizeDeploymentRole(cfg.Deployment.Role)
		instanceID = strings.TrimSpace(cfg.Deployment.InstanceID)
	}
	if instanceID == "" {
		if hostname != "" {
			instanceID = hostname
		} else {
			instanceID = role + "-local"
		}
	}

	return &OpsInstanceHeartbeatService{
		opsRepo:                     opsRepo,
		cfg:                         cfg,
		settingRepo:                 settingRepo,
		instanceID:                  instanceID,
		role:                        role,
		hostname:                    hostname,
		autonomousBackgroundEnabled: shouldStartAutonomousBackgroundServices(cfg),
		startedAt:                   time.Now().UTC(),
	}
}

func (s *OpsInstanceHeartbeatService) Start() {
	if s == nil {
		return
	}
	s.startOnce.Do(func() {
		s.stopCh = make(chan struct{})
		go s.run()
	})
}

func (s *OpsInstanceHeartbeatService) Stop() {
	if s == nil {
		return
	}
	s.stopOnce.Do(func() {
		if s.stopCh != nil {
			close(s.stopCh)
		}
	})
}

func (s *OpsInstanceHeartbeatService) run() {
	s.writeHeartbeat()

	ticker := time.NewTicker(opsInstanceHeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.writeHeartbeat()
		case <-s.stopCh:
			return
		}
	}
}

func (s *OpsInstanceHeartbeatService) writeHeartbeat() {
	if s == nil || s.opsRepo == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), opsInstanceHeartbeatTimeout)
	defer cancel()

	if !s.isMonitoringEnabled(ctx) {
		return
	}

	err := s.opsRepo.UpsertInstanceHeartbeat(ctx, &OpsUpsertInstanceHeartbeatInput{
		InstanceID:                  s.instanceID,
		Role:                        s.role,
		Hostname:                    s.hostname,
		AutonomousBackgroundEnabled: s.autonomousBackgroundEnabled,
		StartedAt:                   s.startedAt,
		LastSeenAt:                  time.Now().UTC(),
	})
	if err != nil {
		log.Printf("[OpsInstanceHeartbeatService] heartbeat failed: %v", err)
	}
}

func (s *OpsInstanceHeartbeatService) isMonitoringEnabled(ctx context.Context) bool {
	if s == nil {
		return false
	}
	if s.cfg != nil && !s.cfg.Ops.Enabled {
		return false
	}
	if s.settingRepo == nil {
		return true
	}

	value, err := s.settingRepo.GetValue(ctx, SettingKeyOpsMonitoringEnabled)
	if err != nil {
		return true
	}
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "false", "0", "off", "disabled":
		return false
	default:
		return true
	}
}
