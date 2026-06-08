package pluginsdk

import pb "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"

func platformsToProto(decls []PlatformDecl) []*pb.PlatformDeclaration {
	if len(decls) == 0 {
		return nil
	}
	out := make([]*pb.PlatformDeclaration, len(decls))
	for i := range decls {
		out[i] = platformDeclToProto(&decls[i])
	}
	return out
}

func platformDeclToProto(d *PlatformDecl) *pb.PlatformDeclaration {
	p := &pb.PlatformDeclaration{
		Platform:           d.Platform,
		DisplayName:        d.DisplayName,
		IconSvg:            d.IconSVG,
		ThemeColor:         d.ThemeColor,
		AccountTypes:       accountTypesToProto(d.AccountTypes),
		CustomActions:      customActionsToProto(d.CustomActions),
		SortOrder:          int32(d.SortOrder),
		PrivacyStates:      privacyStatesToProto(d.PrivacyStates),
		CompatibleGateways: append([]string(nil), d.CompatibleGateways...),
		FrontendMeta:       append([]byte(nil), d.FrontendMeta...),
	}
	if d.CapacityDisplay != nil {
		p.CapacityDisplay = &pb.CapacityDisplayConfig{
			ShowConcurrency: d.CapacityDisplay.ShowConcurrency,
			ExtraRows:       displayRowsToProto(d.CapacityDisplay.ExtraRows),
		}
	}
	if d.UsageDisplay != nil {
		p.UsageDisplay = &pb.UsageDisplayConfig{
			ComponentPath: d.UsageDisplay.ComponentPath,
			WindowLabel:   d.UsageDisplay.WindowLabel,
			ShowReqCount:  d.UsageDisplay.ShowReqCount,
			ShowCost:      d.UsageDisplay.ShowCost,
			ExtraRows:     displayRowsToProto(d.UsageDisplay.ExtraRows),
		}
	}
	if d.TestConfig != nil {
		p.TestConfig = &pb.TestConnectionConfig{
			ModelSelector:      d.TestConfig.ModelSelector,
			TestComponentPath:  d.TestConfig.TestComponentPath,
			DefaultTestModel:   d.TestConfig.DefaultTestModel,
			TestModes:          testModesToProto(d.TestConfig.TestModes),
			ImageModelPatterns: d.TestConfig.ImageModelPatterns,
			PrioritizedModels:  d.TestConfig.PrioritizedModels,
		}
	}
	if d.GroupConfig != nil {
		p.GroupConfig = &pb.GroupConfigDeclaration{
			GroupExtraSchema:  d.GroupConfig.GroupExtraSchema,
			FormComponentPath: d.GroupConfig.FormComponentPath,
			FrontendMeta:      append([]byte(nil), d.GroupConfig.FrontendMeta...),
		}
	}
	return p
}

func accountTypesToProto(types []AccountTypeDecl) []*pb.AccountTypeDeclaration {
	if len(types) == 0 {
		return nil
	}
	out := make([]*pb.AccountTypeDeclaration, len(types))
	for i := range types {
		t := &types[i]
		out[i] = &pb.AccountTypeDeclaration{
			Type:              t.Type,
			DisplayName:       t.DisplayName,
			Description:       t.Description,
			IconSvg:           t.IconSVG,
			ThemeColor:        t.ThemeColor,
			CredentialSchema:  append([]byte(nil), t.CredentialSchema...),
			ExtraSchema:       append([]byte(nil), t.ExtraSchema...),
			FormComponentPath: t.FormComponentPath,
			SubTypes:          subTypesToProto(t.SubTypes),
			SortOrder:         int32(t.SortOrder),
			BadgeLabel:        t.BadgeLabel,
			FrontendMeta:      append([]byte(nil), t.FrontendMeta...),
		}
	}
	return out
}

func subTypesToProto(opts []SubTypeOption) []*pb.SubTypeOption {
	if len(opts) == 0 {
		return nil
	}
	out := make([]*pb.SubTypeOption, len(opts))
	for i := range opts {
		out[i] = &pb.SubTypeOption{Value: opts[i].Value, Label: opts[i].Label}
	}
	return out
}

func displayRowsToProto(rows []DisplayRow) []*pb.DisplayRow {
	if len(rows) == 0 {
		return nil
	}
	out := make([]*pb.DisplayRow, len(rows))
	for i := range rows {
		out[i] = &pb.DisplayRow{
			Label:  rows[i].Label,
			Source: rows[i].Source,
			Format: rows[i].Format,
		}
	}
	return out
}

func customActionsToProto(actions []CustomActionDecl) []*pb.CustomActionDeclaration {
	if len(actions) == 0 {
		return nil
	}
	out := make([]*pb.CustomActionDeclaration, len(actions))
	for i := range actions {
		a := &actions[i]
		out[i] = &pb.CustomActionDeclaration{
			ActionId:      a.ActionID,
			IconSvg:       a.IconSVG,
			Labels:        a.Labels,
			ActionType:    a.ActionType,
			ApiEndpoint:   a.APIEndpoint,
			ComponentPath: a.ComponentPath,
			SortOrder:     int32(a.SortOrder),
		}
	}
	return out
}

func privacyStatesToProto(states []PrivacyStateDecl) []*pb.PrivacyState {
	if len(states) == 0 {
		return nil
	}
	out := make([]*pb.PrivacyState, len(states))
	for i := range states {
		out[i] = &pb.PrivacyState{
			Value:       states[i].Value,
			DisplayName: states[i].DisplayName,
			BadgeColor:  states[i].BadgeColor,
			IsSet:       states[i].IsSet,
		}
	}
	return out
}

