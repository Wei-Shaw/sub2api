package runtime

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
)

const EnvironmentVariable = "SUB2API_ROLE"

type Role string

const (
	RoleAll       Role = "all"
	RoleAPI       Role = "api"
	RoleWorker    Role = "worker"
	RoleScheduler Role = "scheduler"
	RoleMigrate   Role = "migrate"
	RoleBootstrap Role = "bootstrap"
)

func SupportedRoles() []Role {
	return []Role{RoleAll, RoleAPI, RoleWorker, RoleScheduler, RoleMigrate, RoleBootstrap}
}

func (r Role) Valid() bool {
	switch r {
	case RoleAll, RoleAPI, RoleWorker, RoleScheduler, RoleMigrate, RoleBootstrap:
		return true
	default:
		return false
	}
}

func (r Role) Resident() bool {
	switch r {
	case RoleAPI, RoleWorker, RoleScheduler:
		return true
	default:
		return false
	}
}

type EnvironmentLookup func(string) (string, bool)

func ResolveRole(cliSet bool, cliValue string, lookup EnvironmentLookup) (Role, error) {
	var envValue string
	var envSet bool
	if lookup != nil {
		envValue, envSet = lookup(EnvironmentVariable)
	}

	var cliRole Role
	var envRole Role
	var err error
	if cliSet {
		if cliRole, err = parseExplicitRole(cliValue); err != nil {
			return "", fmt.Errorf("command-line runtime role: %w", err)
		}
	}
	if envSet {
		if envRole, err = parseExplicitRole(envValue); err != nil {
			return "", fmt.Errorf("environment runtime role: %w", err)
		}
	}

	if cliSet && envSet && cliRole != envRole {
		return "", fmt.Errorf("conflicting runtime roles: command-line=%q environment=%q", cliRole, envRole)
	}
	if cliSet {
		return cliRole, nil
	}
	if envSet {
		return envRole, nil
	}
	return RoleAll, nil
}

func parseExplicitRole(value string) (Role, error) {
	role := Role(strings.TrimSpace(value))
	if role == "" {
		return "", errors.New("runtime role cannot be empty")
	}
	if !role.Valid() {
		return "", fmt.Errorf("unknown runtime role %q", value)
	}
	return role, nil
}

type Component struct {
	Name  string
	Roles []Role
	Start func(context.Context) error
	Stop  func(context.Context) error
}

type Lifecycle struct {
	components []Component

	mu      sync.Mutex
	started []Component
}

func NewLifecycle(components []Component) (*Lifecycle, error) {
	copied := make([]Component, len(components))
	copy(copied, components)

	names := make(map[string]struct{}, len(copied))
	for _, component := range copied {
		if strings.TrimSpace(component.Name) == "" {
			return nil, errors.New("lifecycle component name cannot be empty")
		}
		if _, exists := names[component.Name]; exists {
			return nil, fmt.Errorf("duplicate lifecycle component %q", component.Name)
		}
		names[component.Name] = struct{}{}
		if len(component.Roles) == 0 {
			return nil, fmt.Errorf("lifecycle component %q has no role", component.Name)
		}
		for _, role := range component.Roles {
			if !role.Resident() {
				return nil, fmt.Errorf("lifecycle component %q has non-resident role %q", component.Name, role)
			}
		}
		if component.Start == nil {
			return nil, fmt.Errorf("lifecycle component %q has no start function", component.Name)
		}
		if component.Stop == nil {
			return nil, fmt.Errorf("lifecycle component %q has no stop function", component.Name)
		}
	}

	return &Lifecycle{components: copied}, nil
}

func (l *Lifecycle) Start(ctx context.Context, role Role) error {
	if l == nil {
		return errors.New("nil lifecycle")
	}
	if !role.Valid() {
		return fmt.Errorf("unknown runtime role %q", role)
	}
	if !role.Resident() && role != RoleAll {
		return fmt.Errorf("runtime role %q has no resident lifecycle", role)
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	if l.started != nil {
		return errors.New("lifecycle already started")
	}

	for _, component := range l.components {
		if !componentOwnedByRole(component, role) {
			continue
		}
		if err := component.Start(ctx); err != nil {
			rollbackErr := stopComponents(ctx, l.started)
			l.started = nil
			return errors.Join(fmt.Errorf("start lifecycle component %q: %w", component.Name, err), rollbackErr)
		}
		l.started = append(l.started, component)
	}
	return nil
}

func (l *Lifecycle) Stop(ctx context.Context) error {
	if l == nil {
		return nil
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	started := l.started
	l.started = nil
	return stopComponents(ctx, started)
}

func componentOwnedByRole(component Component, role Role) bool {
	if role == RoleAll {
		return true
	}
	for _, componentRole := range component.Roles {
		if componentRole == role {
			return true
		}
	}
	return false
}

func stopComponents(ctx context.Context, components []Component) error {
	var result error
	for index := len(components) - 1; index >= 0; index-- {
		component := components[index]
		if err := component.Stop(ctx); err != nil {
			result = errors.Join(result, fmt.Errorf("stop lifecycle component %q: %w", component.Name, err))
		}
	}
	return result
}
