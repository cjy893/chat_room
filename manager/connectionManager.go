package manager

import (
	"context"
	"fmt"
	"socket/models"
	"socket/repositories"
	"socket/services"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

type ConnectionManager struct {
	mu      sync.RWMutex
	clients map[string]*models.Client // userID -> Client (在线连接)

	// 依赖注入
	userService services.UserService
	chatService services.ChatService
	messageRepo repositories.MessageRepository
	roomRepo    repositories.RoomRepository

	// 管理通道
	registerChan   chan *models.Client
	unregisterChan chan *models.Client
	messageChan    chan models.Message
	closeChan      chan struct{}
}

func NewConnectionManager(
	userService services.UserService,
	chatService services.ChatService,
	messageRepo repositories.MessageRepository,
	roomRepo repositories.RoomRepository,
) *ConnectionManager {
	cm := &ConnectionManager{
		clients:        make(map[string]*models.Client),
		userService:    userService,
		chatService:    chatService,
		messageRepo:    messageRepo,
		roomRepo:       roomRepo,
		registerChan:   make(chan *models.Client, 100),
		unregisterChan: make(chan *models.Client, 100),
		messageChan:    make(chan models.Message, 1000),
		closeChan:      make(chan struct{}),
	}

	go cm.run()
	return cm
}

// ==================== 主要业务流程 ====================

func (cm *ConnectionManager) run() {
	for {
		select {
		case client := <-cm.registerChan:
			cm.handleRegister(client)

		case client := <-cm.unregisterChan:
			cm.handleUnregister(client)

		case msg := <-cm.messageChan:
			cm.handleMessage(msg)

		case <-cm.closeChan:
			return
		}
	}
}

// RegisterClient 注册客户端连接
func (cm *ConnectionManager) RegisterClient(client *models.Client) {
	cm.registerChan <- client
}

func (cm *ConnectionManager) handleRegister(client *models.Client) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	// 如果用户已存在在线连接，关闭旧连接
	if oldClient, exists := cm.clients[client.UserID]; exists {
		cm.forceDisconnect(oldClient, "新连接建立")
	}

	// 添加到在线列表
	cm.clients[client.UserID] = client

	// 更新用户状态为在线
	ctx := context.Background()
	cm.userService.SetOnline(ctx, client.UserID)

	// 广播用户上线通知
	cm.broadcastUserStatus(client.UserID, client.Username, true)

	// 发送欢迎消息
	cm.sendWelcomeMessage(client)

	// 启动读写协程
	go cm.readPump(client)
	go cm.writePump(client)

	fmt.Printf("用户 %s 已连接，当前在线用户数: %d\n",
		client.Username, len(cm.clients))
}

// UnregisterClient 注销客户端连接
func (cm *ConnectionManager) UnregisterClient(userID string) {
	cm.mu.RLock()
	client, exists := cm.clients[userID]
	cm.mu.RUnlock()

	if exists {
		cm.unregisterChan <- client
	}
}

func (cm *ConnectionManager) handleUnregister(client *models.Client) {
	cm.mu.Lock()

	// 检查是否是同一个连接（防止重复处理）
	if existingClient, exists := cm.clients[client.UserID]; exists && existingClient == client {
		delete(cm.clients, client.UserID)

		// 更新用户状态为离线
		ctx := context.Background()
		cm.userService.SetOffline(ctx, client.UserID)

		// 广播用户下线通知
		cm.broadcastUserStatus(client.UserID, client.Username, false)

		// 关闭连接
		client.IsOnline = false
		close(client.Send)
		client.Conn.Close()

		fmt.Printf("用户 %s 已断开连接，当前在线用户数: %d\n",
			client.Username, len(cm.clients))
	}

	cm.mu.Unlock()
}

// ==================== 消息处理 ====================

// SendMessage 发送消息
func (cm *ConnectionManager) SendMessage(msg models.Message) error {
	cm.messageChan <- msg
	return nil
}

