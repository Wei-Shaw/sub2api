package upstreamstation

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

type ConnectorRegistry struct {
	ordered []Connector
	byType  map[string]Connector
}

func NewConnectorRegistry(connectors ...Connector) *ConnectorRegistry {
	registry := &ConnectorRegistry{byType: make(map[string]Connector, len(connectors))}
	for _, connector := range connectors {
		if connector == nil {
			continue
		}
		typeName := strings.ToLower(strings.TrimSpace(connector.Type()))
		if typeName == "" {
			continue
		}
		registry.ordered = append(registry.ordered, connector)
		registry.byType[typeName] = connector
	}
	return registry
}

func (r *ConnectorRegistry) Resolve(ctx context.Context, station *Station) (Connector, error) {
	if station == nil {
		return nil, errors.New("upstream station is required")
	}
	if r == nil {
		return nil, errors.New("upstream connector registry is required")
	}

	typeName := strings.ToLower(strings.TrimSpace(station.SiteType))
	if typeName != "" && typeName != SiteTypeAuto {
		connector, ok := r.byType[typeName]
		if !ok {
			return nil, fmt.Errorf("unsupported upstream station type %q", station.SiteType)
		}
		return connector, nil
	}

	for _, connector := range r.ordered {
		detected, err := connector.Detect(ctx, station.BaseURL)
		if err != nil {
			continue
		}
		if detected {
			return connector, nil
		}
	}
	return nil, errors.New("upstream station type could not be detected")
}

func (r *ConnectorRegistry) FirstModelDiscoverer() Connector {
	if r == nil {
		return nil
	}
	for _, connector := range r.ordered {
		if _, ok := connector.(ModelDiscoverer); ok {
			return connector
		}
	}
	return nil
}
