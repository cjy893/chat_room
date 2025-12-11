package repositories

import (
	"context"
	"fmt"
	"socket/models"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type MessageRepository interface {
	// 消息基础操作
	Create(ctx context.Context, message *models.Message) error
	GetByID(ctx context.Context, id string) (*models.Message, error)
	GetByIDs(ctx context.Context, ids []string) ([]*models.Message, error)
	Update(ctx context.Context, message *models.Message) error
	UpdateFields(ctx context.Context, id string, updates map[string]interface{}) error
	Delete(ctx context.Context, id string) error
	DeleteByRoomID(ctx context.Context, roomID string) error

	// 消息查询
	GetMessages(ctx context.Context, query *models.MessageQuery) (*models.MessageResponse, error)
	GetLatestMessages(ctx context.Context, roomID string, limit int) ([]*models.Message, error)
	GetUnreadMessages(ctx context.Context, userID, roomID string) ([]*models.Message, error)
	SearchMessages(ctx context.Context, keyword string, roomID string, limit int) ([]*models.Message, error)
	GetPrivateMessages(ctx context.Context, userID, friendID string, limit int) ([]*models.Message, error)

	// 消息统计
	GetMessageCount(ctx context.Context, roomID string) (int64, error)
	GetMessageCountByUser(ctx context.Context, userID string) (int64, error)
	GetMessageStats(ctx context.Context, roomID string, startTime, endTime time.Time) (*models.MessageStats, error)

	// 已读回执
	CreateReadReceipt(ctx context.Context, receipt *models.ReadReceipt) error
	GetReadReceipts(ctx context.Context, messageID string) ([]*models.ReadReceipt, error)
	GetUnreadCount(ctx context.Context, userID, roomID string) (int64, error)
	MarkMessagesAsRead(ctx context.Context, userID, roomID string, messageIDs []string) error
}

type messageRepository struct {
	db *gorm.DB
}

func NewMessageRepository(db *gorm.DB) MessageRepository {
	return &messageRepository{db: db}
}

// ==================== 消息基础操作 ====================

func (r *messageRepository) Create(ctx context.Context, message *models.Message) error {
	// 生成ID
	if message.ID == "" {
		message.ID = uuid.New().String()
	}

	// 设置时间
	now := time.Now()
	if message.CreatedAt.IsZero() {
		message.CreatedAt = now
	}
	message.UpdatedAt = now

	// 设置默认状态
	if message.Status == "" {
		message.Status = models.MsgStatusSent
	}

	return r.db.WithContext(ctx).Create(message).Error
}

func (r *messageRepository) GetByID(ctx context.Context, id string) (*models.Message, error) {
	var message models.Message
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&message).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("查询消息失败: %w", err)
	}
	return &message, nil
}

func (r *messageRepository) GetByIDs(ctx context.Context, ids []string) ([]*models.Message, error) {
	var messages []*models.Message
	err := r.db.WithContext(ctx).Where("id IN ?", ids).Order("created_at DESC").Find(&messages).Error
	if err != nil {
		return nil, fmt.Errorf("批量查询消息失败: %w", err)
	}
	return messages, nil
}

func (r *messageRepository) Update(ctx context.Context, message *models.Message) error {
	message.UpdatedAt = time.Now()
	return r.db.WithContext(ctx).Save(message).Error
}

func (r *messageRepository) UpdateFields(ctx context.Context, id string, updates map[string]interface{}) error {
	updates["updated_at"] = time.Now()
	return r.db.WithContext(ctx).Model(&models.Message{}).Where("id = ?", id).Updates(updates).Error
}

func (r *messageRepository) Delete(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Delete(&models.Message{}, "id = ?", id).Error
}

func (r *messageRepository) DeleteByRoomID(ctx context.Context, roomID string) error {
	return r.db.WithContext(ctx).Delete(&models.Message{}, "room_id = ?", roomID).Error
}

// ==================== 消息查询 ====================

