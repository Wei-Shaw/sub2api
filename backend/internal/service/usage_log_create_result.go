package service

import "github.com/Wei-Shaw/sub2api/internal/domain"

// UsageLogCreateError + the Mark*/Is*/ShouldBill helpers moved to domain.
// Service keeps a type alias + forwarders so existing call sites keep compiling.

type UsageLogCreateError = domain.UsageLogCreateError

func MarkUsageLogCreateNotPersisted(err error) error {
	return domain.MarkUsageLogCreateNotPersisted(err)
}
func MarkUsageLogCreateDropped(err error) error   { return domain.MarkUsageLogCreateDropped(err) }
func IsUsageLogCreateNotPersisted(err error) bool { return domain.IsUsageLogCreateNotPersisted(err) }
func IsUsageLogCreateDropped(err error) bool      { return domain.IsUsageLogCreateDropped(err) }
func ShouldBillAfterUsageLogCreate(inserted bool, err error) bool {
	return domain.ShouldBillAfterUsageLogCreate(inserted, err)
}
