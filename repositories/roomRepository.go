package repositories

import (
	"context"
	"fmt"
	"socket/models"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type RoomRepository interface {
	Create(ctx context.Context, room *models.ChatRoom) error
	GetByID(ctx context.Context, id string) (*models.ChatRoom, error)
	GetByUserID(ctx context.Context, userID string) ([]*models.ChatRoom, error)
	Update(ctx context.Context, room *models.ChatRoom) error
	Delete(ctx context.Context, id string) error

	// 成员管理
	AddMember(ctx context.Context, roomID, userID, role string) error
	RemoveMember(ctx context.Context, roomID, userID string) error
	GetMembers(ctx context.Context, roomID string) ([]*models.User, error)
	IsMember(ctx context.Context, roomID, userID string) (bool, error)
	UpdateMemberRole(ctx context.Context, roomID, userID, role string) error
	GetMemberRole(ctx context.Context, roomID, userID string) (string, error)

	// 房间查询
	SearchRooms(ctx context.Context, keyword string, roomType string, isPublic bool, page, limit int) ([]*models.ChatRoom, int64, error)
	GetPublicRooms(ctx context.Context, page, limit int) ([]*models.ChatRoom, int64, error)
	GetUserRoomsByType(ctx context.Context, userID, roomType string) ([]*models.ChatRoom, error)

	// 房间统计
	GetRoomStats(ctx context.Context, roomID string) (*models.RoomStats, error)
	GetActiveRooms(ctx context.Context, hours int, limit int) ([]*models.ActiveRoom, error)

	// 房间设置
	UpdateRoomSettings(ctx context.Context, roomID string, settings map[string]interface{}) error
	UpdateRoomAvatar(ctx context.Context, roomID, avatarURL string) error
	UpdateRoomName(ctx context.Context, roomID, name string) error
	UpdateRoomDescription(ctx context.Context, roomID, description string) error
	SetRoomPublic(ctx context.Context, roomID string, isPublic bool) error
	SetRoomMaxMembers(ctx context.Context, roomID string, maxMembers int) error

	// 管理功能
	TransferOwnership(ctx context.Context, roomID, fromUserID, toUserID string) error
	GetRoomInviteCode(ctx context.Context, roomID string) (string, error)
	RegenerateInviteCode(ctx context.Context, roomID string) (string, error)
	ValidateInviteCode(ctx context.Context, inviteCode string) (*models.ChatRoom, error)
	CleanupInactiveRooms(ctx context.Context, days int) (int64, error)

	// GetPrivateRoomBetweenUsers finds the private room between two users
	GetPrivateRoomBetweenUsers(ctx context.Context, userID, friendID string) (*models.ChatRoom, error)
}

type roomRepository struct {
	db *gorm.DB
}

func NewRoomRepository(db *gorm.DB) RoomRepository {
	return &roomRepository{db: db}
}

func (r *roomRepository) Create(ctx context.Context, room *models.ChatRoom) error {

	// 使用事务确保数据一致性
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 创建房间
		if err := tx.Create(room).Error; err != nil {
			return fmt.Errorf("创建房间失败: %w", err)
		}

		return nil
	})
}

func (r *roomRepository) GetByID(ctx context.Context, id string) (*models.ChatRoom, error) {
	var room models.ChatRoom

	err := r.db.WithContext(ctx).
		Preload("Members").
		Preload("Members.User", func(db *gorm.DB) *gorm.DB {
			return db.Select("id, username, avatar, status")
		}).
		Where("id = ?", id).
		First(&room).Error

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("查询房间失败: %w", err)
	}

	return &room, nil
}

func (r *roomRepository) GetByUserID(ctx context.Context, userID string) ([]*models.ChatRoom, error) {
	var rooms []*models.ChatRoom

	// 通过成员表关联查询用户所在的房间
	err := r.db.WithContext(ctx).
		Model(&models.ChatRoom{}).
		Select("chat_rooms.*").
		Joins("INNER JOIN room_members ON room_members.room_id = chat_rooms.id").
		Where("room_members.user_id = ?", userID).
		Preload("Members", func(db *gorm.DB) *gorm.DB {
			return db.Where("user_id = ?", userID).Limit(1)
		}).
		Order("chat_rooms.updated_at DESC").
		Find(&rooms).Error

	if err != nil {
		return nil, fmt.Errorf("查询用户房间失败: %w", err)
	}

	return rooms, nil
}

