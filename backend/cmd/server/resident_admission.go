package main

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/runtime"
)

type residentAdmission func(context.Context) error

func admitResidentRole(role runtime.Role, requireBootstrap residentAdmission) error {
	if role == runtime.RoleAll {
		return nil
	}
	return requireBootstrap(context.Background())
}
