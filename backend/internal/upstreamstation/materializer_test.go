package upstreamstation

import (
	"context"
	"testing"

	coreservice "github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

type materializerTestAdmin struct {
	groups        []coreservice.Group
	accounts      map[int64]*coreservice.Account
	createGroups  int
	createInputs  []*coreservice.CreateAccountInput
	updateInputs  []*coreservice.UpdateAccountInput
	nextGroupID   int64
	nextAccountID int64
}

func newMaterializerTestAdmin() *materializerTestAdmin {
	return &materializerTestAdmin{accounts: make(map[int64]*coreservice.Account), nextGroupID: 10, nextAccountID: 100}
}

func (a *materializerTestAdmin) GetAllGroups(context.Context) ([]coreservice.Group, error) {
	return append([]coreservice.Group(nil), a.groups...), nil
}
func (a *materializerTestAdmin) CreateGroup(_ context.Context, input *coreservice.CreateGroupInput) (*coreservice.Group, error) {
	a.createGroups++
	a.nextGroupID++
	group := coreservice.Group{ID: a.nextGroupID, Name: input.Name, Platform: input.Platform, Status: coreservice.StatusActive}
	a.groups = append(a.groups, group)
	return &group, nil
}
func (a *materializerTestAdmin) GetAccount(_ context.Context, id int64) (*coreservice.Account, error) {
	return a.accounts[id], nil
}
func (a *materializerTestAdmin) CreateAccount(_ context.Context, input *coreservice.CreateAccountInput) (*coreservice.Account, error) {
	a.nextAccountID++
	a.createInputs = append(a.createInputs, input)
	account := &coreservice.Account{
		ID: a.nextAccountID, Name: input.Name, Platform: input.Platform, Type: input.Type,
		Credentials: input.Credentials, Extra: input.Extra, GroupIDs: append([]int64(nil), input.GroupIDs...),
		Concurrency: input.Concurrency, Priority: input.Priority, Status: coreservice.StatusActive, Schedulable: true,
	}
	a.accounts[account.ID] = account
	return account, nil
}
func (a *materializerTestAdmin) UpdateAccount(_ context.Context, id int64, input *coreservice.UpdateAccountInput) (*coreservice.Account, error) {
	a.updateInputs = append(a.updateInputs, input)
	account := a.accounts[id]
	account.Name = input.Name
	account.Type = input.Type
	account.Credentials = input.Credentials
	account.Extra = input.Extra
	if input.GroupIDs != nil {
		account.GroupIDs = append([]int64(nil), (*input.GroupIDs)...)
	}
	return account, nil
}
func (a *materializerTestAdmin) SetAccountSchedulable(_ context.Context, id int64, schedulable bool) (*coreservice.Account, error) {
	a.accounts[id].Schedulable = schedulable
	return a.accounts[id], nil
}

type materializerTestLookup struct {
	admin *materializerTestAdmin
}

func (l materializerTestLookup) FindByExtraField(_ context.Context, key string, value any) ([]coreservice.Account, error) {
	var found []coreservice.Account
	for _, account := range l.admin.accounts {
		if account.Extra != nil && account.Extra[key] == value {
			found = append(found, *account)
		}
	}
	return found, nil
}

func TestManagedAccountMaterializerCreatesAndUpdatesOwnedAccount(t *testing.T) {
	t.Parallel()

	admin := newMaterializerTestAdmin()
	materializer := NewManagedAccountMaterializer(admin, materializerTestLookup{admin: admin})
	station := &Station{ID: 7, Name: "Station A", BaseURL: "https://station.example"}
	route := &Route{
		ID: 9, StationID: 7, RemoteGroupKey: "pro", RemoteGroupName: "Pro",
		Platform: coreservice.PlatformOpenAI, Models: []string{"gpt-5", "o3"}, EffectiveRate: 0.4, Schedulable: true,
	}

	accountID, err := materializer.Materialize(context.Background(), station, route, "sk-managed")
	require.NoError(t, err)
	require.Equal(t, int64(101), accountID)
	require.Equal(t, 1, admin.createGroups)
	require.Equal(t, "自动上游-OpenAI", admin.groups[0].Name)
	require.Len(t, admin.createInputs, 1)
	created := admin.createInputs[0]
	require.Equal(t, coreservice.AccountTypeAPIKey, created.Type)
	require.Equal(t, "sk-managed", created.Credentials["api_key"])
	require.Equal(t, "https://station.example", created.Credentials["base_url"])
	require.Equal(t, map[string]any{"gpt-5": "gpt-5", "o3": "o3"}, created.Credentials["model_mapping"])
	require.Equal(t, ManagedAccountOwner, created.Extra[ManagedAccountOwnerKey])
	require.Equal(t, int64(9), created.Extra[ManagedRouteIDKey])
	require.Equal(t, 0.4, created.Extra[ManagedEffectiveRateKey])
	require.Equal(t, []int64{11}, created.GroupIDs)

	route.ManagedAccountID = &accountID
	route.Models = []string{"gpt-5.1"}
	route.EffectiveRate = 0.3
	updatedID, err := materializer.Materialize(context.Background(), station, route, "sk-managed-2")
	require.NoError(t, err)
	require.Equal(t, accountID, updatedID)
	require.Equal(t, 1, admin.createGroups)
	require.Len(t, admin.updateInputs, 1)
	require.Equal(t, 0.3, admin.updateInputs[0].Extra[ManagedEffectiveRateKey])
	require.Equal(t, map[string]any{"gpt-5.1": "gpt-5.1"}, admin.updateInputs[0].Credentials["model_mapping"])
}

func TestManagedAccountMaterializerDoesNotTakeOverManualAccount(t *testing.T) {
	t.Parallel()

	admin := newMaterializerTestAdmin()
	admin.accounts[55] = &coreservice.Account{ID: 55, Name: "manual", Extra: map[string]any{"owner": "boss"}}
	materializer := NewManagedAccountMaterializer(admin, materializerTestLookup{admin: admin})
	accountID := int64(55)

	_, err := materializer.Materialize(context.Background(), &Station{ID: 7}, &Route{
		ID: 9, Platform: coreservice.PlatformOpenAI, ManagedAccountID: &accountID,
	}, "sk-managed")
	require.ErrorContains(t, err, "not owned")
	require.Empty(t, admin.updateInputs)
}
