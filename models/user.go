package models

import (
	"time"
)

type User struct {
	ID        string    `json:"id" gorm:"primaryKey"`
	Username  string    `json:"username" gorm:"size:191;uniqueIndex;not null"`
	Password  string    `json:"-" gorm:"not null"`
	Email     string    `json:"email" gorm:"size:191;uniqueIndex"`
	Avatar    string    `json:"avatar"`                          // 头像URL
	Status    string    `json:"status" gorm:"default:'offline'"` // online, offline, busy, away
	LastSeen  time.Time `json:"last_seen"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type UserSession struct {
	ID           string    `json:"id" gorm:"primaryKey"`
	UserID       string    `json:"user_id" gorm:"index"`
	Token        string    `json:"token" gorm:"size:512;uniqueIndex"`
	RefreshToken string    `json:"refresh_token" gorm:"size:512;uniqueIndex"`
	ExpiresAt    time.Time `json:"expires_at"`
	IPAddress    string    `json:"ip_address"`
	UserAgent    string    `json:"user_agent"`
	CreatedAt    time.Time `json:"created_at"`
}

type Friend struct {
	ID         string     `json:"id" gorm:"primaryKey"`
	UserID     string     `json:"user_id" gorm:"index:idx_friend_pair,unique"`
	FriendID   string     `json:"friend_id" gorm:"index:idx_friend_pair,unique"`
	Status     string     `json:"status"` // pending, accepted, blocked
	CreatedAt  time.Time  `json:"created_at"`
	AcceptedAt *time.Time `json:"accepted_at"`
}

// 用户统计信息
type UserStats struct {
	UserID        string    `json:"user_id"`
	TotalMessages int64     `json:"total_messages"`
	OnlineHours   float64   `json:"online_hours"`
	LastActive    time.Time `json:"last_active"`
}

type SessionStats struct {
	TotalSessions      int64 `json:"total_sessions"`
	ActiveSessions     int64 `json:"active_sessions"`
	ExpiredSessions    int64 `json:"expired_sessions"`
	RevokedSessions    int64 `json:"revoked_sessions"`
	UniqueUsers        int64 `json:"unique_users"`
	AvgSessionsPerUser int64 `json:"avg_sessions_per_user"`
	MaxConcurrent      int64 `json:"max_concurrent"`
}
