//go:build schedulerfixture

package service

import (
	"context"
	"encoding/csv"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/stretchr/testify/require"
)

// TestSchedulerProductionFixture_NewAndOldSelectionsRespectWeights reads only
// the sanitized local export. It never opens a connection to production.
func TestSchedulerProductionFixture_NewAndOldSelectionsRespectWeights(t *testing.T) {
	dir := os.Getenv("SCHEDULER_FIXTURE_DIR")
	if dir == "" {
		t.Skip("SCHEDULER_FIXTURE_DIR is not set")
	}
	accounts := readSchedulerFixtureAccounts(t, filepath.Join(dir, "accounts.csv"))
	groups := readSchedulerFixtureGroups(t, filepath.Join(dir, "groups.csv"))
	memberships := readSchedulerFixtureMemberships(t, filepath.Join(dir, "account_groups.csv"))

	platformTypes := make(map[string]struct{})
	for _, account := range accounts {
		platformTypes[account.Platform+"/"+account.Type] = struct{}{}
	}
	for _, platform := range []string{PlatformAnthropic, PlatformGemini, PlatformOpenAI, PlatformAntigravity, PlatformGrok} {
		found := false
		for key := range platformTypes {
			if strings.HasPrefix(key, platform+"/") {
				found = true
				break
			}
		}
		require.True(t, found, "fixture must contain platform %s", platform)
	}

	largestGroupID := int64(0)
	largestGroupSize := 0
	validatedGroups := 0
	for groupID, accountIDs := range memberships {
		if len(accountIDs) > largestGroupSize {
			largestGroupID = groupID
			largestGroupSize = len(accountIDs)
		}
		group, ok := groups[groupID]
		if !ok || group.Status != StatusActive {
			continue
		}
		candidates := make([]*Account, 0, len(accountIDs))
		for _, accountID := range accountIDs {
			account := accounts[accountID]
			if account == nil || account.Platform != group.Platform || !account.IsActive() || !account.Schedulable {
				continue
			}
			copyAccount := *account
			copyAccount.GroupIDs = []int64{groupID}
			copyAccount.Concurrency = 1
			candidates = append(candidates, &copyAccount)
		}
		if len(candidates) == 0 {
			continue
		}

		legacy, v2 := selectFixtureAccountsWithBothEngines(t, groupID, group.Platform, candidates)
		assertFixtureWeightSelection(t, legacy, candidates)
		assertFixtureWeightSelection(t, v2, candidates)
		validatedGroups++
	}

	require.Greater(t, validatedGroups, 0)
	require.GreaterOrEqual(t, largestGroupSize, 3000)
	t.Logf("validated %d groups from sanitized fixture; largest group=%d accounts=%d account_rows=%d bindings=%d",
		validatedGroups, largestGroupID, largestGroupSize, len(accounts), fixtureMembershipCount(memberships))
}

func selectFixtureAccountsWithBothEngines(t *testing.T, groupID int64, platform string, candidates []*Account) (*Account, *Account) {
	t.Helper()
	cache := newSchedulerV2CacheStub()
	cache.legacySnapshot = append([]*Account(nil), candidates...)
	values := make([]Account, 0, len(candidates))
	for _, account := range candidates {
		copyAccount := *account
		values = append(values, copyAccount)
		cache.accounts[copyAccount.ID] = &copyAccount
	}
	mode := SchedulerModeForced
	if platform == PlatformOpenAI || platform == PlatformGrok {
		mode = SchedulerModeSingle
	}
	bucket := SchedulerBucket{GroupID: groupID, Platform: platform, Mode: mode}
	require.NoError(t, cache.SetCandidateIndex(context.Background(), bucket, values))
	snapshot := NewSchedulerSnapshotService(cache, nil, nil, nil, &config.Config{})

	selectAccount := func() (*Account, error) {
		if platform == PlatformOpenAI || platform == PlatformGrok {
			gateway := &OpenAIGatewayService{schedulerSnapshot: snapshot}
			return gateway.selectAccountForModelWithExclusions(context.Background(), &groupID, platform, "", "", nil, false, 0, "")
		}
		gateway := &GatewayService{schedulerSnapshot: snapshot, cfg: &config.Config{}}
		ctx := context.WithValue(context.Background(), ctxkey.ForcePlatform, platform)
		return gateway.SelectAccountForModelWithExclusions(ctx, &groupID, "", "", nil)
	}

	cache.state = SchedulerEngineState{Engine: SchedulerEngineLegacy, Status: SchedulerEngineStatusDisabled}
	legacy, err := selectAccount()
	require.NoError(t, err)
	cache.state = SchedulerEngineState{Engine: SchedulerEngineV2, Status: SchedulerEngineStatusActive}
	v2, err := selectAccount()
	require.NoError(t, err)
	return legacy, v2
}