func (r *messageRepository) GetMessages(ctx context.Context, query *models.MessageQuery) (*models.MessageResponse, error) {
	// 构建查询
	db := r.db.WithContext(ctx).Model(&models.Message{})

	// 添加条件
	if query.RoomID != "" {
		db = db.Where("room_id = ?", query.RoomID)
	}

	if query.SenderID != "" {
		db = db.Where("sender_id = ?", query.SenderID)
	}

	if !query.StartTime.IsZero() {
		db = db.Where("created_at >= ?", query.StartTime)
	}

	if !query.EndTime.IsZero() {
		db = db.Where("created_at <= ?", query.EndTime)
	}

	if query.Keyword != "" {
		db = db.Where("content LIKE ?", "%"+query.Keyword+"%")
	}

	if len(query.MessageTypes) > 0 {
		db = db.Where("message_type IN ?", query.MessageTypes)
	}

	// 排除已撤回的消息（如果需要）
	db = db.Where("status != ? OR status IS NULL", models.MsgStatusFailed)

	// 获取总数
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, fmt.Errorf("统计消息总数失败: %w", err)
	}

	// 计算分页
	if query.Page <= 0 {
		query.Page = 1
	}
	if query.PageSize <= 0 {
		query.PageSize = 50
	} else if query.PageSize > 200 {
		query.PageSize = 200 // 限制每页最大数量
	}

	offset := (query.Page - 1) * query.PageSize

	// 排序
	orderBy := "created_at DESC"
	if query.OrderBy == "asc" {
		orderBy = "created_at ASC"
	}

	// 获取分页数据
	var messages []*models.Message
	err := db.Order(orderBy).
		Offset(offset).
		Limit(query.PageSize).
		Find(&messages).Error

	if err != nil {
		return nil, fmt.Errorf("查询消息失败: %w", err)
	}

	// 计算总页数
	totalPages := int(total) / query.PageSize
	if int(total)%query.PageSize > 0 {
		totalPages++
	}

	// 转换类型
	resultMessages := make([]models.Message, len(messages))
	for i, msg := range messages {
		resultMessages[i] = *msg
	}

	return &models.MessageResponse{
		Messages:    resultMessages,
		Total:       total,
		CurrentPage: query.Page,
		TotalPages:  totalPages,
		HasMore:     query.Page < totalPages,
	}, nil
}

func (r *messageRepository) GetLatestMessages(ctx context.Context, roomID string, limit int) ([]*models.Message, error) {
	if limit <= 0 {
		limit = 50
	}

	var messages []*models.Message
	err := r.db.WithContext(ctx).
		Where("room_id = ?", roomID).
		Where("status != ?", models.MsgStatusFailed).
		Order("created_at DESC").
		Limit(limit).
		Find(&messages).Error

	if err != nil {
		return nil, fmt.Errorf("查询最新消息失败: %w", err)
	}

	return messages, nil
}

func (r *messageRepository) GetUnreadMessages(ctx context.Context, userID, roomID string) ([]*models.Message, error) {
	var messages []*models.Message
	err := r.db.WithContext(ctx).
		Where("room_id = ?", roomID).
		Where("sender_id != ?", userID).
		Where("status != ?", models.MsgStatusFailed).
		Order("created_at ASC").
		Find(&messages).Error

	if err != nil {
		return nil, fmt.Errorf("查询未读消息失败: %w", err)
	}

	return messages, nil
}

func (r *messageRepository) SearchMessages(ctx context.Context, keyword string, roomID string, limit int) ([]*models.Message, error) {
	if limit <= 0 {
		limit = 50
	}

	var messages []*models.Message
	db := r.db.WithContext(ctx)

	if roomID != "" {
		db = db.Where("room_id = ?", roomID)
	}

	err := db.Where("content LIKE ?", "%"+keyword+"%").
		Where("status != ?", models.MsgStatusFailed).
		Order("created_at DESC").
		Limit(limit).
		Find(&messages).Error

	if err != nil {
		return nil, fmt.Errorf("搜索消息失败: %w", err)
	}

	return messages, nil
}

func (r *messageRepository) GetPrivateMessages(ctx context.Context, userID, friendID string, limit int) ([]*models.Message, error) {
	if limit <= 0 {
		limit = 50
	}

	var messages []*models.Message
	err := r.db.WithContext(ctx).
		Where("(sender_id = ? AND recipient_type = ? AND to LIKE ?) OR (sender_id = ? AND recipient_type = ? AND to LIKE ?)", 
			userID, models.RecipientUser, "%"+friendID+"%", 
			friendID, models.RecipientUser, "%"+userID+"%").
		Where("status != ?", models.MsgStatusFailed).
		Order("created_at DESC").
		Limit(limit).
		Find(&messages).Error

	if err != nil {
		return nil, fmt.Errorf("查询私聊消息失败: %w", err)
	}

	return messages, nil
}

// ==================== 消息统计 ====================

func (r *messageRepository) GetMessageCount(ctx context.Context, roomID string) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&models.Message{}).
		Where("room_id = ?", roomID).
		Where("status != ?", models.MsgStatusFailed).
		Count(&count).Error

	if err != nil {
		return 0, fmt.Errorf("统计消息数量失败: %w", err)
	}

	return count, nil
}