func (cm *ConnectionManager) handleMessage(msg models.Message) {
	ctx := context.Background()

	// 保存消息到数据库
	if err := cm.messageRepo.Create(ctx, &msg); err != nil {
		fmt.Printf("保存消息失败: %v\n", err)
		return
	}

	// 根据消息类型分发
	switch msg.RecipientType {
	case models.RecipientUser:
		cm.sendToUser(msg)

	case models.RecipientRoom:
		cm.sendToRoom(msg)

	case models.RecipientBroadcast:
		cm.broadcastMessage(msg)

	default:
		fmt.Printf("未知的消息接收类型: %s\n", msg.RecipientType)
	}

	// 发送已读回执（如果是系统消息或不需要回执的消息则跳过）
	// 注意：这里假设 Message 结构体中有 NeedReadReceipt 字段，如果没有请删除此逻辑
	// if msg.NeedReadReceipt && msg.SenderID != "" {
	// 	cm.sendReadReceipt(msg)
	// }
}

func (cm *ConnectionManager) sendToUser(msg models.Message) {
	if len(msg.To) == 0 {
		return
	}

	cm.mu.RLock()
	defer cm.mu.RUnlock()

	targetUserID := msg.To[0]
	if client, exists := cm.clients[targetUserID]; exists && client.IsOnline {
		select {
		case client.Send <- msg:
			// 消息发送成功
		default:
			// 通道已满，记录日志
			fmt.Printf("用户 %s 的消息通道已满\n", targetUserID)
		}
	}
}

func (cm *ConnectionManager) sendToRoom(msg models.Message) {
	if msg.RoomID == "" {
		return
	}

	ctx := context.Background()

	// 获取房间成员
	members, err := cm.roomRepo.GetMembers(ctx, msg.RoomID)
	if err != nil {
		fmt.Printf("获取房间成员失败: %v\n", err)
		return
	}

	cm.mu.RLock()
	defer cm.mu.RUnlock()

	// 发送给房间内所有在线成员（除了发送者自己）
	for _, member := range members {
		// if member.ID == msg.SenderID {
		// 	continue // 不发送给自己
		// }

		if client, exists := cm.clients[member.ID]; exists && client.IsOnline {
			select {
			case client.Send <- msg:
				// 消息发送成功
			default:
				// 通道已满
				fmt.Printf("用户 %s 的消息通道已满\n", member.ID)
			}
		}
	}
}

func (cm *ConnectionManager) broadcastMessage(msg models.Message) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	// 广播给所有在线用户（除了发送者自己）
	for userID, client := range cm.clients {
		if userID == msg.SenderID {
			continue // 不发送给自己
		}

		if client.IsOnline {
			select {
			case client.Send <- msg:
				// 消息发送成功
			default:
				// 通道已满
				fmt.Printf("用户 %s 的消息通道已满\n", userID)
			}
		}
	}
}

// ==================== 房间管理 ====================

// CreateRoom 创建聊天室
func (cm *ConnectionManager) CreateRoom(room *models.ChatRoom) (*models.ChatRoom, error) {
	ctx := context.Background()

	// 创建房间
	if err := cm.roomRepo.Create(ctx, room); err != nil {
		return nil, fmt.Errorf("创建房间失败: %w", err)
	}

	// 添加成员到房间
	for _, member := range room.Members {
		if err := cm.roomRepo.AddMember(ctx, room.ID, member.UserID, member.Role); err != nil {
			// 记录错误但继续添加其他成员
			fmt.Printf("添加成员 %s 到房间失败: %v\n", member.UserID, err)
		}
	}

	// 通知成员加入房间
	cm.notifyRoomMembers(room, "您已被邀请加入聊天室")

	return room, nil
}

// JoinRoom 加入房间
func (cm *ConnectionManager) JoinRoom(roomID, userID string, role string) error {
	ctx := context.Background()

	// 检查房间是否存在
	room, err := cm.roomRepo.GetByID(ctx, roomID)
	if err != nil || room == nil {
		return fmt.Errorf("房间不存在")
	}

	// 检查用户是否已是成员
	isMember, err := cm.roomRepo.IsMember(ctx, roomID, userID)
	if err != nil {
		return fmt.Errorf("检查成员资格失败: %w", err)
	}

	// 如果用户已经是成员，则不需要重复加入，直接通知即可
	if isMember {
		// 通知房间成员该成员上线
		cm.notifyExistingMember(roomID, userID)
		return nil
	}

	// 加入房间
	if err := cm.roomRepo.AddMember(ctx, roomID, userID, role); err != nil {
		return fmt.Errorf("加入房间失败: %w", err)
	}

	// 通知房间成员有新成员加入
	cm.notifyNewMember(roomID, userID)

	return nil
}