// PlatformDeclFromProto converts a proto PlatformDeclaration back to Go type.
// Used by the host when reading manifests from loaded plugins.
func PlatformDeclFromProto(p *pb.PlatformDeclaration) PlatformDecl {
	if p == nil {
		return PlatformDecl{}
	}
	d := PlatformDecl{
		Platform:           p.Platform,
		DisplayName:        p.DisplayName,
		IconSVG:            p.IconSvg,
		ThemeColor:         p.ThemeColor,
		SortOrder:          int(p.SortOrder),
		CompatibleGateways: append([]string(nil), p.CompatibleGateways...),
		FrontendMeta:       append([]byte(nil), p.FrontendMeta...),
	}
	for _, at := range p.AccountTypes {
		d.AccountTypes = append(d.AccountTypes, accountTypeDeclFromProto(at))
	}
	if p.CapacityDisplay != nil {
		d.CapacityDisplay = &CapacityDisplayConfig{
			ShowConcurrency: p.CapacityDisplay.ShowConcurrency,
			ExtraRows:       displayRowsFromProto(p.CapacityDisplay.ExtraRows),
		}
	}
	if p.UsageDisplay != nil {
		d.UsageDisplay = &UsageDisplayConfig{
			ComponentPath: p.UsageDisplay.ComponentPath,
			WindowLabel:   p.UsageDisplay.WindowLabel,
			ShowReqCount:  p.UsageDisplay.ShowReqCount,
			ShowCost:      p.UsageDisplay.ShowCost,
			ExtraRows:     displayRowsFromProto(p.UsageDisplay.ExtraRows),
		}
	}
	for _, ca := range p.CustomActions {
		d.CustomActions = append(d.CustomActions, customActionFromProto(ca))
	}
	if p.TestConfig != nil {
		d.TestConfig = &TestConnectionConfig{
			ModelSelector:      p.TestConfig.ModelSelector,
			TestComponentPath:  p.TestConfig.TestComponentPath,
			DefaultTestModel:   p.TestConfig.DefaultTestModel,
			TestModes:          testModesFromProto(p.TestConfig.TestModes),
			ImageModelPatterns: p.TestConfig.ImageModelPatterns,
			PrioritizedModels:  p.TestConfig.PrioritizedModels,
		}
	}
	for _, ps := range p.PrivacyStates {
		d.PrivacyStates = append(d.PrivacyStates, PrivacyStateDecl{
			Value:       ps.Value,
			DisplayName: ps.DisplayName,
			BadgeColor:  ps.BadgeColor,
			IsSet:       ps.IsSet,
		})
	}
	if p.GroupConfig != nil {
		d.GroupConfig = &GroupConfigDecl{
			GroupExtraSchema:  append([]byte(nil), p.GroupConfig.GroupExtraSchema...),
			FormComponentPath: p.GroupConfig.FormComponentPath,
			FrontendMeta:      append([]byte(nil), p.GroupConfig.FrontendMeta...),
		}
	}
	return d
}

func accountTypeDeclFromProto(p *pb.AccountTypeDeclaration) AccountTypeDecl {
	if p == nil {
		return AccountTypeDecl{}
	}
	d := AccountTypeDecl{
		Type:              p.Type,
		DisplayName:       p.DisplayName,
		Description:       p.Description,
		IconSVG:           p.IconSvg,
		ThemeColor:        p.ThemeColor,
		CredentialSchema:  append([]byte(nil), p.CredentialSchema...),
		ExtraSchema:       append([]byte(nil), p.ExtraSchema...),
		FormComponentPath: p.FormComponentPath,
		SortOrder:         int(p.SortOrder),
		BadgeLabel:        p.BadgeLabel,
		FrontendMeta:      append([]byte(nil), p.FrontendMeta...),
	}
	for _, st := range p.SubTypes {
		d.SubTypes = append(d.SubTypes, SubTypeOption{Value: st.Value, Label: st.Label})
	}
	return d
}

func displayRowsFromProto(rows []*pb.DisplayRow) []DisplayRow {
	if len(rows) == 0 {
		return nil
	}
	out := make([]DisplayRow, len(rows))
	for i, r := range rows {
		out[i] = DisplayRow{Label: r.Label, Source: r.Source, Format: r.Format}
	}
	return out
}

func customActionFromProto(p *pb.CustomActionDeclaration) CustomActionDecl {
	if p == nil {
		return CustomActionDecl{}
	}
	return CustomActionDecl{
		ActionID:      p.ActionId,
		IconSVG:       p.IconSvg,
		Labels:        p.Labels,
		ActionType:    p.ActionType,
		APIEndpoint:   p.ApiEndpoint,
		ComponentPath: p.ComponentPath,
		SortOrder:     int(p.SortOrder),
	}
}

func testModesToProto(modes []TestModeOption) []*pb.TestModeOption {
	if len(modes) == 0 {
		return nil
	}
	out := make([]*pb.TestModeOption, len(modes))
	for i := range modes {
		out[i] = &pb.TestModeOption{Value: modes[i].Value, Label: modes[i].Label}
	}
	return out
}

func testModesFromProto(modes []*pb.TestModeOption) []TestModeOption {
	if len(modes) == 0 {
		return nil
	}
	out := make([]TestModeOption, len(modes))
	for i, m := range modes {
		out[i] = TestModeOption{Value: m.Value, Label: m.Label}
	}
	return out
}
