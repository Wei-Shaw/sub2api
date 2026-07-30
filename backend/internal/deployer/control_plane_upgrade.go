package deployer

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

const (
	controlPlaneProtocolLabel       = "io.tokensupply.sub2api.control-plane-protocol"
	controlPlaneManifestDigestLabel = "io.tokensupply.sub2api.control-plane-manifest-sha256"
	controlPlaneImageProtocolV1     = "1"
	controlPlaneManifestPath        = "/opt/sub2api-control-plane/CONTROL-PLANE-MANIFEST.json"
	controlPlaneBinaryPath          = "/opt/sub2api-control-plane/sub2api-deployer"
)

var digestPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

type controlPlaneManifest struct {
	Schema         int                                   `json:"schema"`
	Version        string                                `json:"version"`
	Commit         string                                `json:"commit"`
	RuntimePayload map[string]controlPlaneRuntimePayload `json:"runtime_payload"`
}

type controlPlaneRuntimePayload struct {
	Assets []controlPlaneManifestAsset `json:"assets"`
}

type controlPlaneManifestAsset struct {
	Type   string `json:"type"`
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Owner  int    `json:"owner"`
	Group  int    `json:"group"`
	Mode   uint32 `json:"mode"`
}

type verifiedControlPlaneStage struct {
	Directory    string
	BinaryPath   string
	BinarySHA256 string
	ManifestPath string
	ManifestSHA  string
	Commit       string
	Arch         string
}

type controlPlaneBuildIdentity struct {
	Version string
	Commit  string
	Type    string
	Arch    string
}