func (r *roomRepository) Update(ctx context.Context, room *models.ChatRoom) error {
	room.UpdatedAt = time.Now()

	if err := r.db.WithContext(ctx).Save(room).Error; err != nil {
		return fmt.Errorf("更新房间失败: %w", err)
	}

	return nil
}

func (r *roomRepository) Delete(ctx context.Context, id string) error {
	// 使用事务删除房间及相关数据
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 删除房间成员
		if err := tx.Where("room_id = ?", id).Delete(&models.RoomMember{}).Error; err != nil {
			return fmt.Errorf("删除房间成员失败: %w", err)
		}

		// 删除房间消息（可选，根据需求决定是否保留历史消息）
		// if err := tx.Where("room_id = ?", id).Delete(&models.Message{}).Error; err != nil {
		//     return fmt.Errorf("删除房间消息失败: %w", err)
		// }

		// 删除房间
		if err := tx.Delete(&models.ChatRoom{}, "id = ?", id).Error; err != nil {
			return fmt.Errorf("删除房间失败: %w", err)
		}

		return nil
	})
}

// ==================== 成员管理 ====================

func (r *roomRepository) AddMember(ctx context.Context, roomID, userID, role string) error {
	// 检查房间是否存在
	var roomCount int64
	if err := r.db.WithContext(ctx).
		Model(&models.ChatRoom{}).
		Where("id = ?", roomID).
		Count(&roomCount).Error; err != nil {
		return fmt.Errorf("检查房间失败: %w", err)
	}

	if roomCount == 0 {
		return fmt.Errorf("房间不存在")
	}

	// 检查用户是否已是成员
	var memberCount int64
	if err := r.db.WithContext(ctx).
		Model(&models.RoomMember{}).
		Where("room_id = ? AND user_id = ?", roomID, userID).
		Count(&memberCount).Error; err != nil {
		return fmt.Errorf("检查成员失败: %w", err)
	}

	if memberCount > 0 {
		return fmt.Errorf("用户已是房间成员")
	}

	// 检查房间成员数量限制
	var currentMembers int64
	if err := r.db.WithContext(ctx).
		Model(&models.RoomMember{}).
		Where("room_id = ?", roomID).
		Count(&currentMembers).Error; err != nil {
		return fmt.Errorf("统计成员失败: %w", err)
	}

	// 获取房间的最大成员数
	var room models.ChatRoom
	if err := r.db.WithContext(ctx).
		Select("max_members").
		Where("id = ?", roomID).
		First(&room).Error; err != nil {
		return fmt.Errorf("获取房间信息失败: %w", err)
	}

	if room.MaxMembers > 0 && currentMembers >= int64(room.MaxMembers) {
		return fmt.Errorf("房间成员数已达上限")
	}

	// 添加成员
	member := &models.RoomMember{
		ID:       uuid.New().String(),
		RoomID:   roomID,
		UserID:   userID,
		Role:     role,
		JoinedAt: time.Now(),
	}

	if err := r.db.WithContext(ctx).Create(member).Error; err != nil {
		return fmt.Errorf("添加成员失败: %w", err)
	}

	// 更新房间的更新时间
	if err := r.db.WithContext(ctx).
		Model(&models.ChatRoom{}).
		Where("id = ?", roomID).
		Update("updated_at", time.Now()).Error; err != nil {
		// 日志记录但不影响主流程
		fmt.Printf("更新房间时间失败: %v\n", err)
	}

	return nil
}

