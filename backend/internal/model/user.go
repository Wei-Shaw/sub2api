package model

import (
	"time"

	"github.com/lib/pq"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type User struct {
	ID            int64          `gorm:"primaryKey" json:"id"`
	Email         string         `gorm:"uniqueIndex;size:255;not null" json:"email"`
	Username      string         `gorm:"size:100;default:''" json:"username"`
	Wechat        string         `gorm:"size:100;default:''" json:"wechat"`
	Notes         string         `gorm:"type:text;default:''" json:"notes"`
	PasswordHash  string         `gorm:"size:255;not null" json:"-"`
	Role          string         `gorm:"size:20;default:user;not null" json:"role"` // admin/user
	Balance       float64        `gorm:"type:decimal(20,8);default:0;not null" json:"balance"`
	Concurrency   int            `gorm:"default:5;not null" json:"concurrency"`
	Status        string         `gorm:"size:20;default:active;not null" json:"status"` // active/disabled
	AllowedGroups pq.Int64Array  `gorm:"type:bigint[]" json:"allowed_groups"`
	CreatedAt     time.Time      `gorm:"not null" json:"created_at"`
	UpdatedAt     time.Time      `gorm:"not null" json:"updated_at"`
	DeletedAt     gorm.DeletedAt `gorm:"index" json:"-"`

	// 关联
	ApiKeys []ApiKey `gorm:"foreignKey:UserID" json:"api_keys,omitempty"`
}

func (User) TableName() string {
	return "users"
}

// IsAdmin 检查是否管理员
func (u *User) IsAdmin() bool {
	return u.Role == "admin"
}

// IsActive 检查是否激活
func (u *User) IsActive() bool {
	return u.Status == "active"
}

// CanBindGroup 检查是否可以绑定指定分组
// 对于标准类型分组：
// - 如果 AllowedGroups 设置了值（非空数组），只能绑定列表中的分组
// - 如果 AllowedGroups 为 nil 或空数组，可以绑定所有非专属分组
func (u *User) CanBindGroup(groupID int64, isExclusive bool) bool {
	// 如果设置了 allowed_groups 且不为空，只能绑定指定的分组
	if len(u.AllowedGroups) > 0 {
		for _, id := range u.AllowedGroups {
			if id == groupID {
				return true
			}
		}
		return false
	}
	// 如果没有设置 allowed_groups 或为空数组，可以绑定所有非专属分组
	return !isExclusive
}

// SetPassword 设置密码（哈希存储）
func (u *User) SetPassword(password string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	u.PasswordHash = string(hash)
	return nil
}

// CheckPassword 验证密码
func (u *User) CheckPassword(password string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password))
	return err == nil
}
