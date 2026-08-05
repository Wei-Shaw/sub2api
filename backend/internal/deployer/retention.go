package deployer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
)

type retainedRelease struct {
	version string
	image   string
}

type dockerImageMetadata struct {
	ID          string   `json:"Id"`
	RepoTags    []string `json:"RepoTags"`
	RepoDigests []string `json:"RepoDigests"`
	Size        int64    `json:"Size"`
	Config      struct {
		Labels map[string]string `json:"Labels"`
	} `json:"Config"`
}

func nextOlderRelease(state State, targetVersion, targetImage, previousVersion, previousImage string) (string, string) {
	candidates := []retainedRelease{
		{version: state.PreviousVersion, image: state.PreviousImage},
		{version: state.OlderVersion, image: state.OlderImage},
	}
	for _, candidate := range candidates {
		if candidate.version == "" || candidate.image == "" {
			continue
		}
		if sameRelease(candidate, retainedRelease{version: targetVersion, image: targetImage}) {
			continue
		}
		if sameRelease(candidate, retainedRelease{version: previousVersion, image: previousImage}) {
			continue
		}
		return candidate.version, candidate.image
	}
	return "", ""
}

func sameRelease(left, right retainedRelease) bool {
	if left.image != "" && right.image != "" && left.image == right.image {
		return true
	}
	return left.version != "" && right.version != "" &&
		strings.TrimPrefix(left.version, "v") == strings.TrimPrefix(right.version, "v")
}

func (m *Manager) performPostSuccessMaintenance(jobID string) string {
	var warnings []string
	removedBackups, backupBytes, backupErr := m.pruneAutomaticBackups()
	if len(removedBackups) > 0 {
		log.Printf("sub2api-deployer job_id=%q cleanup=automatic_backup removed=%q reclaimed_bytes=%d", jobID, strings.Join(removedBackups, ","), backupBytes)
	}
	if backupErr != nil {
		warnings = append(warnings, "automatic backup retention cleanup failed: "+backupErr.Error())
	}

	ctx, cancel := context.WithTimeout(context.Background(), m.cfg.StopTimeout.Duration)
	defer cancel()
	removedImages, imageBytes, imageErr := m.pruneManagedImages(ctx)
	if len(removedImages) > 0 {
		log.Printf("sub2api-deployer job_id=%q cleanup=managed_image removed=%q estimated_reclaimed_bytes=%d", jobID, strings.Join(removedImages, ","), imageBytes)
	}
	if imageErr != nil {
		warnings = append(warnings, "managed image retention cleanup failed: "+imageErr.Error())
	}
	return strings.Join(warnings, "; ")
}

func (m *Manager) pruneManagedImages(ctx context.Context) ([]string, int64, error) {
	output, err := m.runner.Run(ctx, nil, m.cfg.DockerBinary,
		"image", "ls", "--all", "--no-trunc", "--quiet",
	)
	if err != nil {
		return nil, 0, err
	}
	protected := m.protectedImageReferences()
	seen := make(map[string]struct{})
	var removed []string
	var reclaimed int64
	var failures []error
	for _, imageID := range strings.Fields(output) {
		if _, exists := seen[imageID]; exists {
			continue
		}
		seen[imageID] = struct{}{}
		metadata, err := m.inspectManagedImage(ctx, imageID)
		if err != nil {
			failures = append(failures, fmt.Errorf("inspect image %s: %w", imageID, err))
			continue
		}
		if !m.isOwnedManagedImage(metadata) || imageIsProtected(metadata, protected) {
			continue
		}
		if _, err := m.runner.Run(ctx, nil, m.cfg.DockerBinary, "image", "rm", metadata.ID); err != nil {
			failures = append(failures, fmt.Errorf("remove image %s: %w", metadata.ID, err))
			continue
		}
		removed = append(removed, metadata.ID)
		reclaimed += metadata.Size
	}
	return removed, reclaimed, errors.Join(failures...)
}

func (m *Manager) protectedImageReferences() map[string]struct{} {
	m.mu.RLock()
	defer m.mu.RUnlock()
	protected := make(map[string]struct{}, 8)
	add := func(image string) {
		if image != "" {
			protected[image] = struct{}{}
		}
	}
	add(m.state.ActiveImage)
	add(m.state.PreviousImage)
	add(m.state.OlderImage)
	if m.state.Job != nil {
		add(m.state.Job.FromImage)
		add(m.state.Job.TargetImage)
	}
	return protected
}

func (m *Manager) inspectManagedImage(ctx context.Context, imageID string) (dockerImageMetadata, error) {
	output, err := m.runner.Run(ctx, nil, m.cfg.DockerBinary, "image", "inspect", "--format", "{{json .}}", imageID)
	if err != nil {
		return dockerImageMetadata{}, err
	}
	var metadata dockerImageMetadata
	if err := json.Unmarshal([]byte(output), &metadata); err != nil {
		return dockerImageMetadata{}, err
	}
	if metadata.ID == "" {
		return dockerImageMetadata{}, errors.New("image metadata has no immutable ID")
	}
	return metadata, nil
}

func (m *Manager) isOwnedManagedImage(metadata dockerImageMetadata) bool {
	ownedReference := false
	for _, reference := range append(metadata.RepoTags, metadata.RepoDigests...) {
		if strings.HasPrefix(reference, m.cfg.ImageRepository+":") || strings.HasPrefix(reference, m.cfg.ImageRepository+"@") {
			ownedReference = true
			break
		}
	}
	if !ownedReference {
		return false
	}
	for key, expected := range m.cfg.RequiredImageLabels {
		if metadata.Config.Labels[key] != expected {
			return false
		}
	}
	return true
}

func imageIsProtected(metadata dockerImageMetadata, protected map[string]struct{}) bool {
	for _, reference := range append(metadata.RepoTags, metadata.RepoDigests...) {
		if _, ok := protected[reference]; ok {
			return true
		}
	}
	return false
}