func (r *roomRepository) RemoveMember(ctx context.Context, roomID, userID string) error {
	// 检查用户是否是房间的owner
	var member models.RoomMember
	if err := r.db.WithContext(ctx).
		Where("room_id = ? AND user_id = ?", roomID, userID).
		First(&member).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return fmt.Errorf("用户不是房间成员")
		}
		return fmt.Errorf("查询成员失败: %w", err)
	}

	// 如果是owner，需要确保房间还有至少一个owner
	if member.Role == "owner" {
		var ownerCount int64
		if err := r.db.WithContext(ctx).
			Model(&models.RoomMember{}).
			Where("room_id = ? AND role = ? AND user_id != ?", roomID, "owner", userID).
			Count(&ownerCount).Error; err != nil {
			return fmt.Errorf("检查owner数量失败: %w", err)
		}

		if ownerCount == 0 {
			return fmt.Errorf("房间必须至少有一个owner")
		}
	}

	// 删除成员
	if err := r.db.WithContext(ctx).
		Delete(&models.RoomMember{}, "room_id = ? AND user_id = ?", roomID, userID).Error; err != nil {
		return fmt.Errorf("移除成员失败: %w", err)
	}

	// 更新房间的更新时间
	if err := r.db.WithContext(ctx).
		Model(&models.ChatRoom{}).
		Where("id = ?", roomID).
		Update("updated_at", time.Now()).Error; err != nil {
		fmt.Printf("更新房间时间失败: %v\n", err)
	}

	return nil
}

func (r *roomRepository) GetMembers(ctx context.Context, roomID string) ([]*models.User, error) {
	var users []*models.User

	// 通过成员表关联查询房间成员
	err := r.db.WithContext(ctx).
		Model(&models.User{}).
		Select("users.*, room_members.role, room_members.joined_at, room_members.nickname").
		Joins("INNER JOIN room_members ON room_members.user_id = users.id").
		Where("room_members.room_id = ?", roomID).
		Order("CASE room_members.role WHEN 'owner' THEN 1 WHEN 'admin' THEN 2 ELSE 3 END, room_members.joined_at").
		Find(&users).Error

	if err != nil {
		return nil, fmt.Errorf("查询房间成员失败: %w", err)
	}

	return users, nil
}

func (r *roomRepository) IsMember(ctx context.Context, roomID, userID string) (bool, error) {
	var count int64

	err := r.db.WithContext(ctx).
		Model(&models.RoomMember{}).
		Where("room_id = ? AND user_id = ?", roomID, userID).
		Count(&count).Error

	if err != nil {
		return false, fmt.Errorf("检查成员资格失败: %w", err)
	}

	return count > 0, nil
}

func (r *roomRepository) UpdateMemberRole(ctx context.Context, roomID, userID, role string) error {
	// 验证角色
	validRoles := []string{"owner", "admin", "member"}
	valid := false
	for _, r := range validRoles {
		if r == role {
			valid = true
			break
		}
	}
	if !valid {
		return fmt.Errorf("无效的角色: %s", role)
	}

	// 更新角色
	if err := r.db.WithContext(ctx).
		Model(&models.RoomMember{}).
		Where("room_id = ? AND user_id = ?", roomID, userID).
		Update("role", role).Error; err != nil {
		return fmt.Errorf("更新成员角色失败: %w", err)
	}

	return nil
}

func (r *roomRepository) GetMemberRole(ctx context.Context, roomID, userID string) (string, error) {
	var member models.RoomMember

	err := r.db.WithContext(ctx).
		Where("room_id = ? AND user_id = ?", roomID, userID).
		Select("role").
		First(&member).Error

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return "", fmt.Errorf("用户不是房间成员")
		}
		return "", fmt.Errorf("查询成员角色失败: %w", err)
	}

	return member.Role, nil
}

// ==================== 房间查询 ====================

func (r *roomRepository) SearchRooms(ctx context.Context, keyword string, roomType string, isPublic bool, page, limit int) ([]*models.ChatRoom, int64, error) {
	if page <= 0 {
		page = 1
	}
	if limit <= 0 {
		limit = 20
	} else if limit > 100 {
		limit = 100
	}

	offset := (page - 1) * limit

	db := r.db.WithContext(ctx).Model(&models.ChatRoom{})

	// 添加搜索条件
	if keyword != "" {
		db = db.Where("name LIKE ? OR description LIKE ?",
			"%"+keyword+"%", "%"+keyword+"%")
	}

	if roomType != "" {
		db = db.Where("type = ?", roomType)
	}

	// 如果需要只搜索公开房间
	// db = db.Where("is_public = ?", isPublic)

	// 获取总数
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("统计房间数量失败: %w", err)
	}

	// 获取分页数据
	var rooms []*models.ChatRoom
	err := db.Offset(offset).
		Limit(limit).
		Order("updated_at DESC").
		Find(&rooms).Error

	if err != nil {
		return nil, 0, fmt.Errorf("搜索房间失败: %w", err)
	}

	return rooms, total, nil
}