func (r *messageRepository) GetMessageCountByUser(ctx context.Context, userID string) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&models.Message{}).
		Where("sender_id = ?", userID).
		Where("status != ?", models.MsgStatusFailed).
		Count(&count).Error

	if err != nil {
		return 0, fmt.Errorf("统计用户消息数量失败: %w", err)
	}

	return count, nil
}

func (r *messageRepository) GetMessageStats(ctx context.Context, roomID string, startTime, endTime time.Time) (*models.MessageStats, error) {
	var stats models.MessageStats

	// 基础统计
	baseQuery := r.db.WithContext(ctx).Model(&models.Message{}).Where("room_id = ?", roomID)

	// 总消息数
	if err := baseQuery.Where("status != ?", models.MsgStatusFailed).
		Count(&stats.TotalMessages).Error; err != nil {
		return nil, fmt.Errorf("统计总消息数失败: %w", err)
	}

	// 按类型统计
	if err := baseQuery.Where("message_type = ?", models.MsgTypeText).
		Count(&stats.TextMessages).Error; err != nil {
		return nil, fmt.Errorf("统计文本消息失败: %w", err)
	}

	if err := baseQuery.Where("message_type = ?", models.MsgTypeImage).
		Count(&stats.ImageMessages).Error; err != nil {
		return nil, fmt.Errorf("统计图片消息失败: %w", err)
	}

	if err := baseQuery.Where("message_type = ?", models.MsgTypeFile).
		Count(&stats.FileMessages).Error; err != nil {
		return nil, fmt.Errorf("统计文件消息失败: %w", err)
	}

	// 统计活跃用户数（发送过消息的用户）
	var activeUsers int64
	if err := baseQuery.Distinct("sender_id").Count(&activeUsers).Error; err != nil {
		return nil, fmt.Errorf("统计活跃用户失败: %w", err)
	}
	stats.ActiveUsers = activeUsers

	// 按时间段统计
	if !startTime.IsZero() && !endTime.IsZero() {
		days := endTime.Sub(startTime).Hours() / 24
		if days > 0 {
			stats.AveragePerDay = int64(float64(stats.TotalMessages) / days)
		}

		// 统计最繁忙的小时（需要更复杂的查询）
		// 这里简化实现
	}

	return &stats, nil
}

// ==================== 已读回执 ====================

func (r *messageRepository) CreateReadReceipt(ctx context.Context, receipt *models.ReadReceipt) error {
	if receipt.ID == "" {
		receipt.ID = uuid.New().String()
	}

	if receipt.ReadAt.IsZero() {
		receipt.ReadAt = time.Now()
	}

	return r.db.WithContext(ctx).Create(receipt).Error
}

func (r *messageRepository) GetReadReceipts(ctx context.Context, messageID string) ([]*models.ReadReceipt, error) {
	var receipts []*models.ReadReceipt
	err := r.db.WithContext(ctx).Where("message_id = ?", messageID).Order("read_at ASC").Find(&receipts).Error
	if err != nil {
		return nil, fmt.Errorf("查询已读回执失败: %w", err)
	}
	return receipts, nil
}

func (r *messageRepository) GetUnreadCount(ctx context.Context, userID, roomID string) (int64, error) {
	// 获取用户最后读取的时间
	var lastReadAt time.Time
	err := r.db.WithContext(ctx).
		Model(&models.ReadReceipt{}).
		Select("MAX(read_at)").
		Where("user_id = ? AND room_id = ?", userID, roomID).
		Scan(&lastReadAt).Error

	if err != nil {
		// 如果没有已读记录，返回所有消息数量
		var count int64
		err = r.db.WithContext(ctx).
			Model(&models.Message{}).
			Where("room_id = ?", roomID).
			Where("sender_id != ?", userID).
			Where("status != ?", models.MsgStatusFailed).
			Count(&count).Error
		return count, err
	}

	// 统计最后读取时间之后的消息数量
	var count int64
	err = r.db.WithContext(ctx).
		Model(&models.Message{}).
		Where("room_id = ?", roomID).
		Where("created_at > ?", lastReadAt).
		Where("sender_id != ?", userID).
		Where("status != ?", models.MsgStatusFailed).
		Count(&count).Error

	return count, err
}

func (r *messageRepository) MarkMessagesAsRead(ctx context.Context, userID, roomID string, messageIDs []string) error {
	if len(messageIDs) == 0 {
		return nil
	}

	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 为每条消息创建已读回执
		for _, msgID := range messageIDs {
			receipt := &models.ReadReceipt{
				ID:        uuid.New().String(),
				MessageID: msgID,
				UserID:    userID,
				RoomID:    roomID,
				ReadAt:    time.Now(),
			}

			if err := tx.Create(receipt).Error; err != nil {
				return err
			}
		}

		return nil
	})
}
