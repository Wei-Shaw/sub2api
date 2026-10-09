package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func (c *DingTalkClient) organizationRequest(ctx context.Context, path string, body any, result any) error {
	token, err := c.GetAppToken(ctx)
	if err != nil {
		return err
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.dingTalkOAPIBase()+path+"?access_token="+url.QueryEscape(token), bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, err = io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return err
	}
	var envelope struct {
		ErrCode *int            `json:"errcode"`
		Result  json.RawMessage `json:"result"`
	}
	if err = json.Unmarshal(raw, &envelope); err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK || envelope.ErrCode == nil || *envelope.ErrCode != 0 {
		return parseDingTalkErr(raw, resp.StatusCode)
	}
	if len(envelope.Result) == 0 || string(envelope.Result) == "null" {
		return fmt.Errorf("DingTalk returned an incomplete directory response")
	}
	return json.Unmarshal(envelope.Result, result)
}

// ReadOrganization fetches every department and every cursor page before the
// caller replaces the snapshot. API failures leave the last snapshot intact.
func (c *DingTalkClient) ReadOrganization(ctx context.Context) ([]service.DingTalkDepartment, []service.DingTalkDirectoryMember, error) {
	root, err := c.GetDeptInfo(ctx, 1)
	if err != nil {
		return nil, nil, err
	}
	if root.DeptID != 1 || root.Name == "" {
		return nil, nil, fmt.Errorf("DingTalk root department is missing")
	}
	ds := []service.DingTalkDepartment{{ID: 1, Name: root.Name, ParentID: 0}}
	ms := []service.DingTalkDirectoryMember{}
	visited := map[int64]bool{1: true}
	for index := 0; index < len(ds); index++ {
		if len(ds) > 10000 {
			return nil, nil, fmt.Errorf("DingTalk directory exceeds 10000 departments")
		}
		dept := ds[index]
		var children []struct {
			ID       int64  `json:"dept_id"`
			ParentID int64  `json:"parent_id"`
			Name     string `json:"name"`
		}
		if err = c.organizationRequest(ctx, "/topapi/v2/department/listsub", map[string]any{"dept_id": dept.ID}, &children); err != nil {
			return nil, nil, err
		}
		for _, d := range children {
			if d.ID <= 1 || d.ParentID != dept.ID || d.Name == "" || visited[d.ID] {
				return nil, nil, fmt.Errorf("DingTalk returned an invalid department hierarchy")
			}
			visited[d.ID] = true
			ds = append(ds, service.DingTalkDepartment{ID: d.ID, ParentID: d.ParentID, Name: d.Name})
		}
		cursor := int64(0)
		cursors := map[int64]bool{}
		for {
			if cursors[cursor] {
				return nil, nil, fmt.Errorf("DingTalk member pagination repeated a cursor")
			}
			cursors[cursor] = true
			var page struct {
				HasMore    *bool `json:"has_more"`
				NextCursor int64 `json:"next_cursor"`
				List       []struct {
					UnionID string `json:"unionid"`
					StaffID string `json:"userid"`
					Name    string `json:"name"`
				} `json:"list"`
			}
			if err = c.organizationRequest(ctx, "/topapi/v2/user/list", map[string]any{"dept_id": dept.ID, "cursor": cursor, "size": 100}, &page); err != nil {
				return nil, nil, err
			}
			if page.HasMore == nil || page.List == nil {
				return nil, nil, fmt.Errorf("DingTalk returned an incomplete member page")
			}
			for _, m := range page.List {
				if m.UnionID == "" || m.StaffID == "" {
					return nil, nil, fmt.Errorf("DingTalk member has no union ID; check directory permissions")
				}
				ms = append(ms, service.DingTalkDirectoryMember{DepartmentID: dept.ID, UnionID: m.UnionID, StaffID: m.StaffID, Name: m.Name})
			}
			if len(ms) > 200000 {
				return nil, nil, fmt.Errorf("DingTalk directory exceeds 200000 memberships")
			}
			if !*page.HasMore {
				break
			}
			cursor = page.NextCursor
		}
	}
	return ds, ms, nil
}
