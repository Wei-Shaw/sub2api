package main

import (
	"fmt"

	"github.com/Wei-Shaw/sub2api/internal/runtime"
)

type roleLaunchers struct {
	runCLISetup      func() error
	needsSetup       func() bool
	autoSetupEnabled func() bool
	runAutoSetup     func() error
	runSetupServer   func()
	runResident      func(runtime.Role) error
	runMigrate       func() error
	runBootstrap     func() error
}

func dispatchRole(role runtime.Role, setupMode bool, launchers roleLaunchers) error {
	if setupMode {
		if role != runtime.RoleAll {
			return fmt.Errorf("--setup requires runtime role %q", runtime.RoleAll)
		}
		return launchers.runCLISetup()
	}

	switch role {
	case runtime.RoleAll:
		if launchers.needsSetup() {
			if launchers.autoSetupEnabled() {
				if err := launchers.runAutoSetup(); err != nil {
					return err
				}
				return launchers.runResident(role)
			}
			launchers.runSetupServer()
			return nil
		}
		return launchers.runResident(role)
	case runtime.RoleAPI, runtime.RoleWorker, runtime.RoleScheduler:
		return launchers.runResident(role)
	case runtime.RoleMigrate:
		return launchers.runMigrate()
	case runtime.RoleBootstrap:
		return launchers.runBootstrap()
	default:
		return fmt.Errorf("unsupported runtime role %q", role)
	}
}
