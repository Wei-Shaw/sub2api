package web3deposit

import (
	"errors"
	"time"
)

var (
	ErrCursorNotFound          = errors.New("web3 scanner cursor not found")
	ErrCursorIdentityConflict  = errors.New("web3 scanner cursor identity conflicts with existing cursor")
	ErrCursorWouldRegress      = errors.New("web3 scanner cursor cannot move backward")
	ErrFinalizerAheadOfScanner = errors.New("web3 finalizer cursor cannot move ahead of scanner")
	ErrLeaseNotHeld            = errors.New("web3 scanner lease is not held")
	ErrCursorAdvanceRejected   = errors.New("web3 scanner cursor advance rejected")
)

type ScannerCursor struct {
	ID                 int64
	ScannerKey         string
	ChainID            uint64
	TokenContract      string
	ScanStartBlock     uint64
	LastScannedBlock   uint64
	LastFinalizedBlock uint64
	LeaseOwner         *string
	LeaseToken         *string
	LeaseExpiresAt     *time.Time
	LastError          *string
	LastSuccessAt      *time.Time
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

func (c ScannerCursor) HasActiveLease(token string, now time.Time) bool {
	return c.LeaseToken != nil &&
		*c.LeaseToken == token &&
		c.LeaseExpiresAt != nil &&
		c.LeaseExpiresAt.After(now)
}
