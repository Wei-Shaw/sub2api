package service

import (
	"context"
	"sort"
	"strconv"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/lib/pq"
)

type DingTalkStatisticsApp struct {
	ID          string               `json:"id"`
	Name        string               `json:"name"`
	CompanyID   string               `json:"company_id"`
	Departments []DingTalkDepartment `json:"departments"`
}
type DingTalkUsageRow struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	AppID     string  `json:"app_id,omitempty"`
	CompanyID string  `json:"company_id,omitempty"`
	Members   int     `json:"members"`
	Requests  int64   `json:"requests"`
	Cost      float64 `json:"cost"`
}
type DingTalkStatistics struct {
	Organizations []DingTalkStatisticsApp `json:"organizations"`
	Companies     []DingTalkUsageRow      `json:"companies"`
	Departments   []DingTalkUsageRow      `json:"departments"`
	Users         []DingTalkUsageRow      `json:"users"`
	Total         DingTalkUsageRow        `json:"total"`
}
type dingTalkUserUsage struct {
	Requests int64
	Cost     float64
}
type dingTalkStatisticsDirectory struct {
	App       DingTalkStatisticsApp
	Directory *DingTalkDirectory
}

func (s *DingTalkOrganizationService) Statistics(ctx context.Context, apps []DingTalkStatisticsApp, actor int64, admin bool, start, end time.Time, company, app string, department int64) (*DingTalkStatistics, error) {
	if !start.Before(end) || department < 0 || (department > 0 && app == "") {
		return nil, infraerrors.BadRequest("INVALID_STATISTICS_FILTER", "Invalid time range or department filter")
	}
	directories := []dingTalkStatisticsDirectory{}
	ids := map[int64]bool{}
	for _, a := range apps {
		directory, err := s.Directory(ctx, a.ID, actor, admin)
		if err != nil {
			return nil, err
		}
		if len(directory.Departments) == 0 {
			continue
		}
		a.Departments = directory.Departments
		for _, d := range directory.Departments {
			if d.ID == 1 {
				a.Name = d.Name
				break
			}
		}
		directories = append(directories, dingTalkStatisticsDirectory{a, directory})
		if company != "" && company != a.CompanyID || app != "" && app != a.ID {
			continue
		}
		var scope map[int64]bool
		if department > 0 {
			scope = DingTalkDepartmentScope(directory.Departments, []int64{department})
		}
		for _, m := range directory.Members {
			if m.UserID > 0 && (scope == nil || scope[m.DepartmentID]) {
				ids[m.UserID] = true
			}
		}
	}
	usage := map[int64]dingTalkUserUsage{}
	if len(ids) > 0 {
		userIDs := make([]int64, 0, len(ids))
		for id := range ids {
			userIDs = append(userIDs, id)
		}
		// Aggregate once per platform user before joining any department memberships.
		rows, err := s.db.QueryContext(ctx, `SELECT user_id,COUNT(*),COALESCE(SUM(actual_cost),0) FROM usage_logs WHERE user_id=ANY($1) AND created_at >= $2 AND created_at < $3 GROUP BY user_id`, pq.Array(userIDs), start, end)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		for rows.Next() {
			var id int64
			var u dingTalkUserUsage
			if err = rows.Scan(&id, &u.Requests, &u.Cost); err != nil {
				return nil, err
			}
			usage[id] = u
		}
		if err = rows.Err(); err != nil {
			return nil, err
		}
	}
	return aggregateDingTalkStatistics(directories, usage, company, app, department), nil
}

// Membership reflects the current synced directory. A user's usage is counted
// once within each company/department (including descendants), and once globally.
func aggregateDingTalkStatistics(directories []dingTalkStatisticsDirectory, usage map[int64]dingTalkUserUsage, company, app string, department int64) *DingTalkStatistics {
	result := &DingTalkStatistics{Organizations: []DingTalkStatisticsApp{}, Companies: []DingTalkUsageRow{}, Departments: []DingTalkUsageRow{}, Users: []DingTalkUsageRow{}}
	type bucket struct {
		row   DingTalkUsageRow
		users map[int64]bool
	}
	companies, departments := map[string]*bucket{}, map[string]*bucket{}
	users := map[int64]string{}
	add := func(b *bucket, id int64) {
		if b.users[id] {
			return
		}
		b.users[id] = true
		b.row.Members++
		b.row.Requests += usage[id].Requests
		b.row.Cost += usage[id].Cost
	}
	for _, entry := range directories {
		a, dir := entry.App, entry.Directory
		result.Organizations = append(result.Organizations, a)
		if company != "" && company != a.CompanyID || app != "" && app != a.ID {
			continue
		}
		scope := map[int64]bool{}
		if department > 0 {
			scope = DingTalkDepartmentScope(dir.Departments, []int64{department})
		} else {
			for _, d := range dir.Departments {
				scope[d.ID] = true
			}
		}
		if len(scope) == 0 {
			continue
		}
		if companies[a.CompanyID] == nil {
			companies[a.CompanyID] = &bucket{DingTalkUsageRow{ID: a.CompanyID, Name: a.Name}, map[int64]bool{}}
		}
		byID := map[int64]DingTalkDepartment{}
		for _, d := range dir.Departments {
			byID[d.ID] = d
			if scope[d.ID] {
				key := a.ID + ":" + strconv.FormatInt(d.ID, 10)
				departments[key] = &bucket{DingTalkUsageRow{ID: strconv.FormatInt(d.ID, 10), Name: d.Name, AppID: a.ID, CompanyID: a.CompanyID}, map[int64]bool{}}
			}
		}
		for _, m := range dir.Members {
			if m.UserID <= 0 || !scope[m.DepartmentID] {
				continue
			}
			users[m.UserID] = m.Name
			add(companies[a.CompanyID], m.UserID)
			seen := map[int64]bool{}
			for id := m.DepartmentID; scope[id] && !seen[id]; id = byID[id].ParentID {
				seen[id] = true
				add(departments[a.ID+":"+strconv.FormatInt(id, 10)], m.UserID)
			}
		}
	}
	for _, b := range companies {
		result.Companies = append(result.Companies, b.row)
	}
	for _, b := range departments {
		result.Departments = append(result.Departments, b.row)
	}
	for id, name := range users {
		u := usage[id]
		result.Users = append(result.Users, DingTalkUsageRow{ID: strconv.FormatInt(id, 10), Name: name, Members: 1, Requests: u.Requests, Cost: u.Cost})
		result.Total.Members++
		result.Total.Requests += u.Requests
		result.Total.Cost += u.Cost
	}
	for _, rows := range [][]DingTalkUsageRow{result.Companies, result.Departments, result.Users} {
		sort.Slice(rows, func(i, j int) bool {
			if rows[i].Cost != rows[j].Cost {
				return rows[i].Cost > rows[j].Cost
			}
			if rows[i].AppID != rows[j].AppID {
				return rows[i].AppID < rows[j].AppID
			}
			return rows[i].ID < rows[j].ID
		})
	}
	return result
}
