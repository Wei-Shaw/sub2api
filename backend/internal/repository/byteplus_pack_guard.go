package repository

import (
	"database/sql"
	"fmt"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/packguard"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// The typed SQL dependency and startup readiness check prevent missing wiring
// from silently removing financial enforcement. Unenrolled credentials pass through.
func NewPackGuardedHTTPUpstream(cfg *config.Config, db *sql.DB) (service.HTTPUpstream, error) {
	if db == nil {
		return nil, fmt.Errorf("prepaid funding requires the application SQL database")
	}
	return service.NewBytePlusPackHTTP(NewHTTPUpstream(cfg), packguard.NewStore(db))
}
