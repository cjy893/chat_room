package services

import (
	"context"
	"socket/models"
)

type UserService interface {
	Register(ctx context.Context, username, password, email string) (*models.User, string, error)
	Login(ctx context.Context, username, password string, ip, userAgent string) (*models.User, string, error)
	Logout(ctx context.Context, token string) error
	ValidateToken(ctx context.Context, token string) (*models.User, error)
	RefreshToken(ctx context.Context, refreshToken string) (string, error)

	// 用户管理
	GetUserByID(ctx context.Context, userID string) (*models.User, error)
	GetUserByUsername(ctx context.Context, username string) (*models.User, error)
	UpdateUser(ctx context.Context, userID string, updates map[string]interface{}) error
	ChangePassword(ctx context.Context, userID, oldPassword, newPassword string) error
	DeleteUser(ctx context.Context, userID string) error

	// 在线状态管理
	SetOnline(ctx context.Context, userID string) error
	SetOffline(ctx context.Context, userID string) error
	SetStatus(ctx context.Context, userID, status string) error
	GetOnlineUsers(ctx context.Context) ([]*models.User, error)
	GetUserStatus(ctx context.Context, userID string) (string, error)

	// 好友管理
	AddFriend(ctx context.Context, userID, friendID string) error
	RemoveFriend(ctx context.Context, userID, friendID string) error
	GetFriends(ctx context.Context, userID string) ([]*models.User, error)
	GetFriendRequests(ctx context.Context, userID string) ([]*models.Friend, error)
	AcceptFriendRequest(ctx context.Context, userID, requestID string) error
	BlockUser(ctx context.Context, userID, blockUserID string) error

	// 统计信息
	GetUserStats(ctx context.Context, userID string) (*models.UserStats, error)
	RecordUserActivity(ctx context.Context, userID string) error
}

type ChatService interface {
	// 消息管理
	SendMessage(ctx context.Context, msg *models.Message) error
	GetChatHistory(ctx context.Context, roomID, page, limit string) (*models.MessageResponse, error)
	GetUnreadMessages(ctx context.Context, userID, roomID string) ([]*models.Message, error)
	SearchMessages(ctx context.Context, keyword, roomID, limit string) ([]*models.Message, error)
	RecallMessage(ctx context.Context, messageID, userID, reason string) error
	DeleteMessage(ctx context.Context, messageID, userID string) error

	// 已读回执
	MarkAsRead(ctx context.Context, userID, roomID string, messageIDs []string) error
	GetReadReceipts(ctx context.Context, messageID string) ([]*models.ReadReceipt, error)
	GetUnreadCount(ctx context.Context, userID, roomID string) (int64, error)

	// 消息统计
	GetMessageStats(ctx context.Context, roomID string) (*models.MessageStats, error)

	// 房间管理
	CreateRoom(ctx context.Context, room *models.ChatRoom) error
	JoinRoom(ctx context.Context, roomID, userID, role string) error
	LeaveRoom(ctx context.Context, roomID, userID string) error
	GetRoomMembers(ctx context.Context, roomID string) ([]*models.User, error)
	GetUserRooms(ctx context.Context, userID string) ([]*models.ChatRoom, error)
	DeleteRoom(ctx context.Context, roomID, userID string) error
}
