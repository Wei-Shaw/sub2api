package service

import "context"

// ResolveAPIKeyGroupFallback keeps an API key on its primary group whenever
// that group has a schedulable account for the requested model. When it does
// not, the ordered per-key fallback list is checked from left to right.
//
// The returned key is a request-local shallow copy. Persisted key assignment
// never changes, so the next request automatically returns to the primary
// group as soon as it recovers.
func (s *OpenAIGatewayService) ResolveAPIKeyGroupFallback(ctx context.Context, apiKey *APIKey, requestedModel string) (*APIKey, bool) {
	if s == nil || s.schedulerSnapshot == nil || apiKey == nil || len(apiKey.FallbackGroupIDs) == 0 {
		return apiKey, false
	}

	for index, groupID := range apiKey.OrderedGroupIDs() {
		var group *Group
		if index == 0 && apiKey.Group != nil && apiKey.Group.ID == groupID {
			group = apiKey.Group
		} else {
			resolved, err := s.schedulerSnapshot.GetGroupByID(ctx, groupID)
			if err != nil {
				continue
			}
			group = resolved
		}
		if group == nil || group.Status != StatusActive {
			continue
		}

		candidateID := group.ID
		platform := normalizeOpenAICompatiblePlatform(group.Platform)
		if _, err := s.selectAccountForModelWithExclusions(
			s.withOpenAIQuotaAutoPauseContext(ctx),
			&candidateID,
			platform,
			"",
			requestedModel,
			nil,
			false,
			0,
			"",
			false,
		); err != nil {
			continue
		}
		if index == 0 {
			return apiKey, false
		}

		clone := *apiKey
		clone.GroupID = &candidateID
		clone.Group = group
		return &clone, true
	}
	return apiKey, false
}