func (r *roomRepository) GetPublicRooms(ctx context.Context, page, limit int) ([]*models.ChatRoom, int64, error) {
	if page <= 0 {
		page = 1
	}
	if limit <= 0 {
		limit = 20
	}

	offset := (page - 1) * limit

	var total int64
	if err := r.db.WithContext(ctx).
		Model(&models.ChatRoom{}).
		Where("is_public = ?", true).
		Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("统计公开房间失败: %w", err)
	}

	var rooms []*models.ChatRoom
	err := r.db.WithContext(ctx).
		Where("is_public = ?", true).
		Offset(offset).
		Limit(limit).
		Order("updated_at DESC").
		Find(&rooms).Error

	if err != nil {
		return nil, 0, fmt.Errorf("查询公开房间失败: %w", err)
	}

	return rooms, total, nil
}

func (r *roomRepository) GetUserRoomsByType(ctx context.Context, userID, roomType string) ([]*models.ChatRoom, error) {
	var rooms []*models.ChatRoom

	query := r.db.WithContext(ctx).
		Model(&models.ChatRoom{}).
		Select("chat_rooms.*").
		Joins("INNER JOIN room_members ON room_members.room_id = chat_rooms.id").
		Where("room_members.user_id = ?", userID)

	if roomType != "" {
		query = query.Where("chat_rooms.type = ?", roomType)
	}

	err := query.Order("chat_rooms.updated_at DESC").Find(&rooms).Error

	if err != nil {
		return nil, fmt.Errorf("查询用户房间失败: %w", err)
	}

	return rooms, nil
}

// ==================== 房间统计 ====================

func (r *roomRepository) GetRoomStats(ctx context.Context, roomID string) (*models.RoomStats, error) {
	var stats models.RoomStats

	// 统计成员数量
	if err := r.db.WithContext(ctx).
		Model(&models.RoomMember{}).
		Where("room_id = ?", roomID).
		Count(&stats.TotalMembers).Error; err != nil {
		return nil, fmt.Errorf("统计成员数量失败: %w", err)
	}

	// 统计消息数量
	if err := r.db.WithContext(ctx).
		Model(&models.Message{}).
		Where("room_id = ?", roomID).
		Count(&stats.TotalMessages).Error; err != nil {
		return nil, fmt.Errorf("统计消息数量失败: %w", err)
	}

	// 获取最后活动时间
	if err := r.db.WithContext(ctx).
		Model(&models.Message{}).
		Select("MAX(created_at)").
		Where("room_id = ?", roomID).
		Scan(&stats.LastActivity).Error; err != nil {
		// 如果没有消息，使用房间创建时间
		var room models.ChatRoom
		if err := r.db.WithContext(ctx).
			Select("created_at").
			Where("id = ?", roomID).
			First(&room).Error; err != nil {
			stats.LastActivity = time.Now()
		} else {
			stats.LastActivity = room.CreatedAt
		}
	}

	// 统计活跃成员（最近7天有活动的成员）
	weekAgo := time.Now().AddDate(0, 0, -7)
	if err := r.db.WithContext(ctx).
		Model(&models.RoomMember{}).
		Joins("INNER JOIN users ON users.id = room_members.user_id").
		Where("room_members.room_id = ? AND users.last_seen > ?", roomID, weekAgo).
		Count(&stats.ActiveMembers).Error; err != nil {
		stats.ActiveMembers = 0
	}

	// 今日新增成员
	today := time.Now().Truncate(24 * time.Hour)
	if err := r.db.WithContext(ctx).
		Model(&models.RoomMember{}).
		Where("room_id = ? AND joined_at >= ?", roomID, today).
		Count(&stats.NewMembersToday).Error; err != nil {
		stats.NewMembersToday = 0
	}

	// 今日消息数量
	if err := r.db.WithContext(ctx).
		Model(&models.Message{}).
		Where("room_id = ? AND created_at >= ?", roomID, today).
		Count(&stats.MessagesToday).Error; err != nil {
		stats.MessagesToday = 0
	}

	return &stats, nil
}

