package service

import (
	"context"
	"fmt"
	"strings"
)

func (s *adminServiceImpl) ListCompositeRouteSchemes(ctx context.Context) ([]CompositeRouteScheme, error) {
	if s.compositeRouteRepo == nil {
		return nil, fmt.Errorf("composite route repository is not configured")
	}
	return s.compositeRouteRepo.ListSchemes(ctx)
}

func (s *adminServiceImpl) GetCompositeRouteScheme(ctx context.Context, id int64) (*CompositeRouteScheme, error) {
	if s.compositeRouteRepo == nil {
		return nil, fmt.Errorf("composite route repository is not configured")
	}
	return s.compositeRouteRepo.GetScheme(ctx, id)
}

func (s *adminServiceImpl) CreateCompositeRouteScheme(ctx context.Context, input CompositeRouteSchemeInput) (*CompositeRouteScheme, error) {
	if s.compositeRouteRepo == nil {
		return nil, fmt.Errorf("composite route repository is not configured")
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}
	scheme := &CompositeRouteScheme{
		Name:        name,
		Description: strings.TrimSpace(input.Description),
	}
	if err := s.compositeRouteRepo.CreateScheme(ctx, scheme); err != nil {
		return nil, err
	}
	if input.CopyFromSchemeID > 0 {
		if err := s.copySchemeRoutes(ctx, input.CopyFromSchemeID, scheme.ID); err != nil {
			return nil, err
		}
		hydrated, err := s.compositeRouteRepo.GetScheme(ctx, scheme.ID)
		if err != nil {
			return nil, err
		}
		return hydrated, nil
	}
	return scheme, nil
}

func (s *adminServiceImpl) UpdateCompositeRouteScheme(ctx context.Context, id int64, input CompositeRouteSchemeInput) (*CompositeRouteScheme, error) {
	if s.compositeRouteRepo == nil {
		return nil, fmt.Errorf("composite route repository is not configured")
	}
	scheme, err := s.compositeRouteRepo.GetScheme(ctx, id)
	if err != nil {
		return nil, err
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}
	scheme.Name = name
	scheme.Description = strings.TrimSpace(input.Description)
	if err := s.compositeRouteRepo.UpdateScheme(ctx, scheme); err != nil {
		return nil, err
	}
	return scheme, nil
}

func (s *adminServiceImpl) DeleteCompositeRouteScheme(ctx context.Context, id int64) error {
	if s.compositeRouteRepo == nil {
		return fmt.Errorf("composite route repository is not configured")
	}
	if _, err := s.compositeRouteRepo.GetScheme(ctx, id); err != nil {
		return err
	}
	inUse, err := s.compositeRouteRepo.CountGroupsByScheme(ctx, id)
	if err != nil {
		return err
	}
	if inUse > 0 {
		return ErrCompositeRouteSchemeInUse
	}
	if err := s.compositeRouteRepo.DeleteByScheme(ctx, id); err != nil {
		return err
	}
	return s.compositeRouteRepo.DeleteScheme(ctx, id)
}

func (s *adminServiceImpl) DuplicateCompositeRouteScheme(ctx context.Context, id int64, name string) (*CompositeRouteScheme, error) {
	source, err := s.GetCompositeRouteScheme(ctx, id)
	if err != nil {
		return nil, err
	}
	copyName := strings.TrimSpace(name)
	if copyName == "" {
		copyName = source.Name + " (Copy)"
	}
	return s.CreateCompositeRouteScheme(ctx, CompositeRouteSchemeInput{
		Name:             copyName,
		Description:      source.Description,
		CopyFromSchemeID: source.ID,
	})
}

func (s *adminServiceImpl) ListCompositeRouteSchemeRoutes(ctx context.Context, schemeID int64) ([]CompositeModelRoute, error) {
	if _, err := s.GetCompositeRouteScheme(ctx, schemeID); err != nil {
		return nil, err
	}
	return s.compositeRouteRepo.ListByScheme(ctx, schemeID, true)
}

func (s *adminServiceImpl) CreateCompositeRouteSchemeRoute(ctx context.Context, schemeID int64, input CompositeRouteInput) (*CompositeModelRoute, error) {
	if _, err := s.GetCompositeRouteScheme(ctx, schemeID); err != nil {
		return nil, err
	}
	route, err := compositeRouteFromInput(schemeID, input)
	if err != nil {
		return nil, err
	}
	if err := s.compositeRouteRepo.Create(ctx, route); err != nil {
		return nil, err
	}
	return route, nil
}

