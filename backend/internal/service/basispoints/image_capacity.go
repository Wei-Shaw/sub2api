package basispoints

// Image capacity ceilings are administrative bounds, not allocation sizes.
// Defaults and the native attachment size limits remain unchanged. Keep the
// frontend excelBPSImageLimits constants in sync with these values.
const (
	MaxImageBodyMiB        = 1024
	MinImageBudgetMiB      = 512
	MaxImageBudgetMiB      = 65536
	MaxImageRequests       = 4096
	MaxRelayImageMiB       = 512
	MaxRelayRequestMiB     = 512
	MaxRelayImages         = 65536
	MaxRelayStorageMiB     = 262144
	MaxRelayStorageEntries = 1048576
	MaxRelayTTLMinutes     = 10080
)