// LeaveRoom 离开房间
func (cm *ConnectionManager) LeaveRoom(roomID, userID string) error {
	ctx := context.Background()

	if err := cm.roomRepo.RemoveMember(ctx, roomID, userID); err != nil {
		return fmt.Errorf("离开房间失败: %w", err)
	}

	// 通知房间成员有成员离开
	cm.notifyMemberLeft(roomID, userID)

	return nil
}

// ==================== 辅助方法 ====================

func (cm *ConnectionManager) broadcastUserStatus(userID, username string, isOnline bool) {
	statusMsg := models.Message{
		ID:            uuid.New().String(),
		MessageType:   models.MsgTypeSystem,
		SenderID:      "system",
		SenderName:    "系统",
		Content:       fmt.Sprintf("用户 %s %s", username, map[bool]string{true: "上线了", false: "离线了"}[isOnline]),
		RecipientType: models.RecipientBroadcast,
		Status:        models.MsgStatusSent,
		CreatedAt:     time.Now(),
		Metadata:      []byte(fmt.Sprintf(`{"user_id": "%s", "status": "%s"}`, userID, map[bool]string{true: "online", false: "offline"}[isOnline])),
	}

	cm.SendMessage(statusMsg)
}

func (cm *ConnectionManager) sendWelcomeMessage(client *models.Client) {
	welcomeMsg := models.Message{
		ID:            uuid.New().String(),
		MessageType:   models.MsgTypeSystem,
		SenderID:      "system",
		SenderName:    "系统",
		Content:       "欢迎来到聊天室！",
		RecipientType: models.RecipientUser,
		To:            []string{client.UserID},
		Status:        models.MsgStatusSent,
		CreatedAt:     time.Now(),
	}

	client.Send <- welcomeMsg

	// 发送未读消息
	cm.sendUnreadMessages(client)
}

func (cm *ConnectionManager) sendUnreadMessages(client *models.Client) {
	ctx := context.Background()

	// 获取用户加入的房间
	rooms, err := cm.roomRepo.GetByUserID(ctx, client.UserID)
	if err != nil {
		fmt.Printf("获取用户房间失败: %v\n", err)
		return
	}

	// 发送每个房间的未读消息
	for _, room := range rooms {
		unreadMessages, err := cm.messageRepo.GetUnreadMessages(ctx, client.UserID, room.ID)
		if err != nil {
			continue
		}

		if len(unreadMessages) > 0 {
			// 发送一条通知消息
			notifyMsg := models.Message{
				ID:            uuid.New().String(),
				MessageType:   models.MsgTypeSystem,
				SenderID:      "system",
				SenderName:    "系统",
				Content:       fmt.Sprintf("您在房间【%s】中有 %d 条未读消息", room.Name, len(unreadMessages)),
				RoomID:        room.ID,
				RecipientType: models.RecipientUser,
				To:            []string{client.UserID},
				Status:        models.MsgStatusSent,
				CreatedAt:     time.Now(),
			}

			client.Send <- notifyMsg
		}
	}
}

func (cm *ConnectionManager) notifyRoomMembers(room *models.ChatRoom, content string) {
	ctx := context.Background()

	members, err := cm.roomRepo.GetMembers(ctx, room.ID)
	if err != nil {
		return
	}

	for _, member := range members {
		cm.sendRoomNotification(room.ID, member.ID, content)
	}
}

func (cm *ConnectionManager) notifyNewMember(roomID, newMemberID string) {
	// 获取新成员信息
	ctx := context.Background()
	user, err := cm.userService.GetUserByID(ctx, newMemberID)
	if err != nil || user == nil {
		return
	}

	content := fmt.Sprintf("用户 %s 加入了聊天室", user.Username)

	// 获取房间所有成员（除了新成员自己）
	members, err := cm.roomRepo.GetMembers(ctx, roomID)
	if err != nil {
		return
	}

	for _, member := range members {
		if member.ID != newMemberID {
			cm.sendRoomNotification(roomID, member.ID, content)
		}
	}
}