type schedulerFixtureGroup struct {
	Platform string
	Status   string
}

func readSchedulerFixtureAccounts(t *testing.T, path string) map[int64]*Account {
	t.Helper()
	file, err := os.Open(path)
	require.NoError(t, err)
	defer file.Close()
	reader := csv.NewReader(file)
	_, err = reader.Read()
	require.NoError(t, err)
	accounts := make(map[int64]*Account)
	for {
		row, err := reader.Read()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		require.GreaterOrEqual(t, len(row), 16)
		id, parseErr := strconv.ParseInt(row[0], 10, 64)
		require.NoError(t, parseErr)
		priority, parseErr := strconv.Atoi(row[4])
		require.NoError(t, parseErr)
		accounts[id] = &Account{
			ID:          id,
			Platform:    row[1],
			Type:        row[2],
			Priority:    priority,
			Status:      row[6],
			Schedulable: row[7] == "t" || row[7] == "true",
			LastUsedAt:  parseSchedulerFixtureTime(t, row[8]),
		}
	}
	return accounts
}

func readSchedulerFixtureGroups(t *testing.T, path string) map[int64]schedulerFixtureGroup {
	t.Helper()
	file, err := os.Open(path)
	require.NoError(t, err)
	defer file.Close()
	reader := csv.NewReader(file)
	_, err = reader.Read()
	require.NoError(t, err)
	groups := make(map[int64]schedulerFixtureGroup)
	for {
		row, err := reader.Read()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		id, parseErr := strconv.ParseInt(row[0], 10, 64)
		require.NoError(t, parseErr)
		groups[id] = schedulerFixtureGroup{Platform: row[1], Status: row[2]}
	}
	return groups
}

func readSchedulerFixtureMemberships(t *testing.T, path string) map[int64][]int64 {
	t.Helper()
	file, err := os.Open(path)
	require.NoError(t, err)
	defer file.Close()
	reader := csv.NewReader(file)
	_, err = reader.Read()
	require.NoError(t, err)
	memberships := make(map[int64][]int64)
	for {
		row, err := reader.Read()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		accountID, parseErr := strconv.ParseInt(row[0], 10, 64)
		require.NoError(t, parseErr)
		groupID, parseErr := strconv.ParseInt(row[1], 10, 64)
		require.NoError(t, parseErr)
		memberships[groupID] = append(memberships[groupID], accountID)
	}
	return memberships
}

func parseSchedulerFixtureTime(t *testing.T, raw string) *time.Time {
	t.Helper()
	if raw == "" {
		return nil
	}
	normalized := strings.Replace(raw, " ", "T", 1)
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02T15:04:05.999999999-07"} {
		parsed, err := time.Parse(layout, normalized)
		if err == nil {
			return &parsed
		}
	}
	t.Fatalf("parse fixture timestamp %q", raw)
	return nil
}

func schedulerFixtureMode(platform string) string {
	if platform == PlatformAnthropic || platform == PlatformGemini {
		return SchedulerModeMixed
	}
	return SchedulerModeSingle
}

func assertFixtureWeightSelection(t *testing.T, selected *Account, candidates []*Account) {
	t.Helper()
	require.NotNil(t, selected)
	minPriority := candidates[0].Priority
	for _, account := range candidates[1:] {
		if account.Priority < minPriority {
			minPriority = account.Priority
		}
	}
	require.Equal(t, minPriority, selected.Priority)
	for _, account := range candidates {
		if account.Priority != minPriority {
			continue
		}
		if account.LastUsedAt == nil {
			require.Nil(t, selected.LastUsedAt)
			return
		}
	}
	for _, account := range candidates {
		if account.Priority == minPriority && account.LastUsedAt.Unix() < selected.LastUsedAt.Unix() {
			t.Fatalf("selected account %d is newer than eligible account %d at the same priority", selected.ID, account.ID)
		}
	}
}

func fixtureMembershipCount(memberships map[int64][]int64) int {
	total := 0
	for _, ids := range memberships {
		total += len(ids)
	}
	return total
}