func (m *Manager) stageControlPlaneUpgrade(ctx context.Context, job *Job) (*verifiedControlPlaneStage, bool, error) {
	labels, err := m.imageLabels(ctx, job.TargetImage)
	if err != nil {
		return nil, false, err
	}
	protocol := strings.TrimSpace(labels[controlPlaneProtocolLabel])
	if protocol == "" {
		return nil, true, nil
	}
	if protocol != controlPlaneImageProtocolV1 {
		return nil, false, fmt.Errorf("unsupported control-plane protocol %q", protocol)
	}
	expectedManifestSHA := strings.TrimSpace(labels[controlPlaneManifestDigestLabel])
	if !digestPattern.MatchString(expectedManifestSHA) {
		return nil, false, errors.New("control-plane protocol 1 image has an invalid manifest digest label")
	}
	expectedCommit := strings.TrimSpace(labels["org.opencontainers.image.revision"])
	if expectedCommit == "" {
		return nil, false, errors.New("control-plane protocol 1 image has no OCI revision label")
	}

	stageRoot := filepath.Join(controlPlaneStateDirectory(m.cfg), "control-plane-staging")
	if err := os.MkdirAll(stageRoot, 0700); err != nil {
		return nil, false, fmt.Errorf("create control-plane staging root: %w", err)
	}
	if err := validateOwnedDirectory(stageRoot, os.Geteuid(), os.Getegid(), 0700); err != nil {
		return nil, false, fmt.Errorf("control-plane staging root is unsafe: %w", err)
	}
	finalStage := filepath.Join(stageRoot, job.ID)
	if _, err := os.Lstat(finalStage); err == nil {
		stage, verifyErr := m.verifyPublishedControlPlaneStage(ctx, job, finalStage, expectedManifestSHA, expectedCommit)
		return stage, false, verifyErr
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, false, fmt.Errorf("inspect existing control-plane stage: %w", err)
	}
	temporaryStage, err := os.MkdirTemp(stageRoot, ".stage-")
	if err != nil {
		return nil, false, fmt.Errorf("create temporary control-plane stage: %w", err)
	}
	published := false
	defer func() {
		if !published {
			_ = os.RemoveAll(temporaryStage)
		}
	}()

	containerID, err := m.runner.Run(ctx, nil, m.cfg.DockerBinary, "create", "--network=none", job.TargetImage, "/bin/true")
	if err != nil {
		return nil, false, fmt.Errorf("create immutable control-plane extraction container: %w", err)
	}
	containerID = strings.TrimSpace(containerID)
	if containerID == "" {
		return nil, false, errors.New("docker create returned an empty extraction container id")
	}
	defer func() {
		_, _ = m.runner.Run(context.Background(), nil, m.cfg.DockerBinary, "rm", "--force", containerID)
	}()

	manifestPath := filepath.Join(temporaryStage, "CONTROL-PLANE-MANIFEST.json")
	binaryPath := filepath.Join(temporaryStage, "sub2api-deployer")
	if _, err := m.runner.Run(ctx, nil, m.cfg.DockerBinary, "cp", containerID+":"+controlPlaneManifestPath, manifestPath); err != nil {
		return nil, false, fmt.Errorf("extract control-plane manifest: %w", err)
	}
	if _, err := m.runner.Run(ctx, nil, m.cfg.DockerBinary, "cp", containerID+":"+controlPlaneBinaryPath, binaryPath); err != nil {
		return nil, false, fmt.Errorf("extract control-plane deployer: %w", err)
	}
	for _, path := range []string{manifestPath, binaryPath} {
		info, err := os.Lstat(path)
		if err != nil {
			return nil, false, fmt.Errorf("inspect staged control-plane payload: %w", err)
		}
		if !info.Mode().IsRegular() {
			return nil, false, fmt.Errorf("staged control-plane payload %s is not a regular file", filepath.Base(path))
		}
		expectedMode := os.FileMode(0644)
		if path == binaryPath {
			expectedMode = 0755
		}
		if _, err := readRegularFileMode(path, os.Geteuid(), os.Getegid(), expectedMode); err != nil {
			return nil, false, fmt.Errorf("staged control-plane payload %s has unsafe ownership: %w", filepath.Base(path), err)
		}
	}

	manifestBytes, err := readRegularFileMode(manifestPath, os.Geteuid(), os.Getegid(), 0644)
	if err != nil {
		return nil, false, fmt.Errorf("read staged control-plane manifest: %w", err)
	}
	manifestSHA := sha256Digest(manifestBytes)
	if manifestSHA != expectedManifestSHA {
		return nil, false, fmt.Errorf("control-plane manifest digest mismatch: expected %s, got %s", expectedManifestSHA, manifestSHA)
	}
	var manifest controlPlaneManifest
	decoder := json.NewDecoder(strings.NewReader(string(manifestBytes)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return nil, false, fmt.Errorf("decode control-plane manifest: %w", err)
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return nil, false, errors.New("control-plane manifest contains trailing JSON data")
	}
	if manifest.Schema != 2 {
		return nil, false, fmt.Errorf("unsupported control-plane manifest schema %d", manifest.Schema)
	}
	if manifest.Version != job.TargetVersion {
		return nil, false, fmt.Errorf("control-plane manifest version mismatch: expected %q, got %q", job.TargetVersion, manifest.Version)
	}
	if manifest.Commit != expectedCommit {
		return nil, false, fmt.Errorf("control-plane manifest commit mismatch: expected %q, got %q", expectedCommit, manifest.Commit)
	}
	runtimeKey := "linux/" + runtime.GOARCH
	payload, ok := manifest.RuntimePayload[runtimeKey]
	if !ok {
		return nil, false, fmt.Errorf("control-plane manifest has no payload for %s", runtimeKey)
	}
	if len(payload.Assets) != 1 {
		return nil, false, fmt.Errorf("control-plane manifest payload for %s must contain exactly one asset", runtimeKey)
	}
	asset := payload.Assets[0]
	if asset.Type != controlPlaneAssetDeployer || asset.Path != controlPlaneBinaryPath || !digestPattern.MatchString(asset.SHA256) || asset.Owner != 0 || asset.Group != 0 || asset.Mode != 0755 {
		return nil, false, fmt.Errorf("control-plane manifest payload for %s is invalid", runtimeKey)
	}
	binarySHA, err := digestRegularFile(binaryPath, os.Geteuid(), os.Getegid(), 0755)
	if err != nil {
		return nil, false, fmt.Errorf("read staged deployer: %w", err)
	}
	if binarySHA != asset.SHA256 {
		return nil, false, fmt.Errorf("staged deployer digest mismatch: expected %s, got %s", asset.SHA256, binarySHA)
	}
	versionOutput, err := m.runner.Run(ctx, nil, binaryPath, "--version")
	if err != nil {
		return nil, false, fmt.Errorf("read staged deployer build identity: %w", err)
	}
	identity, err := parseControlPlaneBuildIdentity(versionOutput)
	if err != nil {
		return nil, false, err
	}
	if identity.Version != job.TargetVersion || identity.Commit != expectedCommit || identity.Type != "release" || identity.Arch != runtime.GOARCH {
		return nil, false, fmt.Errorf("staged deployer build identity mismatch: got version=%q commit=%q type=%q arch=%q", identity.Version, identity.Commit, identity.Type, identity.Arch)
	}
	if m.cfg.LoadedFrom != "" {
		if _, err := m.runner.Run(ctx, nil, binaryPath, "-config", m.cfg.LoadedFrom, "-check"); err != nil {
			return nil, false, fmt.Errorf("validate staged deployer against host configuration: %w", err)
		}
	}

	if info, err := os.Lstat(finalStage); err == nil {
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return nil, false, errors.New("existing control-plane stage is unsafe")
		}
		existingManifest, manifestErr := readRegularFileMode(filepath.Join(finalStage, "CONTROL-PLANE-MANIFEST.json"), os.Geteuid(), os.Getegid(), 0644)
		existingBinarySHA, binaryErr := digestRegularFile(filepath.Join(finalStage, "sub2api-deployer"), os.Geteuid(), os.Getegid(), 0755)
		if manifestErr != nil || binaryErr != nil || sha256Digest(existingManifest) != manifestSHA || existingBinarySHA != binarySHA {
			return nil, false, errors.New("existing control-plane stage does not match the verified target payload")
		}
		if err := os.RemoveAll(temporaryStage); err != nil {
			return nil, false, fmt.Errorf("remove duplicate temporary control-plane stage: %w", err)
		}
		published = true
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, false, fmt.Errorf("inspect existing control-plane stage: %w", err)
	} else if err := os.Rename(temporaryStage, finalStage); err != nil {
		return nil, false, fmt.Errorf("publish verified control-plane stage: %w", err)
	} else {
		published = true
	}
	return &verifiedControlPlaneStage{
		Directory:    finalStage,
		BinaryPath:   filepath.Join(finalStage, "sub2api-deployer"),
		BinarySHA256: binarySHA,
		ManifestPath: filepath.Join(finalStage, "CONTROL-PLANE-MANIFEST.json"),
		ManifestSHA:  manifestSHA,
		Commit:       expectedCommit,
		Arch:         runtime.GOARCH,
	}, false, nil
}