func (r *roomRepository) GetActiveRooms(ctx context.Context, hours int, limit int) ([]*models.ActiveRoom, error) {
	if hours <= 0 {
		hours = 24
	}
	if limit <= 0 {
		limit = 10
	}

	since := time.Now().Add(-time.Duration(hours) * time.Hour)

	var activeRooms []*models.ActiveRoom

	// 查询最近活跃的房间
	err := r.db.WithContext(ctx).
		Model(&models.Message{}).
		Select(`
            room_id,
            MAX(chat_rooms.name) as room_name,
            COUNT(*) as message_count,
            MAX(messages.created_at) as last_message_at,
            COUNT(DISTINCT messages.sender_id) as active_users
        `).
		Joins("INNER JOIN chat_rooms ON chat_rooms.id = messages.room_id").
		Where("messages.created_at >= ?", since).
		Group("room_id").
		Order("message_count DESC").
		Limit(limit).
		Scan(&activeRooms).Error

	if err != nil {
		return nil, fmt.Errorf("查询活跃房间失败: %w", err)
	}

	return activeRooms, nil
}

// ==================== 房间设置 ====================

func (r *roomRepository) UpdateRoomSettings(ctx context.Context, roomID string, settings map[string]interface{}) error {
	settings["updated_at"] = time.Now()

	if err := r.db.WithContext(ctx).
		Model(&models.ChatRoom{}).
		Where("id = ?", roomID).
		Updates(settings).Error; err != nil {
		return fmt.Errorf("更新房间设置失败: %w", err)
	}

	return nil
}

func (r *roomRepository) UpdateRoomAvatar(ctx context.Context, roomID, avatarURL string) error {
	return r.UpdateRoomSettings(ctx, roomID, map[string]interface{}{
		"avatar": avatarURL,
	})
}

func (r *roomRepository) UpdateRoomName(ctx context.Context, roomID, name string) error {
	return r.UpdateRoomSettings(ctx, roomID, map[string]interface{}{
		"name": name,
	})
}

func (r *roomRepository) UpdateRoomDescription(ctx context.Context, roomID, description string) error {
	return r.UpdateRoomSettings(ctx, roomID, map[string]interface{}{
		"description": description,
	})
}

func (r *roomRepository) SetRoomPublic(ctx context.Context, roomID string, isPublic bool) error {
	return r.UpdateRoomSettings(ctx, roomID, map[string]interface{}{
		"is_public": isPublic,
	})
}

func (r *roomRepository) SetRoomMaxMembers(ctx context.Context, roomID string, maxMembers int) error {
	if maxMembers < 1 {
		maxMembers = 1
	} else if maxMembers > 10000 {
		maxMembers = 10000
	}

	return r.UpdateRoomSettings(ctx, roomID, map[string]interface{}{
		"max_members": maxMembers,
	})
}

// ==================== 管理功能 ====================

func (r *roomRepository) TransferOwnership(ctx context.Context, roomID, fromUserID, toUserID string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 验证原所有者
		var fromMember models.RoomMember
		if err := tx.
			Where("room_id = ? AND user_id = ? AND role = ?", roomID, fromUserID, "owner").
			First(&fromMember).Error; err != nil {
			return fmt.Errorf("原用户不是房间所有者")
		}

		// 验证新所有者是房间成员
		var toMember models.RoomMember
		if err := tx.
			Where("room_id = ? AND user_id = ?", roomID, toUserID).
			First(&toMember).Error; err != nil {
			return fmt.Errorf("新用户不是房间成员")
		}

		// 更新原所有者为普通成员
		if err := tx.
			Model(&models.RoomMember{}).
			Where("room_id = ? AND user_id = ?", roomID, fromUserID).
			Update("role", "member").Error; err != nil {
			return fmt.Errorf("更新原所有者角色失败: %w", err)
		}

		// 更新新所有者为所有者
		if err := tx.
			Model(&models.RoomMember{}).
			Where("room_id = ? AND user_id = ?", roomID, toUserID).
			Update("role", "owner").Error; err != nil {
			return fmt.Errorf("更新新所有者角色失败: %w", err)
		}

		// 更新房间的创建者信息（如果需要）
		if err := tx.
			Model(&models.ChatRoom{}).
			Where("id = ?", roomID).
			Update("creator_id", toUserID).Error; err != nil {
			return fmt.Errorf("更新房间创建者失败: %w", err)
		}

		return nil
	})
}

