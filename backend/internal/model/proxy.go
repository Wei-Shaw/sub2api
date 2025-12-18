package model

import (
	"fmt"
	"time"

	"gorm.io/gorm"
)

type Proxy struct {
	ID        int64          `gorm:"primaryKey" json:"id"`
	Name      string         `gorm:"size:100;not null" json:"name"`
	Protocol  string         `gorm:"size:20;not null" json:"protocol"` // http/https/socks5
	Host      string         `gorm:"size:255;not null" json:"host"`
	Port      int            `gorm:"not null" json:"port"`
	Username  string         `gorm:"size:100" json:"username"`
	Password  string         `gorm:"size:100" json:"-"`
	Status    string         `gorm:"size:20;default:active;not null" json:"status"` // active/disabled
	CreatedAt time.Time      `gorm:"not null" json:"created_at"`
	UpdatedAt time.Time      `gorm:"not null" json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
REDACTED

func (Proxy) TableName() string {
	return "proxies"
REDACTED

// IsActive 检查是否激活
func (p *Proxy) IsActive() bool {
	return p.Status == "active"
REDACTED

// URL 返回代理URL
func (p *Proxy) URL() string {
	if p.Username != "" && p.Password != "" {
		return fmt.Sprintf("%s://%s:%s@%s:%d", p.Protocol, p.Username, p.Password, p.Host, p.Port)
REDACTED
	return fmt.Sprintf("%s://%s:%d", p.Protocol, p.Host, p.Port)
REDACTED

// ProxyWithAccountCount extends Proxy with account count information
type ProxyWithAccountCount struct {
	Proxy
	AccountCount int64 `json:"account_count"`
REDACTED
