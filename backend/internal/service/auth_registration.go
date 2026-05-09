package service

import (
	"context"
	"hash/fnv"
	"net"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	dbuser "github.com/Wei-Shaw/sub2api/ent/user"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
)

const (
	maxRegistrationsPerIP = 3
	signupTrialBalanceTTL = 24 * time.Hour
)

var ErrRegistrationIPLimitExceeded = infraerrors.TooManyRequests("REGISTRATION_IP_LIMIT_EXCEEDED", "too many accounts registered from this IP")

type registrationIPContextKey struct{}

func ContextWithRegistrationIP(ctx context.Context, remoteIP string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	normalized := normalizeRegistrationIP(remoteIP)
	if normalized == "" {
		return ctx
	}
	return context.WithValue(ctx, registrationIPContextKey{}, normalized)
}

func registrationIPFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	ip, _ := ctx.Value(registrationIPContextKey{}).(string)
	return normalizeRegistrationIP(ip)
}

func normalizeRegistrationIP(remoteIP string) string {
	remoteIP = strings.TrimSpace(remoteIP)
	if remoteIP == "" {
		return ""
	}
	if host, _, err := net.SplitHostPort(remoteIP); err == nil {
		remoteIP = host
	}
	remoteIP = strings.Trim(remoteIP, "[]")
	if parsed := net.ParseIP(remoteIP); parsed != nil {
		return parsed.String()
	}
	if len(remoteIP) > 64 {
		return remoteIP[:64]
	}
	return remoteIP
}

func applySignupTrialBalance(user *User, plan signupGrantPlan, now time.Time) {
	if user == nil || plan.Balance <= 0 {
		return
	}
	expiresAt := now.Add(signupTrialBalanceTTL)
	user.TrialBalance = plan.Balance
	user.TrialBalanceExpiresAt = &expiresAt
	user.Balance = 0
}

func (s *AuthService) createSignupUser(ctx context.Context, user *User) error {
	if user == nil {
		return nil
	}
	registrationIP := registrationIPFromContext(ctx)
	user.RegistrationIP = registrationIP
	if registrationIP == "" || s == nil || s.entClient == nil {
		return s.userRepo.Create(ctx, user)
	}

	if tx := dbent.TxFromContext(ctx); tx != nil {
		return s.createSignupUserWithClient(ctx, tx.Client(), user, registrationIP)
	}

	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	txCtx := dbent.NewTxContext(ctx, tx)
	if err := s.createSignupUserWithClient(txCtx, tx.Client(), user, registrationIP); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *AuthService) createSignupUserWithClient(ctx context.Context, client *dbent.Client, user *User, registrationIP string) error {
	if err := lockRegistrationIPForSignup(ctx, client, registrationIP); err != nil {
		return err
	}
	if err := s.ensureRegistrationIPAllowed(ctx, client, registrationIP); err != nil {
		return err
	}
	return s.userRepo.Create(ctx, user)
}

func (s *AuthService) ensureRegistrationIPAllowed(ctx context.Context, client *dbent.Client, registrationIP string) error {
	if registrationIP == "" || client == nil {
		return nil
	}
	count, err := client.User.Query().
		Where(
			dbuser.RegistrationIPEQ(registrationIP),
			dbuser.DeletedAtIsNil(),
		).
		Count(ctx)
	if err != nil {
		logger.LegacyPrintf("service.auth", "[Auth] Database error checking registration IP limit: %v", err)
		return ErrServiceUnavailable
	}
	if count >= maxRegistrationsPerIP {
		return ErrRegistrationIPLimitExceeded
	}
	return nil
}

func lockRegistrationIPForSignup(ctx context.Context, client *dbent.Client, registrationIP string) error {
	if client == nil || registrationIP == "" || client.Driver().Dialect() != dialect.Postgres {
		return nil
	}
	var result entsql.Result
	return client.Driver().Exec(
		ctx,
		"SELECT pg_advisory_xact_lock($1)",
		[]any{registrationIPLockHash("registration-ip:" + registrationIP)},
		&result,
	)
}

func registrationIPLockHash(key string) int64 {
	hasher := fnv.New64a()
	_, _ = hasher.Write([]byte(key))
	return int64(hasher.Sum64())
}
