package domain

import "sort"

// SmartRoutingMember 描述智能路由分组的一个成员分组配置。
//
// Priority 为调度优先级，数值越小优先级越高（1 为最高）。请求先发给
// 最高优先级层内的某个成员分组；该分组调度/上游失败后，按优先级从高到
// 低逐层重试，直到成功或所有候选耗尽。
//
// Weight 为同优先级层内的流量权重（>=1）。同一层内能提供当前模型的多个
// 成员分组按权重做加权随机排列：首个候选按权重比例选出，失败后同层其余
// 候选仍按剩余权重继续加权选择。
type SmartRoutingMember struct {
	GroupID  int64 `json:"group_id"`
	Priority int   `json:"priority"`
	Weight   int   `json:"weight"`
}

// SmartRoutingMaxMembers 限制单个智能路由分组可配置的成员数量，
// 防止异常配置导致单请求候选链过长。
const SmartRoutingMaxMembers = 32

// NormalizeSmartRoutingMembers 清洗成员配置：丢弃非法条目、按 group_id
// 去重（保留先出现者）、强制优先级与权重下限，并按优先级稳定排序。
// 返回的切片可安全直接序列化存储。
func NormalizeSmartRoutingMembers(members []SmartRoutingMember) []SmartRoutingMember {
	if len(members) == 0 {
		return nil
	}
	seen := make(map[int64]struct{}, len(members))
	out := make([]SmartRoutingMember, 0, len(members))
	for _, m := range members {
		if m.GroupID <= 0 {
			continue
		}
		if _, dup := seen[m.GroupID]; dup {
			continue
		}
		seen[m.GroupID] = struct{}{}
		if m.Priority <= 0 {
			m.Priority = 1
		}
		if m.Weight <= 0 {
			m.Weight = 1
		}
		out = append(out, m)
	}
	if len(out) > SmartRoutingMaxMembers {
		out = out[:SmartRoutingMaxMembers]
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Priority != out[j].Priority {
			return out[i].Priority < out[j].Priority
		}
		return out[i].GroupID < out[j].GroupID
	})
	return out
}
