//go:build unit

package service

import (
	"context"
	"errors"
	"testing"
)

func TestUpdateVersionComparisonWithBuildMetadata(t *testing.T) {
	for _, tc := range []struct {
		current, latest string
		want            int
	}{
		{"0.2.18+toolrefs.20261025", "0.2.18", 0},
		{"0.2.18+toolrefs.20261025", "0.2.19", -1},
		{"0.2.18+toolrefs.20261025", "0.2.17", 1},
		{"v0.2.18+toolrefs.20261025", "v0.2.18+official.7", 0},
		{"0.2.18-rc1+toolrefs.2", "0.2.18", 0},
	} {
		if got := compareVersions(tc.current, tc.latest); got != tc.want {
			t.Errorf("compareVersions(%q, %q) = %d, want %d", tc.current, tc.latest, got, tc.want)
		}
	}
}

func TestUpdateBuildMetadataDoesNotReplaceSameRelease(t *testing.T) {
	ctx := context.Background()
	svc := NewUpdateService(&updateServiceCacheStub{}, &updateServiceGitHubClientStub{release: &GitHubRelease{TagName: "v0.2.18", Name: "v0.2.18"}}, "0.2.18+toolrefs.20261025.2", "release")
	fresh, err := svc.CheckUpdate(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if fresh.HasUpdate || fresh.CurrentVersion != "0.2.18+toolrefs.20261025.2" || fresh.LatestVersion != "0.2.18" {
		t.Fatalf("same release was offered as an update: %#v", fresh)
	}
	cached, err := svc.CheckUpdate(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	if cached.HasUpdate || !cached.Cached {
		t.Fatalf("cached check offered an update: %#v", cached)
	}
	if err := svc.PerformUpdate(ctx); !errors.Is(err, ErrNoUpdateAvailable) {
		t.Fatalf("same-release update must stop before download/replacement, got %v", err)
	}
}

func TestUpdateBuildMetadataStillOffersNewerRelease(t *testing.T) {
	svc := NewUpdateService(&updateServiceCacheStub{}, &updateServiceGitHubClientStub{release: &GitHubRelease{TagName: "v0.2.19", Name: "v0.2.19"}}, "0.2.18+toolrefs.20261025.2", "release")
	info, err := svc.CheckUpdate(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	if !info.HasUpdate || info.LatestVersion != "0.2.19" {
		t.Fatalf("genuinely newer releases must remain available: %#v", info)
	}
}

func TestUpdateBuildMetadataDoesNotOfferSameReleaseAsRollback(t *testing.T) {
	svc := newRollbackTestService("0.2.18+toolrefs.20261025.2", []*GitHubRelease{{TagName: "v0.2.18"}, {TagName: "v0.2.17"}})
	versions, err := svc.ListRollbackVersions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 1 || versions[0].Version != "0.2.17" {
		t.Fatalf("same release must not become a rollback target: %#v", versions)
	}
}
