package service

// APIKeyWithResolvedGroup returns a request-scoped API key copy bound to group.
// The original multi-group binding remains on GroupIDs/Groups; GroupID/Group
// represent the concrete group used for routing, billing, and logging.
func APIKeyWithResolvedGroup(apiKey *APIKey, group *Group) *APIKey {
	if apiKey == nil || group == nil {
		return apiKey
	}
	cloned := *apiKey
	groupCopy := *group
	groupID := groupCopy.ID
	cloned.GroupID = &groupID
	cloned.Group = &groupCopy
	if len(cloned.GroupIDs) == 0 {
		cloned.GroupIDs = []int64{groupID}
	}
	if cloned.User != nil && cloned.User.UserGroupRPMOverrides != nil {
		userCopy := *cloned.User
		userCopy.UserGroupRPMOverride = cloned.User.UserGroupRPMOverrides[groupID]
		cloned.User = &userCopy
	}
	return &cloned
}