func (r *roomRepository) GetRoomInviteCode(ctx context.Context, roomID string) (string, error) {
	// 这里简化实现，实际项目中可能需要更复杂的邀请码生成和存储逻辑
	var room models.ChatRoom
	if err := r.db.WithContext(ctx).
		Select("id, invite_code").
		Where("id = ?", roomID).
		First(&room).Error; err != nil {
		return "", fmt.Errorf("获取房间信息失败: %w", err)
	}

	// 如果房间没有邀请码，生成一个
	if room.InviteCode == "" {
		inviteCode := uuid.New().String()[:8] // 简化，取前8位
		if err := r.db.WithContext(ctx).
			Model(&models.ChatRoom{}).
			Where("id = ?", roomID).
			Update("invite_code", inviteCode).Error; err != nil {
			return "", fmt.Errorf("生成邀请码失败: %w", err)
		}
		return inviteCode, nil
	}

	return room.InviteCode, nil
}

func (r *roomRepository) RegenerateInviteCode(ctx context.Context, roomID string) (string, error) {
	inviteCode := uuid.New().String()[:8]

	if err := r.db.WithContext(ctx).
		Model(&models.ChatRoom{}).
		Where("id = ?", roomID).
		Update("invite_code", inviteCode).Error; err != nil {
		return "", fmt.Errorf("重新生成邀请码失败: %w", err)
	}

	return inviteCode, nil
}

func (r *roomRepository) ValidateInviteCode(ctx context.Context, inviteCode string) (*models.ChatRoom, error) {
	var room models.ChatRoom

	err := r.db.WithContext(ctx).
		Where("invite_code = ?", inviteCode).
		First(&room).Error

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("验证邀请码失败: %w", err)
	}

	return &room, nil
}

func (r *roomRepository) CleanupInactiveRooms(ctx context.Context, days int) (int64, error) {
	if days <= 0 {
		days = 30 // 默认清理30天未活跃的房间
	}

	threshold := time.Now().AddDate(0, 0, -days)

	// 这里只清理公共房间，私聊房间不应被清理
	result := r.db.WithContext(ctx).
		Where("type != ?", "private").
		Where("updated_at < ?", threshold).
		Delete(&models.ChatRoom{})

	if result.Error != nil {
		return 0, fmt.Errorf("清理不活跃房间失败: %w", result.Error)
	}

	return result.RowsAffected, nil
}

// GetPrivateRoomBetweenUsers finds the private room between two users
func (r *roomRepository) GetPrivateRoomBetweenUsers(ctx context.Context, userID, friendID string) (*models.ChatRoom, error) {
	var room models.ChatRoom

	// 查找类型为private且只有这两个用户的房间
	err := r.db.WithContext(ctx).
		Model(&models.ChatRoom{}).
		Joins("INNER JOIN room_members rm1 ON rm1.room_id = chat_rooms.id").
		Joins("INNER JOIN room_members rm2 ON rm2.room_id = chat_rooms.id").
		Where("chat_rooms.type = ?", "private").
		Where("rm1.user_id = ? AND rm2.user_id = ?", userID, friendID).
		Or("rm1.user_id = ? AND rm2.user_id = ?", friendID, userID).
		Group("chat_rooms.id").
		Having("COUNT(DISTINCT rm1.user_id) = 2").
		First(&room).Error

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("查询私聊房间失败: %w", err)
	}

	return &room, nil
}
