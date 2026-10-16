package service

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestDingTalkStatisticsDeduplicatesAndScopesRankings(t *testing.T) {
	ds := []DingTalkDepartment{{ID: 1, Name: "Corp"}, {ID: 2, ParentID: 1, Name: "Team"}, {ID: 3, ParentID: 2, Name: "Child"}, {ID: 4, ParentID: 1, Name: "Other"}}
	entries := []dingTalkStatisticsDirectory{
		{DingTalkStatisticsApp{ID: "a", Name: "Corp", CompanyID: "corp", Departments: ds}, &DingTalkDirectory{Departments: ds, Members: []DingTalkDirectoryMember{{DepartmentID: 2, UserID: 1, Name: "A"}, {DepartmentID: 3, UserID: 1, Name: "A"}, {DepartmentID: 3, UserID: 2, Name: "B"}, {DepartmentID: 4, UserID: 3, Name: "C"}, {DepartmentID: 3, Name: "Unlinked"}}}},
		{DingTalkStatisticsApp{ID: "b", Name: "Same corporation", CompanyID: "corp", Departments: ds}, &DingTalkDirectory{Departments: ds, Members: []DingTalkDirectoryMember{{DepartmentID: 3, UserID: 1, Name: "A"}}}},
		{DingTalkStatisticsApp{ID: "c", Name: "Other corporation", CompanyID: "other", Departments: ds}, &DingTalkDirectory{Departments: ds, Members: []DingTalkDirectoryMember{{DepartmentID: 3, UserID: 1, Name: "A"}}}},
	}
	usage := map[int64]dingTalkUserUsage{1: {Requests: 2, Cost: 1.25}, 2: {Requests: 4, Cost: 10.5}, 3: {Requests: 1, Cost: 3}}
	result := aggregateDingTalkStatistics(entries, usage, "", "", 0)
	require.Equal(t, 3, result.Total.Members)
	require.EqualValues(t, 7, result.Total.Requests)
	require.InDelta(t, 14.75, result.Total.Cost, 0.000001)
	require.Len(t, result.Companies, 2)
	require.InDelta(t, 14.75, result.Companies[0].Cost, 0.000001)
	require.Equal(t, "2", result.Users[0].ID)
	for _, row := range result.Departments {
		if row.AppID == "a" && row.ID == "2" {
			require.InDelta(t, 11.75, row.Cost, 0.000001)
			require.Equal(t, 2, row.Members)
		}
	}
	filtered := aggregateDingTalkStatistics(entries, usage, "corp", "a", 2)
	require.Len(t, filtered.Companies, 1)
	require.Len(t, filtered.Departments, 2)
	require.Len(t, filtered.Users, 2)
	require.InDelta(t, 11.75, filtered.Total.Cost, 0.000001)
	require.Empty(t, aggregateDingTalkStatistics(entries, usage, "corp", "a", 99).Users)
	require.Empty(t, aggregateDingTalkStatistics(entries, usage, "unknown", "", 0).Companies)
}

func TestDingTalkStatisticsNoAuthorityDoesNotReadUsage(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	mock.ExpectQuery("SELECT synced_at").WithArgs("a").WillReturnRows(sqlmock.NewRows([]string{"synced_at"}).AddRow(time.Now()))
	mock.ExpectQuery("SELECT department_id,parent_id,name").WithArgs("a").WillReturnRows(sqlmock.NewRows([]string{"department_id", "parent_id", "name"}).AddRow(1, 0, "Company"))
	mock.ExpectQuery("SELECT b.user_id").WithArgs(false, int64(9)).WillReturnRows(sqlmock.NewRows([]string{"user_id", "username", "limit_cents", "used_cents", "enabled"}))
	s := NewDingTalkOrganizationService(db, nil)
	result, err := s.Statistics(context.Background(), []DingTalkStatisticsApp{{ID: "a", CompanyID: "corp"}}, 9, false, time.Now().Add(-time.Hour), time.Now(), "", "", 0)
	require.NoError(t, err)
	require.Empty(t, result.Organizations)
	require.Empty(t, result.Users)
	require.NoError(t, mock.ExpectationsWereMet())
}
