package service

import "time"

type Setting struct {
	ID        int64
	Key       string
	Value     string
	UpdatedAt time.Time
}