func (cm *ConnectionManager) notifyExistingMember(roomID, userID string) {
	// 获取成员信息
	ctx := context.Background()
	user, err := cm.userService.GetUserByID(ctx, userID)
	if err != nil || user == nil {
		return
	}

	content := fmt.Sprintf("用户 %s 回到了聊天室", user.Username)

	// 获取房间所有成员（除了该成员自己）
	members, err := cm.roomRepo.GetMembers(ctx, roomID)
	if err != nil {
		return
	}

	for _, member := range members {
		if member.ID != userID {
			cm.sendRoomNotification(roomID, member.ID, content)
		}
	}
}

func (cm *ConnectionManager) notifyMemberLeft(roomID, memberID string) {
	// 类似 notifyNewMember，通知其他成员有人离开
	ctx := context.Background()
	user, err := cm.userService.GetUserByID(ctx, memberID)
	if err != nil || user == nil {
		return
	}

	content := fmt.Sprintf("用户 %s 离开了聊天室", user.Username)

	members, err := cm.roomRepo.GetMembers(ctx, roomID)
	if err != nil {
		return
	}

	for _, member := range members {
		if member.ID != memberID {
			cm.sendRoomNotification(roomID, member.ID, content)
		}
	}
}

func (cm *ConnectionManager) sendRoomNotification(roomID, userID, content string) {
	cm.mu.RLock()
	client, exists := cm.clients[userID]
	cm.mu.RUnlock()

	if !exists || !client.IsOnline {
		return
	}

	notification := models.Message{
		ID:            uuid.New().String(),
		MessageType:   models.MsgTypeSystem,
		SenderID:      "system",
		SenderName:    "系统",
		Content:       content,
		RoomID:        roomID,
		RecipientType: models.RecipientUser,
		To:            []string{userID},
		Status:        models.MsgStatusSent,
		CreatedAt:     time.Now(),
	}

	client.Send <- notification
}

func (cm *ConnectionManager) sendReadReceipt(msg models.Message) {
	receipt := models.ReadReceipt{
		ID:        uuid.New().String(),
		MessageID: msg.ID,
		UserID:    msg.SenderID,
		RoomID:    msg.RoomID,
		ReadAt:    time.Now(),
	}

	ctx := context.Background()
	if err := cm.messageRepo.CreateReadReceipt(ctx, &receipt); err != nil {
		fmt.Printf("创建已读回执失败: %v\n", err)
	}
}

// ==================== 连接管理 ====================

func (cm *ConnectionManager) readPump(client *models.Client) {
	defer func() {
		cm.UnregisterClient(client.UserID)
	}()

	client.Conn.SetReadLimit(1024 * 1024) // 1MB
	client.Conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	client.Conn.SetPongHandler(func(string) error {
		client.Conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		var msg models.Message
		err := client.Conn.ReadJSON(&msg)
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				fmt.Printf("读取错误: %v\n", err)
			}
			break
		}

		// 设置消息基本信息
		msg.ID = uuid.New().String()
		msg.SenderID = client.UserID
		msg.SenderName = client.Username
		msg.Status = models.MsgStatusSending
		msg.CreatedAt = time.Now()

		// 处理消息
		cm.handleIncomingMessage(client, msg)
	}
}

