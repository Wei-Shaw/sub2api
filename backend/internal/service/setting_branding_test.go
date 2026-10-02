//go:build unit

package service

import (
	"context"
	"encoding/base64"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func brandingDataURL(content string) string {
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString([]byte(content))
}

func TestSiteBrandingAssetFromDataURL(t *testing.T) {
	for _, tt := range []struct {
		name, content, contentType, extension string
	}{
		{"png", "\x89PNG\r\n\x1a\n", "image/png", "png"},
		{"jpeg", "\xff\xd8\xff", "image/jpeg", "jpg"},
		{"gif", "GIF89a", "image/gif", "gif"},
		{"webp", "RIFFxxxxWEBP", "image/webp", "webp"},
		{"ico", "\x00\x00\x01\x00", "image/x-icon", "ico"},
		{"svg", `<svg xmlns="http://www.w3.org/2000/svg"/>`, "image/svg+xml", "svg"},
		{"svg_xml", "\xef\xbb\xbf<?xml version=\"1.0\"?><!-- logo --><svg/>", "image/svg+xml", "svg"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			raw := brandingDataURL(tt.content)
			asset := siteBrandingAssetFromDataURL(raw)
			require.NotNil(t, asset)
			require.Equal(t, tt.contentType, asset.contentType)
			require.Equal(t, []byte(tt.content), asset.content)
			require.True(t, strings.HasSuffix(asset.path, "."+tt.extension))
			require.True(t, brandingAssetPathPattern.MatchString(asset.path))
			require.Equal(t, asset.path, siteBrandingAssetFromDataURL(raw).path)
			// MIME metadata does not affect the content address or actual type.
			require.Equal(t, asset.path, siteBrandingAssetFromDataURL(strings.Replace(raw, "image/png", "image/jpeg;charset=utf-8", 1)).path)
		})
	}
}

func TestPublicSiteLogoPreservesUnsupportedValues(t *testing.T) {
	for _, raw := range []string{
		"", "https://example.com/logo.png", "/logo.svg", " relative/logo.png ",
		"data:image/png;base64,broken", "data:image/png;base64,",
		"data:image/svg+xml,%3Csvg/%3E", "data:text/html;base64,PHN2Zy8+",
		brandingDataURL(`<html><svg/></html>`), brandingDataURL(`text<svg/>`),
		brandingDataURL(`<svg xmlns="urn:wrong"/>`), brandingDataURL("not an image"),
	} {
		svc := &SettingService{}
		require.Equal(t, raw, svc.publicSiteLogo(raw))
		require.Nil(t, svc.siteBrandingAsset.Load())
	}
}

type brandingRepoStub struct {
	settingPublicRepoStub
	reads int
}

func (r *brandingRepoStub) GetValue(_ context.Context, key string) (string, error) {
	r.reads++
	return r.values[key], r.err
}

func TestGetSiteBrandingAssetColdAndCrossInstanceRefresh(t *testing.T) {
	ctx := context.Background()
	oldRaw, newRaw := brandingDataURL(`<svg/>`), brandingDataURL(`<svg id="new"/>`)
	oldAsset, newAsset := siteBrandingAssetFromDataURL(oldRaw), siteBrandingAssetFromDataURL(newRaw)
	require.NotEqual(t, oldAsset.path, newAsset.path)
	repo := &brandingRepoStub{settingPublicRepoStub: settingPublicRepoStub{values: map[string]string{SettingKeySiteLogo: oldRaw}}}
	svc := NewSettingService(repo, &config.Config{})

	for range 2 {
		content, contentType, ok := svc.GetSiteBrandingAsset(ctx, oldAsset.path)
		require.True(t, ok)
		require.Equal(t, oldAsset.content, content)
		require.Equal(t, oldAsset.contentType, contentType)
	}
	require.Equal(t, 1, repo.reads)
	_, _, ok := svc.GetSiteBrandingAsset(ctx, "/api/v1/settings/public/invalid.svg")
	require.False(t, ok)
	require.Equal(t, 1, repo.reads)

	// Another instance saved a new logo without updating this process's cache.
	repo.values[SettingKeySiteLogo] = newRaw
	content, _, ok := svc.GetSiteBrandingAsset(ctx, newAsset.path)
	require.True(t, ok)
	require.Equal(t, newAsset.content, content)
	require.Equal(t, 2, repo.reads)
	_, _, ok = svc.GetSiteBrandingAsset(ctx, oldAsset.path)
	require.False(t, ok, "an old hash must never return new bytes")

	repo.err = errors.New("database unavailable")
	_, _, ok = svc.GetSiteBrandingAsset(ctx, oldAsset.path)
	require.False(t, ok)
	_, _, ok = svc.GetSiteBrandingAsset(ctx, newAsset.path)
	require.True(t, ok, "cached assets remain available during a database outage")
}

func TestBrandingPublicInjectionAndAdminRoundTrip(t *testing.T) {
	ctx := context.Background()
	raw := brandingDataURL(`<svg/>`)
	values := map[string]string{SettingKeySiteLogo: raw, SettingKeySiteName: "Example"}
	svc := NewSettingService(&settingPublicRepoStub{values: values}, &config.Config{})
	public, err := svc.GetPublicSettings(ctx)
	require.NoError(t, err)
	require.Equal(t, siteBrandingAssetFromDataURL(raw).path, public.SiteLogo)
	cached := svc.siteBrandingAsset.Load()
	injected, err := svc.GetPublicSettingsForInjection(ctx)
	require.NoError(t, err)
	require.Equal(t, public.SiteLogo, injected.(*PublicSettingsInjectionPayload).SiteLogo)
	require.Equal(t, "Example", injected.(*PublicSettingsInjectionPayload).SiteName)
	require.Same(t, cached, svc.siteBrandingAsset.Load(), "unchanged logo should not be decoded again")

	admin := NewSettingService(&settingGetAllRepoStub{values: values}, &config.Config{})
	settings, err := admin.GetAllSettings(ctx)
	require.NoError(t, err)
	require.Equal(t, raw, settings.SiteLogo)
	repo := &settingUpdateRepoStub{}
	admin.settingRepo = repo
	require.NoError(t, admin.UpdateSettings(ctx, settings))
	require.Equal(t, raw, repo.updates[SettingKeySiteLogo])
	require.Equal(t, public.SiteLogo, admin.siteBrandingAsset.Load().path)

	newRaw := brandingDataURL(`<svg id="new"/>`)
	repo.setMultipleErr = errors.New("write failed")
	settings.SiteLogo = newRaw
	require.Error(t, admin.UpdateSettings(ctx, settings))
	require.Equal(t, public.SiteLogo, admin.siteBrandingAsset.Load().path)
	repo.setMultipleErr = nil
	require.NoError(t, admin.UpdateSettings(ctx, settings))
	require.Equal(t, siteBrandingAssetFromDataURL(newRaw).path, admin.siteBrandingAsset.Load().path)
	settings.SiteLogo = ""
	require.NoError(t, admin.UpdateSettings(ctx, settings))
	require.Nil(t, admin.siteBrandingAsset.Load())
}

func TestBrandingConcurrentReadsAndRefresh(t *testing.T) {
	svc := &SettingService{}
	var wg sync.WaitGroup
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 20 {
				raw := brandingDataURL(`<svg/>`)
				path := svc.publicSiteLogo(raw)
				svc.GetSiteBrandingAsset(context.Background(), path)
				svc.refreshSiteBrandingAsset(brandingDataURL(`<svg id="new"/>`))
			}
		}()
	}
	wg.Wait()
}
