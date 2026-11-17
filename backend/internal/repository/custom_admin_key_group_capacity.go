package repository

import (
	"context"

	"github.com/Wei-Shaw/sub2api/ent/group"
	"github.com/Wei-Shaw/sub2api/ent/predicate"
	"github.com/Wei-Shaw/sub2api/internal/pkg/adminaccess"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// ListScopedAdminCapacityGroupIDs 先在数据库内限定分组，再查询账户与实时容量。
func (r *groupRepository) ListScopedAdminCapacityGroupIDs(ctx context.Context) ([]int64, error) {
	p := adminaccess.FromContext(ctx)
	if p == nil {
		return nil, infraerrors.Forbidden("SCOPED_GROUP_CAPACITY_FORBIDDEN", "缺少受限身份")
	}
	alternatives := []predicate.Group{}
	for _, rule := range p.Policy.RulesFor("groups.capacity.read") {
		terms := []predicate.Group{group.IDGT(0)}
		if !rule.AllGroups {
			if len(rule.GroupIDs) == 0 {
				continue
			}
			terms = append(terms, group.IDIn(rule.GroupIDs...))
		}
		if len(rule.Platforms) > 0 {
			terms = append(terms, group.PlatformIn(rule.Platforms...))
		}
		alternatives = append(alternatives, group.And(terms...))
	}
	if len(alternatives) == 0 {
		return []int64{}, nil
	}
	return r.client.Group.Query().Where(group.DeletedAtIsNil(), group.StatusEQ(service.StatusActive), group.Or(alternatives...)).Order(group.ByID()).IDs(ctx)
}
