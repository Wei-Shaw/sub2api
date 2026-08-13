package main

import (
	"fmt"

	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/runtime"
)

type applicationInitializer func(handler.BuildInfo) (*Application, error)

type applicationInitializers struct {
	all       applicationInitializer
	api       applicationInitializer
	worker    applicationInitializer
	scheduler applicationInitializer
}

func initializeApplicationForRole(role runtime.Role, buildInfo handler.BuildInfo, initializers applicationInitializers) (*Application, error) {
	var initialize applicationInitializer
	switch role {
	case runtime.RoleAll:
		initialize = initializers.all
	case runtime.RoleAPI:
		initialize = initializers.api
	case runtime.RoleWorker:
		initialize = initializers.worker
	case runtime.RoleScheduler:
		initialize = initializers.scheduler
	default:
		return nil, fmt.Errorf("role %q does not have a resident application graph", role)
	}
	if initialize == nil {
		return nil, fmt.Errorf("role %q application graph is not configured", role)
	}
	return initialize(buildInfo)
}

func initializeApplication(buildInfo handler.BuildInfo, role runtime.Role) (*Application, error) {
	return initializeApplicationForRole(role, buildInfo, applicationInitializers{
		all:       initializeAllApplication,
		api:       initializeAPIApplication,
		worker:    initializeWorkerApplication,
		scheduler: initializeSchedulerApplication,
	})
}
