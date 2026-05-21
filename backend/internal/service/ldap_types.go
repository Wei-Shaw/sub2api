package service

import "time"

// LDAPGroupMapping maps an LDAP group DN to Sub2API entitlements.
type LDAPGroupMapping struct {
	LDAPGroupDN string  `json:"ldap_group_dn"`
	TargetRole  string  `json:"target_role"`
	Balance     float64 `json:"balance"`
	Concurrency int     `json:"concurrency"`
	Priority    int     `json:"priority"`
}

// LDAPConfig defines runtime LDAP settings loaded from DB settings.
type LDAPConfig struct {
	Enabled            bool
	Host               string
	Port               int
	UseTLS             bool
	StartTLS           bool
	InsecureSkipVerify bool
	BindDN             string
	BindPassword       string
	UserBaseDN         string
	UserFilter         string
	LoginAttr          string
	UIDAttr            string
	EmailAttr          string
	DisplayNameAttr    string
	DepartmentAttr     string
	GroupAttr          string
	AllowedGroupDNs    []string
	GroupMappings      []LDAPGroupMapping
	SyncEnabled        bool
	SyncIntervalMins   int
}

// LDAPIdentity is the resolved LDAP user record after successful lookup.
type LDAPIdentity struct {
	UID         string
	Username    string
	Email       string
	DisplayName string
	Department  string
	GroupDNs    []string
	Disabled    bool
}

// LDAPUserProfile stores persisted LDAP linkage metadata in local DB.
type LDAPUserProfile struct {
	UserID       int64
	LDAPUID      string
	LDAPUsername string
	LDAPEmail    string
	DisplayName  string
	Department   string
	GroupsHash   string
	Active       bool
	LastSyncedAt time.Time
}

// LDAPSyncTarget describes a local LDAP-backed user to revalidate.
type LDAPSyncTarget struct {
	UserID       int64
	Email        string
	LDAPUID      string
	LDAPUsername string
}