func (cm *ConnectionManager) writePump(client *models.Client) {
	ticker := time.NewTicker(30 * time.Second)
	defer func() {
		ticker.Stop()
		client.Conn.Close()
	}()

	for {
		select {
		case message, ok := <-client.Send:
			client.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))

			if !ok {
				// 通道关闭
				client.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			// 发送消息
			if err := client.Conn.WriteJSON(message); err != nil {
				fmt.Printf("发送消息失败: %v\n", err)
				return
			}

		case <-ticker.C:
			// 发送心跳包
			client.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := client.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func (cm *ConnectionManager) handleIncomingMessage(client *models.Client, msg models.Message) {
	// 验证消息类型和内容
	if !cm.validateMessage(msg) {
		cm.sendError(client, "消息格式错误")
		return
	}

	// 检查发送者是否在目标房间中（如果是房间消息）
	if msg.RecipientType == models.RecipientRoom && msg.RoomID != "" {
		isMember, err := cm.roomRepo.IsMember(context.Background(), msg.RoomID, client.UserID)
		if err != nil || !isMember {
			cm.sendError(client, "您不在该房间中")
			return
		}
	}

	// 发送消息
	if err := cm.SendMessage(msg); err != nil {
		cm.sendError(client, "发送消息失败")
		return
	}
}

func (cm *ConnectionManager) validateMessage(msg models.Message) bool {
	// 检查消息类型
	validTypes := map[string]bool{
		models.MsgTypeText:   true,
		models.MsgTypeImage:  true,
		models.MsgTypeFile:   true,
		models.MsgTypeVoice:  true,
		models.MsgTypeVideo:  true,
		models.MsgTypeSystem: true,
	}

	if !validTypes[msg.MessageType] {
		return false
	}

	// 检查接收者类型
	validRecipientTypes := map[string]bool{
		models.RecipientUser:      true,
		models.RecipientRoom:      true,
		models.RecipientBroadcast: true,
	}

	if !validRecipientTypes[msg.RecipientType] {
		return false
	}

	// 检查必要字段
	if msg.Content == "" && msg.MessageType == models.MsgTypeText {
		return false
	}

	if msg.RecipientType == models.RecipientUser && len(msg.To) == 0 {
		return false
	}

	if msg.RecipientType == models.RecipientRoom && msg.RoomID == "" {
		return false
	}

	return true
}

func (cm *ConnectionManager) sendError(client *models.Client, errorMsg string) {
	errorMessage := models.Message{
		ID:            uuid.New().String(),
		MessageType:   models.MsgTypeSystem,
		SenderID:      "system",
		SenderName:    "系统",
		Content:       errorMsg,
		RecipientType: models.RecipientUser,
		To:            []string{client.UserID},
		Status:        models.MsgStatusFailed,
		CreatedAt:     time.Now(),
	}

	client.Send <- errorMessage
}

func (cm *ConnectionManager) forceDisconnect(client *models.Client, reason string) {
	client.IsOnline = false

	// 发送断开通知
	disconnectMsg := models.Message{
		ID:            uuid.New().String(),
		MessageType:   models.MsgTypeSystem,
		SenderID:      "system",
		SenderName:    "系统",
		Content:       fmt.Sprintf("连接被断开: %s", reason),
		RecipientType: models.RecipientUser,
		To:            []string{client.UserID},
		Status:        models.MsgStatusSent,
		CreatedAt:     time.Now(),
	}

	select {
	case client.Send <- disconnectMsg:
	default:
		// 通道可能已满
	}

	close(client.Send)
	client.Conn.Close()

	delete(cm.clients, client.UserID)
}

// ==================== 查询方法 ====================

// GetOnlineUsers 获取在线用户列表
func (cm *ConnectionManager) GetOnlineUsers() []*models.User {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	onlineUsers := make([]*models.User, 0, len(cm.clients))

	for _, client := range cm.clients {
		if client.IsOnline {
			user := &models.User{
				ID:       client.UserID,
				Username: client.Username,
				Status:   "online",
				LastSeen: time.Now(),
			}
			onlineUsers = append(onlineUsers, user)
		}
	}

	return onlineUsers
}

// IsUserOnline 检查用户是否在线
func (cm *ConnectionManager) IsUserOnline(userID string) bool {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	client, exists := cm.clients[userID]
	return exists && client.IsOnline
}

// GetClientByUserID 根据用户ID获取客户端
func (cm *ConnectionManager) GetClientByUserID(userID string) (*models.Client, bool) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	client, exists := cm.clients[userID]
	return client, exists && client.IsOnline
}

// GetOnlineCount 获取在线用户数
func (cm *ConnectionManager) GetOnlineCount() int {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	count := 0
	for _, client := range cm.clients {
		if client.IsOnline {
			count++
		}
	}

	return count
}

// ==================== 管理方法 ====================

// Close 关闭ConnectionManager
func (cm *ConnectionManager) Close() {
	close(cm.closeChan)

	cm.mu.Lock()
	defer cm.mu.Unlock()

	// 断开所有连接
	for _, client := range cm.clients {
		cm.forceDisconnect(client, "服务器关闭")
	}

	// 清空客户端列表
	cm.clients = make(map[string]*models.Client)
}

// BroadcastSystemMessage 广播系统消息
func (cm *ConnectionManager) BroadcastSystemMessage(content string) {
	msg := models.Message{
		ID:            uuid.New().String(),
		MessageType:   models.MsgTypeSystem,
		SenderID:      "system",
		SenderName:    "系统",
		Content:       content,
		RecipientType: models.RecipientBroadcast,
		Status:        models.MsgStatusSent,
		CreatedAt:     time.Now(),
	}

	cm.SendMessage(msg)
}
