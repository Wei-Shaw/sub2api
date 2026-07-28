package repository

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "modernc.org/sqlite"
)

func newUserEntRepo(t *testing.T) (*userRepository, *dbent.Client) {
REDACTED

	db, err := sql.Open("sqlite", fmt.Sprintf("file:%s?mode=memory&cache=shared&_fk=1", t.Name()))
REDACTED
	t.Cleanup(func() { _ = db.Close() REDACTED)
	db.SetMaxOpenConns(10)

	_, err = db.Exec("PRAGMA foreign_keys = ON")
REDACTED

	drv := entsql.OpenDB(dialect.SQLite, db)
	client := enttest.NewClient(t, enttest.WithOptions(dbent.Driver(drv)))
	t.Cleanup(func() { _ = client.Close() REDACTED)

	return newUserRepositoryWithSQL(client, db), client
REDACTED

func TestUserRepositoryGetByEmailNormalizesLegacySpacingAndCase(t *testing.T) {
	repo, _ := newUserEntRepo(t)
	ctx := context.Background()

	err := repo.Create(ctx, &service.User{
		Email:        " Legacy@Example.com ",
		Username:     "legacy-user",
		PasswordHash: "hash",
		Role:         service.RoleUser,
		Status:       service.StatusActive,
REDACTED)
REDACTED

	got, err := repo.GetByEmail(ctx, "legacy@example.com")
REDACTED
	require.Equal(t, " Legacy@Example.com ", got.Email)
REDACTED

func TestUserRepositoryExistsByEmailNormalizesLegacySpacingAndCase(t *testing.T) {
	repo, _ := newUserEntRepo(t)
	ctx := context.Background()

	err := repo.Create(ctx, &service.User{
		Email:        " Legacy@Example.com ",
		Username:     "legacy-user",
		PasswordHash: "hash",
		Role:         service.RoleUser,
		Status:       service.StatusActive,
REDACTED)
REDACTED

	exists, err := repo.ExistsByEmail(ctx, "  LEGACY@example.com  ")
REDACTED
	require.True(t, exists)
REDACTED

func TestUserRepositoryCreateRejectsNormalizedEmailDuplicate(t *testing.T) {
	repo, _ := newUserEntRepo(t)
	ctx := context.Background()

	err := repo.Create(ctx, &service.User{
		Email:        " Existing@Example.com ",
		Username:     "existing-user",
		PasswordHash: "hash",
		Role:         service.RoleUser,
		Status:       service.StatusActive,
REDACTED)
REDACTED

	err = repo.Create(ctx, &service.User{
		Email:        "existing@example.com",
		Username:     "duplicate-user",
		PasswordHash: "hash",
		Role:         service.RoleUser,
		Status:       service.StatusActive,
REDACTED)
	require.ErrorIs(t, err, service.ErrEmailExists)
REDACTED

func TestUserRepositoryUpdateRejectsNormalizedEmailDuplicate(t *testing.T) {
	repo, _ := newUserEntRepo(t)
	ctx := context.Background()

	first := &service.User{
		Email:        " Existing@Example.com ",
		Username:     "existing-user",
		PasswordHash: "hash",
		Role:         service.RoleUser,
		Status:       service.StatusActive,
REDACTED
	require.NoError(t, repo.Create(ctx, first))

	second := &service.User{
		Email:        "second@example.com",
		Username:     "second-user",
		PasswordHash: "hash",
		Role:         service.RoleUser,
		Status:       service.StatusActive,
REDACTED
	require.NoError(t, repo.Create(ctx, second))

	second.Email = " existing@example.com "
	err := repo.Update(ctx, second, service.UserUpdateFields{Email: trueREDACTED)
	require.ErrorIs(t, err, service.ErrEmailExists)
REDACTED

func TestUserRepositoryGetByEmailReportsNormalizedEmailConflict(t *testing.T) {
	repo, client := newUserEntRepo(t)
	ctx := context.Background()

	_, err := client.User.Create().
		SetEmail("Conflict@Example.com").
		SetUsername("conflict-user-1").
		SetPasswordHash("hash").
		SetRole(service.RoleUser).
		SetStatus(service.StatusActive).
		Save(ctx)
REDACTED

	_, err = client.User.Create().
		SetEmail(" conflict@example.com ").
		SetUsername("conflict-user-2").
		SetPasswordHash("hash").
		SetRole(service.RoleUser).
		SetStatus(service.StatusActive).
		Save(ctx)
REDACTED

	_, err = repo.GetByEmail(ctx, "conflict@example.com")
REDACTED
	require.ErrorContains(t, err, "normalized email lookup matched multiple users")
REDACTED

func TestUserRepositoryCreateSerializesNormalizedEmailConflictsUnderConcurrency(t *testing.T) {
	repo, client := newUserEntRepo(t)
	ctx := context.Background()

	firstCreateStarted := make(chan struct{REDACTED)
	releaseFirstCreate := make(chan struct{REDACTED)
	var firstCreate sync.Once
	client.User.Use(func(next dbent.Mutator) dbent.Mutator {
		return dbent.MutateFunc(func(ctx context.Context, m dbent.Mutation) (dbent.Value, error) {
			blocked := false
			if m.Op().Is(dbent.OpCreate) {
				firstCreate.Do(func() {
					blocked = true
					close(firstCreateStarted)
			REDACTED)
		REDACTED
			if blocked {
				<-releaseFirstCreate
		REDACTED
			return next.Mutate(ctx, m)
	REDACTED)
REDACTED)

	type createResult struct {
		err error
REDACTED

	results := make(chan createResult, 2)
	go func() {
		results <- createResult{err: repo.Create(ctx, &service.User{
			Email:        " Race@Example.com ",
			Username:     "race-user-1",
			PasswordHash: "hash",
			Role:         service.RoleUser,
			Status:       service.StatusActive,
	REDACTED)REDACTED
REDACTED()

	<-firstCreateStarted

	go func() {
		results <- createResult{err: repo.Create(ctx, &service.User{
			Email:        "race@example.com",
			Username:     "race-user-2",
			PasswordHash: "hash",
			Role:         service.RoleUser,
			Status:       service.StatusActive,
	REDACTED)REDACTED
REDACTED()

	time.Sleep(100 * time.Millisecond)
	close(releaseFirstCreate)

	first := <-results
	second := <-results

	errors := []error{first.err, second.errREDACTED
	successes := 0
	conflicts := 0
	for _, err := range errors {
		switch err {
		case nil:
			successes++
		case service.ErrEmailExists:
			conflicts++
		default:
			t.Fatalf("unexpected create error: %v", err)
	REDACTED
REDACTED
	require.Equal(t, 1, successes)
	require.Equal(t, 1, conflicts)

	count, err := client.User.Query().Where(userEmailLookupPredicate("race@example.com")).Count(ctx)
REDACTED
	require.Equal(t, 1, count)
REDACTED