func (s *adminServiceImpl) UpdateCompositeRouteSchemeRoute(ctx context.Context, schemeID, routeID int64, input CompositeRouteInput) (*CompositeModelRoute, error) {
	if _, err := s.GetCompositeRouteScheme(ctx, schemeID); err != nil {
		return nil, err
	}
	if ok, err := s.compositeRouteBelongsToScheme(ctx, schemeID, routeID); err != nil {
		return nil, err
	} else if !ok {
		return nil, ErrCompositeRouteNotFound
	}
	route, err := compositeRouteFromInput(schemeID, input)
	if err != nil {
		return nil, err
	}
	route.ID = routeID
	if err := s.compositeRouteRepo.Update(ctx, route); err != nil {
		return nil, err
	}
	return route, nil
}

func (s *adminServiceImpl) DeleteCompositeRouteSchemeRoute(ctx context.Context, schemeID, routeID int64) error {
	if _, err := s.GetCompositeRouteScheme(ctx, schemeID); err != nil {
		return err
	}
	if ok, err := s.compositeRouteBelongsToScheme(ctx, schemeID, routeID); err != nil {
		return err
	} else if !ok {
		return ErrCompositeRouteNotFound
	}
	return s.compositeRouteRepo.Delete(ctx, routeID)
}

func (s *adminServiceImpl) PreviewCompositeRouteScheme(ctx context.Context, schemeID int64, input CompositeRoutePreviewRequest) (*CompositeRouteDecision, error) {
	if _, err := s.GetCompositeRouteScheme(ctx, schemeID); err != nil {
		return nil, err
	}
	if s.compositeRouteRepo == nil {
		return nil, fmt.Errorf("composite route repository is not configured")
	}
	model := strings.TrimSpace(input.Model)
	endpoint := normalizeCompositeRouteEndpoint(input.Endpoint)
	decision := CompositeRouteDecision{
		PublicModel: model,
		Endpoint:    endpoint,
	}
	if model == "" {
		decision.Reason = "model is required"
		return &decision, nil
	}
	routes, err := s.compositeRouteRepo.ListByScheme(ctx, schemeID, false)
	if err != nil {
		return nil, err
	}
	if route, ok := matchCompositeRoute(routes, model, endpoint); ok {
		upstreamModel := strings.TrimSpace(route.UpstreamModel)
		if upstreamModel == "" {
			upstreamModel = model
		}
		return &CompositeRouteDecision{
			Matched:        true,
			Source:         CompositeRouteSourceExplicit,
			PublicModel:    model,
			TargetPlatform: route.TargetPlatform,
			UpstreamModel:  upstreamModel,
			Endpoint:       endpoint,
			Route:          &route,
		}, nil
	}
	if platform, ok := DetectModelPlatform(model); ok {
		return &CompositeRouteDecision{
			Matched:        true,
			Source:         CompositeRouteSourceDetector,
			PublicModel:    model,
			TargetPlatform: platform,
			UpstreamModel:  model,
			Endpoint:       endpoint,
		}, nil
	}
	decision.Reason = "no explicit route or built-in detector match"
	return &decision, nil
}

func (s *adminServiceImpl) compositeRouteBelongsToScheme(ctx context.Context, schemeID, routeID int64) (bool, error) {
	route, err := s.compositeRouteRepo.GetByID(ctx, routeID)
	if err != nil {
		return false, err
	}
	return route.SchemeID == schemeID, nil
}

func (s *adminServiceImpl) copySchemeRoutes(ctx context.Context, fromID, toID int64) error {
	if fromID == toID {
		return nil
	}
	if _, err := s.compositeRouteRepo.GetScheme(ctx, fromID); err != nil {
		return err
	}
	routes, err := s.compositeRouteRepo.ListByScheme(ctx, fromID, true)
	if err != nil {
		return err
	}
	for i := range routes {
		clone := routes[i]
		clone.ID = 0
		clone.SchemeID = toID
		clone.GroupID = 0
		if err := s.compositeRouteRepo.Create(ctx, &clone); err != nil {
			return err
		}
	}
	return nil
}
