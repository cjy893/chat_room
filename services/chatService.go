package services

import (
	"context"
	"encoding/json"
	"fmt"
	"socket/models"
	"socket/repositories"
	"strconv"
	"time"

	"github.com/google/uuid"
)

type chatServiceImpl struct {
	messageRepo repositories.MessageRepository
	userRepo    repositories.UserRepository
	roomRepo    repositories.RoomRepository
}

func NewChatService(
	messageRepo repositories.MessageRepository,
	userRepo repositories.UserRepository,
	roomRepo repositories.RoomRepository,
) ChatService {
	return &chatServiceImpl{
		messageRepo: messageRepo,
		userRepo:    userRepo,
		roomRepo:    roomRepo,
	}
}

// ==================== 消息管理 ====================

func (s *chatServiceImpl) SendMessage(ctx context.Context, msg *models.Message) error {
	// 验证发送者
	user, err := s.userRepo.GetByID(ctx, msg.SenderID)
	if err != nil || user == nil {
		return fmt.Errorf("发送者不存在")
	}

	// 验证房间
	room, err := s.roomRepo.GetByID(ctx, msg.RoomID)
	if err != nil || room == nil {
		return fmt.Errorf("聊天室不存在")
	}

	// 检查用户是否在房间中
	isMember := false
	for _, member := range room.Members {
		if member.UserID == msg.SenderID {
			isMember = true
			break
		}
	}

	if !isMember {
		return fmt.Errorf("用户不在该聊天室中")
	}

	// 设置发送者名称
	if msg.SenderName == "" {
		msg.SenderName = user.Username
	}

	// 保存消息
	return s.messageRepo.Create(ctx, msg)
}

func (s *chatServiceImpl) GetChatHistory(ctx context.Context, roomID, page, limit string) (*models.MessageResponse, error) {
	// 解析分页参数
	pageInt, err := strconv.Atoi(page)
	if err != nil || pageInt <= 0 {
		pageInt = 1
	}

	limitInt, err := strconv.Atoi(limit)
	if err != nil || limitInt <= 0 {
		limitInt = 50
	}

	// 构建查询
	query := &models.MessageQuery{
		RoomID:   roomID,
		Page:     pageInt,
		PageSize: limitInt,
		OrderBy:  "desc",
	}

	return s.messageRepo.GetMessages(ctx, query)
}

func (s *chatServiceImpl) GetPrivateChatHistory(ctx context.Context, userID, friendID, page, limit string) (*models.MessageResponse, error) {
	// 查找这两个用户之间的私有房间
	roomID, err := s.GetPrivateRoomBetweenUsers(ctx, userID, friendID)
	if err != nil {
		return nil, fmt.Errorf("查找私聊房间失败: %w", err)
	}

	if roomID == "" {
		// 如果没有私聊房间，返回空结果
		return &models.MessageResponse{
			Messages:    []models.Message{},
			Total:       0,
			CurrentPage: 1,
			TotalPages:  0,
			HasMore:     false,
		}, nil
	}

	// 获取房间消息记录
	return s.GetChatHistory(ctx, roomID, page, limit)
}

func (s *chatServiceImpl) GetUnreadMessages(ctx context.Context, userID, roomID string) ([]*models.Message, error) {
	return s.messageRepo.GetUnreadMessages(ctx, userID, roomID)
}

func (s *chatServiceImpl) SearchMessages(ctx context.Context, keyword, roomID, limit string) ([]*models.Message, error) {
	limitInt, err := strconv.Atoi(limit)
	if err != nil || limitInt <= 0 {
		limitInt = 50
	}

	return s.messageRepo.SearchMessages(ctx, keyword, roomID, limitInt)
}

func (s *chatServiceImpl) RecallMessage(ctx context.Context, messageID, userID, reason string) error {
	// 获取消息
	message, err := s.messageRepo.GetByID(ctx, messageID)
	if err != nil || message == nil {
		return fmt.Errorf("消息不存在")
	}

	// 检查权限（只有发送者或管理员可以撤回）
	if message.SenderID != userID {
		// 这里可以添加管理员检查逻辑
		return fmt.Errorf("无权撤回该消息")
	}

	// 检查撤回时间（例如：2分钟内可撤回）
	recallDeadline := message.CreatedAt.Add(2 * time.Minute)
	if time.Now().After(recallDeadline) {
		return fmt.Errorf("消息已超过可撤回时间")
	}

	// 创建撤回元数据
	metadata := &models.MessageMetadata{
		RecallBy:     userID,
		RecallAt:     time.Now().Format(time.RFC3339),
		RecallReason: reason,
	}

	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("创建撤回元数据失败: %w", err)
	}

	// 更新消息状态和内容
	updates := map[string]interface{}{
		"status":     models.MsgTypeRecall,
		"content":    "消息已撤回",
		"metadata":   metadataJSON,
		"updated_at": time.Now(),
	}

	return s.messageRepo.UpdateFields(ctx, messageID, updates)
}

func (s *chatServiceImpl) DeleteMessage(ctx context.Context, messageID, userID string) error {
	// 获取消息
	message, err := s.messageRepo.GetByID(ctx, messageID)
	if err != nil || message == nil {
		return fmt.Errorf("消息不存在")
	}

	// 检查权限（只有发送者或管理员可以删除）
	if message.SenderID != userID {
		// 这里可以添加管理员检查逻辑
		return fmt.Errorf("无权删除该消息")
	}

	return s.messageRepo.Delete(ctx, messageID)
}

