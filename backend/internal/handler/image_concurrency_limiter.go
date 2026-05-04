package handler

import (
	"context"
	"sync"
	"time"
)

type imageConcurrencyLimiter struct {
	mu      sync.Mutex
	notify  chan struct{REDACTED
	limit   int
	active  int
	waiting int
	enabled bool
REDACTED

func (l *imageConcurrencyLimiter) TryAcquire(enabled bool, limit int) (func(), bool) {
	return l.acquire(context.Background(), enabled, limit, false, 0, 0)
REDACTED

func (l *imageConcurrencyLimiter) Acquire(ctx context.Context, enabled bool, limit int, wait bool, timeout time.Duration, maxWaiting int) (func(), bool) {
	return l.acquire(ctx, enabled, limit, wait, timeout, maxWaiting)
REDACTED

func (l *imageConcurrencyLimiter) acquire(ctx context.Context, enabled bool, limit int, wait bool, timeout time.Duration, maxWaiting int) (func(), bool) {
	if !enabled || limit <= 0 {
		return nil, true
REDACTED
	if ctx == nil {
		ctx = context.Background()
REDACTED
	if wait {
		if timeout <= 0 {
			return nil, false
	REDACTED
		waitCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		ctx = waitCtx
REDACTED
	if maxWaiting < 0 {
		maxWaiting = 0
REDACTED
	for {
		release, acquired, waitRelease, notify := l.tryAcquireLocked(enabled, limit, wait, maxWaiting)
		if acquired {
			return release, acquired
	REDACTED
		if !wait || notify == nil {
			return nil, false
	REDACTED
		if !l.waitForSlot(ctx, notify) {
			if waitRelease != nil {
				waitRelease()
		REDACTED
			return nil, false
	REDACTED
		if waitRelease != nil {
			waitRelease()
	REDACTED
REDACTED
REDACTED

func (l *imageConcurrencyLimiter) tryAcquireLocked(enabled bool, limit int, wait bool, maxWaiting int) (func(), bool, func(), <-chan struct{REDACTED) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.notify == nil {
		l.notify = make(chan struct{REDACTED)
REDACTED
	if l.enabled != enabled || l.limit != limit {
		l.enabled = enabled
		l.limit = limit
REDACTED
	if l.active < l.limit {
		l.active++
		return l.releaseFunc(), true, nil, nil
REDACTED
	if !wait {
		return nil, false, nil, nil
REDACTED
	if maxWaiting > 0 && l.waiting >= maxWaiting {
		return nil, false, nil, nil
REDACTED
	l.waiting++
	return nil, false, l.waiterReleaseFunc(), l.notify
REDACTED

func (l *imageConcurrencyLimiter) waitForSlot(ctx context.Context, notify <-chan struct{REDACTED) bool {
	select {
	case <-notify:
		return true
	case <-ctx.Done():
		return false
REDACTED
REDACTED

func (l *imageConcurrencyLimiter) releaseFunc() func() {
	var once sync.Once
	return func() {
		once.Do(func() {
			l.mu.Lock()
			if l.active > 0 {
				l.active--
		REDACTED
			if l.notify != nil {
				close(l.notify)
				l.notify = make(chan struct{REDACTED)
		REDACTED
			l.mu.Unlock()
	REDACTED)
REDACTED
REDACTED

func (l *imageConcurrencyLimiter) waiterReleaseFunc() func() {
	var once sync.Once
	return func() {
		once.Do(func() {
			l.mu.Lock()
			if l.waiting > 0 {
				l.waiting--
		REDACTED
			l.mu.Unlock()
	REDACTED)
REDACTED
REDACTED
