package service

import (
	"fmt"
	"strings"
)

// CodexClientProfileProvider supplies version-specific client metadata. It does
// not install clients or configure TLS/HTTP2 transport fingerprints. Providers
// must be backed by reviewed client fixtures before returning Verified.
type CodexClientProfileProvider interface {
	LookupCodexClientProfile(CodexClientProfileRequest) (CodexClientProfileRecord, error)
}

type CodexClientProfileRequest struct {
	OSClass        CodexOSClass
	Surface        CodexClientSurface
	Architecture   CodexArchitecture
	ClientVersion  string
	CatalogVersion int64
}

type CodexClientProfileVerification string

const (
	CodexClientProfileUnverified CodexClientProfileVerification = "unverified"
	CodexClientProfileVerified   CodexClientProfileVerification = "verified"
)

// Version and device family must match exactly. Evidence identifies the fixture
// provenance (for example a reviewed source commit and test report), not a claim
// inferred from a version number. The host still validates generated headers.
type CodexClientProfileRecord struct {
	Request      CodexClientProfileRequest
	ClientName   string
	Originator   string
	OSLabel      string
	Terminal     string
	AppBuild     string
	Source       string
	Evidence     string
	Verification CodexClientProfileVerification
}

type BuiltinCodexClientProfileProvider struct{}

func (BuiltinCodexClientProfileProvider) LookupCodexClientProfile(request CodexClientProfileRequest) (CodexClientProfileRecord, error) {
	fixture, err := codexRuntimeCatalogFixture(request.OSClass, request.Surface)
	if err != nil {
		return CodexClientProfileRecord{}, err
	}
	return CodexClientProfileRecord{
		Request: request, ClientName: fixture.clientName, Originator: fixture.originator,
		OSLabel: fixture.osLabel, Terminal: fixture.terminal, AppBuild: fixture.appBuild,
		Source: "builtin", Verification: CodexClientProfileUnverified,
	}, nil
}

func validateCodexClientProfileRecord(request CodexClientProfileRequest, record CodexClientProfileRecord) error {
	if request != record.Request {
		return fmt.Errorf("codex client profile does not match the requested version or device family")
	}
	if strings.TrimSpace(record.Source) == "" {
		return fmt.Errorf("codex client profile source is required")
	}
	switch record.Verification {
	case CodexClientProfileUnverified:
	case CodexClientProfileVerified:
		if strings.TrimSpace(record.Evidence) == "" {
			return fmt.Errorf("verified Codex client metadata requires fixture evidence")
		}
	default:
		return fmt.Errorf("unsupported Codex client profile verification status %q", record.Verification)
	}
	return nil
}