// ==================== 已读回执 ====================

func (s *chatServiceImpl) MarkAsRead(ctx context.Context, userID, roomID string, messageIDs []string) error {
	return s.messageRepo.MarkMessagesAsRead(ctx, userID, roomID, messageIDs)
}

func (s *chatServiceImpl) GetReadReceipts(ctx context.Context, messageID string) ([]*models.ReadReceipt, error) {
	return s.messageRepo.GetReadReceipts(ctx, messageID)
}

func (s *chatServiceImpl) GetUnreadCount(ctx context.Context, userID, roomID string) (int64, error) {
	return s.messageRepo.GetUnreadCount(ctx, userID, roomID)
}

// ==================== 消息统计 ====================

func (s *chatServiceImpl) GetMessageStats(ctx context.Context, roomID string) (*models.MessageStats, error) {
	// 默认统计最近30天
	endTime := time.Now()
	startTime := endTime.AddDate(0, 0, -30)

	return s.messageRepo.GetMessageStats(ctx, roomID, startTime, endTime)
}

// ==================== 房间管理 ====================

func (s *chatServiceImpl) CreateRoom(ctx context.Context, room *models.ChatRoom) error {
	// 确保房间有ID
	if room.ID == "" {
		room.ID = uuid.New().String()
	}

	// 确保房间成员有ID
	for _, member := range room.Members {
		if member.ID == "" {
			member.ID = uuid.New().String()
		}
	}

	// 确保时间字段被设置
	now := time.Now()
	if room.CreatedAt.IsZero() {
		room.CreatedAt = now
	}
	room.UpdatedAt = now

	// 设置默认值
	if room.Type == "" {
		room.Type = "group"
	}

	if room.MaxMembers == 0 {
		room.MaxMembers = 200
	}

	return s.roomRepo.Create(ctx, room)
}

func (s *chatServiceImpl) CreatePrivateRoom(ctx context.Context, userID, friendID string) (string, error) {
	// 检查是否已经有私聊房间
	existingRoomID, err := s.GetPrivateRoomBetweenUsers(ctx, userID, friendID)
	if err != nil {
		return "", fmt.Errorf("检查现有私聊房间失败: %w", err)
	}

	if existingRoomID != "" {
		// 如果已有房间，直接返回房间ID
		return existingRoomID, nil
	}

	// 创建新的私聊房间
	room := &models.ChatRoom{
		ID:         uuid.New().String(),
		Name:       "Private chat",
		Type:       "private",
		IsPublic:   false,
		MaxMembers: 2,
		InviteCode: uuid.New().String(),
		CreatorID:  userID,
		Members: []*models.RoomMember{
			{
				ID:       uuid.New().String(),
				UserID:   userID,
				Role:     "member",
				JoinedAt: time.Now(),
			},
			{
				ID:       uuid.New().String(),
				UserID:   friendID,
				Role:     "member",
				JoinedAt: time.Now(),
			},
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := s.roomRepo.Create(ctx, room); err != nil {
		return "", fmt.Errorf("创建私聊房间失败: %w", err)
	}

	return room.ID, nil
}

func (s *chatServiceImpl) GetPrivateRoomBetweenUsers(ctx context.Context, userID, friendID string) (string, error) {
	// 查找两个用户共同参与的私有房间
	room, err := s.roomRepo.GetPrivateRoomBetweenUsers(ctx, userID, friendID)
	if err != nil {
		return "", fmt.Errorf("查询私聊房间失败: %w", err)
	}

	if room == nil {
		return "", nil // 没有找到私聊房间
	}

	return room.ID, nil
}

func (s *chatServiceImpl) JoinRoom(ctx context.Context, roomID, userID, role string) error {
	return s.roomRepo.AddMember(ctx, roomID, userID, role)
}

func (s *chatServiceImpl) LeaveRoom(ctx context.Context, roomID, userID string) error {
	return s.roomRepo.RemoveMember(ctx, roomID, userID)
}

func (s *chatServiceImpl) GetRoomMembers(ctx context.Context, roomID string) ([]*models.User, error) {
	// 获取房间成员ID
	room, err := s.roomRepo.GetByID(ctx, roomID)
	if err != nil || room == nil {
		return nil, fmt.Errorf("聊天室不存在")
	}

	// 获取成员详细信息
	memberIDs := make([]string, 0, len(room.Members))
	for _, member := range room.Members {
		memberIDs = append(memberIDs, member.UserID)
	}

	return s.userRepo.GetByIDs(ctx, memberIDs)
}

func (s *chatServiceImpl) GetUserRooms(ctx context.Context, userID string) ([]*models.ChatRoom, error) {
	return s.roomRepo.GetByUserID(ctx, userID)
}

func (s *chatServiceImpl) DeleteRoom(ctx context.Context, roomID, userID string) error {
	// 检查权限（只有创建者可以删除）
	room, err := s.roomRepo.GetByID(ctx, roomID)
	if err != nil || room == nil {
		return fmt.Errorf("聊天室不存在")
	}

	if room.CreatorID != userID {
		return fmt.Errorf("无权删除该聊天室")
	}

	return s.roomRepo.Delete(ctx, roomID)
}