func (m *Manager) verifyPublishedControlPlaneStage(ctx context.Context, job *Job, directory, expectedManifestSHA, expectedCommit string) (*verifiedControlPlaneStage, error) {
	if err := validateOwnedDirectory(directory, os.Geteuid(), os.Getegid(), 0700); err != nil {
		return nil, errors.New("existing control-plane stage is unsafe")
	}
	manifestPath := filepath.Join(directory, "CONTROL-PLANE-MANIFEST.json")
	binaryPath := filepath.Join(directory, controlPlaneAssetDeployer)
	manifestBytes, err := readRegularFileMode(manifestPath, os.Geteuid(), os.Getegid(), 0644)
	if err != nil || sha256Digest(manifestBytes) != expectedManifestSHA {
		return nil, errors.New("existing control-plane manifest does not match the immutable image label")
	}
	var manifest controlPlaneManifest
	decoder := json.NewDecoder(strings.NewReader(string(manifestBytes)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return nil, fmt.Errorf("decode existing control-plane manifest: %w", err)
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return nil, errors.New("existing control-plane manifest contains trailing JSON data")
	}
	runtimeKey := "linux/" + runtime.GOARCH
	payload, ok := manifest.RuntimePayload[runtimeKey]
	if manifest.Schema != 2 || manifest.Version != job.TargetVersion || manifest.Commit != expectedCommit || !ok || len(payload.Assets) != 1 {
		return nil, errors.New("existing control-plane manifest identity is invalid")
	}
	asset := payload.Assets[0]
	if asset.Type != controlPlaneAssetDeployer || asset.Path != controlPlaneBinaryPath || !digestPattern.MatchString(asset.SHA256) || asset.Owner != 0 || asset.Group != 0 || asset.Mode != 0755 {
		return nil, errors.New("existing control-plane manifest asset is invalid")
	}
	binaryDigest, err := digestRegularFile(binaryPath, os.Geteuid(), os.Getegid(), 0755)
	if err != nil || binaryDigest != asset.SHA256 {
		return nil, errors.New("existing staged deployer digest is invalid")
	}
	versionOutput, err := m.runner.Run(ctx, nil, binaryPath, "--version")
	if err != nil {
		return nil, fmt.Errorf("read existing staged deployer build identity: %w", err)
	}
	identity, err := parseControlPlaneBuildIdentity(versionOutput)
	if err != nil || identity.Version != job.TargetVersion || identity.Commit != expectedCommit || identity.Type != "release" || identity.Arch != runtime.GOARCH {
		return nil, errors.New("existing staged deployer build identity is invalid")
	}
	if m.cfg.LoadedFrom != "" {
		if _, err := m.runner.Run(ctx, nil, binaryPath, "-config", m.cfg.LoadedFrom, "-check"); err != nil {
			return nil, fmt.Errorf("validate existing staged deployer against host configuration: %w", err)
		}
	}
	return &verifiedControlPlaneStage{
		Directory:    directory,
		BinaryPath:   binaryPath,
		BinarySHA256: binaryDigest,
		ManifestPath: manifestPath,
		ManifestSHA:  expectedManifestSHA,
		Commit:       expectedCommit,
		Arch:         runtime.GOARCH,
	}, nil
}

func (m *Manager) cleanupControlPlaneStage(jobID string) {
	if strings.TrimSpace(m.cfg.ControlPlaneUpgradePath) == "" || !requestIDPattern.MatchString(jobID) {
		return
	}
	stageRoot := filepath.Join(controlPlaneStateDirectory(m.cfg), "control-plane-staging")
	_ = os.RemoveAll(filepath.Join(stageRoot, jobID))
}

func (m *Manager) imageLabels(ctx context.Context, image string) (map[string]string, error) {
	if m.runner == nil {
		return nil, errors.New("docker command runner is not configured")
	}
	output, err := m.runner.Run(ctx, nil, m.cfg.DockerBinary, "image", "inspect", "--format", "{{json .Config.Labels}}", image)
	if err != nil {
		return nil, fmt.Errorf("inspect image labels: %w", err)
	}
	labels := make(map[string]string)
	if err := json.Unmarshal([]byte(output), &labels); err != nil {
		return nil, fmt.Errorf("decode image labels: %w", err)
	}
	return labels, nil
}

func sha256Digest(data []byte) string {
	digest := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(digest[:])
}

func parseControlPlaneBuildIdentity(output string) (controlPlaneBuildIdentity, error) {
	const prefix = "Sub2API Deployer "
	line := strings.TrimSpace(output)
	if !strings.HasPrefix(line, prefix) {
		return controlPlaneBuildIdentity{}, errors.New("staged deployer returned an invalid build identity")
	}
	versionEnd := strings.Index(line[len(prefix):], " (commit: ")
	if versionEnd < 0 {
		return controlPlaneBuildIdentity{}, errors.New("staged deployer build identity has no commit")
	}
	version := line[len(prefix) : len(prefix)+versionEnd]
	commitStart := len(prefix) + versionEnd + len(" (commit: ")
	commitEnd := strings.Index(line[commitStart:], ", built:")
	typeMarker := ", type: "
	typeStart := strings.LastIndex(line, typeMarker)
	archMarker := ", arch: "
	archStart := strings.LastIndex(line, archMarker)
	if commitEnd < 0 || typeStart < 0 || archStart < 0 || typeStart >= archStart || !strings.HasSuffix(line, ")") {
		return controlPlaneBuildIdentity{}, errors.New("staged deployer returned an incomplete build identity")
	}
	commit := line[commitStart : commitStart+commitEnd]
	buildType := line[typeStart+len(typeMarker) : archStart]
	arch := strings.TrimSuffix(line[archStart+len(archMarker):], ")")
	if version == "" || commit == "" || buildType == "" || arch == "" {
		return controlPlaneBuildIdentity{}, errors.New("staged deployer returned an empty build identity field")
	}
	return controlPlaneBuildIdentity{Version: version, Commit: commit, Type: buildType, Arch: arch}, nil
}
