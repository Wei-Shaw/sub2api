package basispoints

import (
	"fmt"
	"strings"
)

// ImageRelayLimits bounds decoded images and per-process temporary storage.
// A saved change applies to new conversions; existing links keep their expiry
// until resubmission. Lowering storage limits never evicts live images.
type ImageRelayLimits struct {
	MaxImageMiB    int
	MaxImages      int
	MaxTotalMiB    int
	StorageMiB     int
	StorageEntries int
	TTLMinutes     int
}

func DefaultImageRelayLimits() ImageRelayLimits {
	return ImageRelayLimits{
		MaxImageMiB:    imageRelayMaxImageBytes >> 20,
		MaxImages:      imageRelayMaxRequestImages,
		MaxTotalMiB:    imageRelayMaxRequestBytes >> 20,
		StorageMiB:     imageRelayMaxBytes >> 20,
		StorageEntries: imageRelayMaxEntries,
		TTLMinutes:     30,
	}
}

func (l ImageRelayLimits) Validate() error {
	if l.MaxImageMiB < 1 || l.MaxImageMiB > MaxRelayImageMiB {
		return fmt.Errorf("image relay max_image_mib must be 1-%d", MaxRelayImageMiB)
	}
	if l.MaxImages < 1 || l.MaxImages > MaxRelayImages {
		return fmt.Errorf("image relay max_images must be 1-%d", MaxRelayImages)
	}
	if l.MaxTotalMiB < 1 || l.MaxTotalMiB > MaxRelayRequestMiB {
		return fmt.Errorf("image relay max_total_mib must be 1-%d", MaxRelayRequestMiB)
	}
	if l.StorageMiB < 1 || l.StorageMiB > MaxRelayStorageMiB {
		return fmt.Errorf("image relay storage_mib must be 1-%d", MaxRelayStorageMiB)
	}
	if l.StorageEntries < 1 || l.StorageEntries > MaxRelayStorageEntries {
		return fmt.Errorf("image relay storage_entries must be 1-%d", MaxRelayStorageEntries)
	}
	if l.TTLMinutes < 1 || l.TTLMinutes > MaxRelayTTLMinutes {
		return fmt.Errorf("image relay ttl_minutes must be 1-%d", MaxRelayTTLMinutes)
	}
	if l.MaxTotalMiB < l.MaxImageMiB {
		return fmt.Errorf("image relay request image size limit must cover a single image")
	}
	if l.StorageMiB < l.MaxTotalMiB {
		return fmt.Errorf("image relay storage must cover the request image size limit")
	}
	if l.StorageEntries < l.MaxImages {
		return fmt.Errorf("image relay storage entries must cover the request image count")
	}
	return nil
}

// Configure updates origin and limits together without invalidating live links.
func (r *ImageRelay) Configure(baseURL string, limits ImageRelayLimits) error {
	if err := ValidateImageRelayOrigin(baseURL); err != nil {
		return err
	}
	if err := limits.Validate(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return ErrImageRelayStorage
	}
	r.baseURL = strings.TrimRight(baseURL, "/")
	r.limits = limits
	return nil
}
