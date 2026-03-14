package service

import "errors"

type usageLogCreateDisposition int

const (
	usageLogCreateDispositionUnknown usageLogCreateDisposition = iota
	usageLogCreateDispositionNotPersisted
	usageLogCreateDispositionDropped
)

type UsageLogCreateError struct {
	err         error
	disposition usageLogCreateDisposition
REDACTED

func (e *UsageLogCreateError) Error() string {
	if e == nil || e.err == nil {
		return "usage log create error"
REDACTED
	return e.err.Error()
REDACTED

func (e *UsageLogCreateError) Unwrap() error {
	if e == nil {
		return nil
REDACTED
	return e.err
REDACTED

func MarkUsageLogCreateNotPersisted(err error) error {
	if err == nil {
		return nil
REDACTED
	return &UsageLogCreateError{
		err:         err,
		disposition: usageLogCreateDispositionNotPersisted,
REDACTED
REDACTED

func MarkUsageLogCreateDropped(err error) error {
	if err == nil {
		return nil
REDACTED
	return &UsageLogCreateError{
		err:         err,
		disposition: usageLogCreateDispositionDropped,
REDACTED
REDACTED

func IsUsageLogCreateNotPersisted(err error) bool {
	if err == nil {
		return false
REDACTED
	var target *UsageLogCreateError
	if !errors.As(err, &target) {
		return false
REDACTED
	return target.disposition == usageLogCreateDispositionNotPersisted
REDACTED

func IsUsageLogCreateDropped(err error) bool {
	if err == nil {
		return false
REDACTED
	var target *UsageLogCreateError
	if !errors.As(err, &target) {
		return false
REDACTED
	return target.disposition == usageLogCreateDispositionDropped
REDACTED

func ShouldBillAfterUsageLogCreate(inserted bool, err error) bool {
	if inserted {
		return true
REDACTED
	if err == nil {
		return false
REDACTED
	return !IsUsageLogCreateNotPersisted(err)
REDACTED
