// Package handlers provides HTTP request handlers for the socket application.
//
// This package contains two main handler types:
// 1. HTTPHandler - handles RESTful API requests
// 2. WebSocketHandler - handles WebSocket connections for real-time communication
//
// The handlers act as an interface between the HTTP layer and the business logic
// implemented in the services package.
package handlers

import (
	"net/http"
	"socket/models"
	"socket/services"
	"socket/utils"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// HTTPHandler handles HTTP RESTful API requests.
//
// It depends on UserService and ChatService to perform business operations.
type HTTPHandler struct {
	userService services.UserService
	chatService services.ChatService
}

// NewHTTPHandler creates a new HTTPHandler instance.
//
// Parameters:
//   - userService: Service for user-related operations
//   - chatService: Service for chat-related operations
//
// Returns:
//   - *HTTPHandler: New HTTPHandler instance
func NewHTTPHandler(userService services.UserService, chatService services.ChatService) *HTTPHandler {
	return &HTTPHandler{
		userService: userService,
		chatService: chatService,
	}
}

// Register handles user registration requests.
//
// Accepts username, password, and email in JSON format.
// Creates a new user account and returns a JWT token for authentication.
//
// Request:
//
//	POST /api/v1/register
//	Content-Type: application/json
//	{
//	  "username": "example_user",
//	  "password": "secure_password",
//	  "email": "user@example.com"
//	}
//
// Response:
//
//	{
//	  "code": 200,
//	  "message": "success",
//	  "data": {
//	    "user_id": "user_uuid",
//	    "username": "example_user",
//	    "token": "jwt_token"
//	  }
//	}
func (h *HTTPHandler) Register(c *gin.Context) {
	var req struct {
		Username string `json:"username" binding:"required,min=3,max=20"`
		Password string `json:"password" binding:"required,min=6"`
		Email    string `json:"email" binding:"required,email"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, "参数错误", err)
		return
	}

	user, token, err := h.userService.Register(c.Request.Context(), req.Username, req.Password, req.Email)
	if err != nil {
		utils.ErrorResponse(c, http.StatusConflict, "注册失败", err)
		return
	}

	utils.SuccessResponse(c, gin.H{
		"user_id":  user.ID,
		"username": user.Username,
		"token":    token,
	})
}

// Login handles user authentication requests.
//
// Accepts username and password in JSON format.
// Validates credentials and returns a JWT token for authenticated sessions.
//
// Request:
//
//	POST /api/v1/login
//	Content-Type: application/json
//	{
//	  "username": "example_user",
//	  "password": "secure_password",
//	  "ip": "optional_client_ip",
//	  "user_agent": "optional_user_agent"
//	}
//
// Response:
//
//	{
//	  "code": 200,
//	  "message": "success",
//	  "data": {
//	    "user_id": "user_uuid",
//	    "username": "example_user",
//	    "token": "jwt_token"
//	  }
//	}
func (h *HTTPHandler) Login(c *gin.Context) {
	var req struct {
		Username  string `json:"username" binding:"required"`
		Password  string `json:"password" binding:"required"`
		IP        string `json:"ip"`
		UserAgent string `json:"user_agent"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, "参数错误", err)
		return
	}

	user, token, err := h.userService.Login(c.Request.Context(), req.Username, req.Password, req.IP, req.UserAgent)
	if err != nil {
		utils.ErrorResponse(c, http.StatusUnauthorized, "登录失败", err)
		return
	}

	utils.SuccessResponse(c, gin.H{
		"user_id":  user.ID,
		"username": user.Username,
		"token":    token,
	})
}

// GetUsers retrieves the list of online users.
//
// Returns information about all currently connected users.
//
// Request:
//
//	GET /api/v1/users
//
// Response:
//
//	{
//	  "code": 200,
//	  "message": "success",
//	  "data": [
//	    {
//	      "user_id": "user_uuid",
//	      "username": "example_user",
//	      "is_online": true,
//	      "last_seen": "2023-01-01T00:00:00Z"
//	    }
//	  ]
//	}
func (h *HTTPHandler) GetUsers(c *gin.Context) {
	users, err := h.userService.GetOnlineUsers(c.Request.Context())
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "获取用户列表失败", err)
		return
	}
	utils.SuccessResponse(c, users)
}

// GetHistory retrieves chat message history for a specific room.
//
// Fetches paginated message history from the database.
//
// Request:
//
//	GET /api/v1/rooms/{room_id}/messages?page=1&limit=50
//
// Response:
//
//	{
//	  "code": 200,
//	  "message": "success",
//	  "data": [
//	    {
//	      "id": "message_uuid",
//	      "sender_id": "user_uuid",
//	      "content": "Hello world!",
//	      "created_at": "2023-01-01T00:00:00Z"
//	    }
//	  ]
//	}
func (h *HTTPHandler) GetHistory(c *gin.Context) {
	roomID := c.Param("room_id")
	page := c.DefaultQuery("page", "1")
	limit := c.DefaultQuery("limit", "50")

	messages, err := h.chatService.GetChatHistory(c.Request.Context(), roomID, page, limit)
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "获取历史失败", err)
		return
	}

	utils.SuccessResponse(c, messages)
}

// SearchMessages searches for messages containing a specific keyword.
//
// Searches through message content in a given room.
//
// Request:
//
//	GET /api/v1/rooms/{room_id}/messages/search?keyword=hello&limit=50
//
// Response:
//
//	{
//	  "code": 200,
//	  "message": "success",
//	  "data": [
//	    {
//	      "id": "message_uuid",
//	      "sender_id": "user_uuid",
//	      "content": "Hello world!",
//	      "created_at": "2023-01-01T00:00:00Z"
//	    }
//	  ]
//	}
func (h *HTTPHandler) SearchMessages(c *gin.Context) {
	roomID := c.Param("room_id")
	keyword := c.Query("keyword")
	limit := c.DefaultQuery("limit", "50")

	if keyword == "" {
		utils.ErrorResponse(c, http.StatusBadRequest, "搜索关键词不能为空", nil)
		return
	}

	messages, err := h.chatService.SearchMessages(c.Request.Context(), keyword, roomID, limit)
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "搜索失败", err)
		return
	}

	utils.SuccessResponse(c, messages)
}

// GetUnreadCount retrieves the count of unread messages in a room for a user.
//
// Calculates how many messages the user hasn't read yet in a specific room.
//
// Request:
//
//	GET /api/v1/rooms/{room_id}/unread
//
// Response:
//
//	{
//	  "code": 200,
//	  "message": "success",
//	  "data": {
//	    "room_id": "room_uuid",
//	    "unread_count": 5
//	  }
//	}
func (h *HTTPHandler) GetUnreadCount(c *gin.Context) {
	roomID := c.Param("room_id")
	userID, exists := c.Get("user_id")
	if !exists {
		utils.ErrorResponse(c, http.StatusUnauthorized, "未授权", nil)
		return
	}

	count, err := h.chatService.GetUnreadCount(c.Request.Context(), userID.(string), roomID)
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "获取未读数失败", err)
		return
	}

	utils.SuccessResponse(c, gin.H{
		"room_id":      roomID,
		"unread_count": count,
	})
}

// MarkAsRead marks specific messages as read by a user.
//
// Updates message status to indicate they've been read by the user.
//
// Request:
//
//	POST /api/v1/rooms/{room_id}/read
//	Content-Type: application/json
//	{
//	  "message_ids": ["msg_uuid1", "msg_uuid2"]
//	}
//
// Response:
//
//	{
//	  "code": 200,
//	  "message": "success",
//	  "data": null
//	}
func (h *HTTPHandler) MarkAsRead(c *gin.Context) {
	roomID := c.Param("room_id")
	userID, exists := c.Get("user_id")
	if !exists {
		utils.ErrorResponse(c, http.StatusUnauthorized, "未授权", nil)
		return
	}

	var req struct {
		MessageIDs []string `json:"message_ids" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, "参数错误", err)
		return
	}

	if err := h.chatService.MarkAsRead(c.Request.Context(), userID.(string), roomID, req.MessageIDs); err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "标记已读失败", err)
		return
	}

	utils.SuccessResponse(c, nil)
}

// RecallMessage allows a user to retract a previously sent message.
//
// Removes a message from public view, though it may still exist in logs.
//
// Request:
//
//	DELETE /api/v1/messages/{message_id}
//	Content-Type: application/json
//	{
//	  "reason": "optional_reason_for_recall"
//	}
//
// Response:
//
//	{
//	  "code": 200,
//	  "message": "success",
//	  "data": null
//	}
func (h *HTTPHandler) RecallMessage(c *gin.Context) {
	messageID := c.Param("message_id")
	userID, exists := c.Get("user_id")
	if !exists {
		utils.ErrorResponse(c, http.StatusUnauthorized, "未授权", nil)
		return
	}

	var req struct {
		Reason string `json:"reason"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, "参数错误", err)
		return
	}

	if err := h.chatService.RecallMessage(c.Request.Context(), messageID, userID.(string), req.Reason); err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "撤回消息失败", err)
		return
	}

	utils.SuccessResponse(c, nil)
}

// CreateRoom creates a new chat room.
//
// Supports creation of private, group, or channel rooms with customizable settings.
//
// Request:
//
//	POST /api/v1/rooms
//	Content-Type: application/json
//	{
//	  "name": "Room Name",
//	  "description": "Optional description",
//	  "type": "private|group|channel",
//	  "is_public": true,
//	  "member_ids": ["user_uuid1", "user_uuid2"]
//	}
//
// Response:
//
//	{
//	  "code": 200,
//	  "message": "success",
//	  "data": {
//	    "id": "room_uuid",
//	    "name": "Room Name",
//	    "description": "Optional description",
//	    "type": "private|group|channel",
//	    "creator_id": "user_uuid",
//	    "is_public": true,
//	    "members": [...]
//	  }
//	}
func (h *HTTPHandler) CreateRoom(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		utils.ErrorResponse(c, http.StatusUnauthorized, "未授权", nil)
		return
	}

	var req struct {
		Name        string   `json:"name" binding:"required"`
		Description string   `json:"description"`
		Type        string   `json:"type" binding:"required"` // private, group, channel
		IsPublic    bool     `json:"is_public"`
		MemberIDs   []string `json:"member_ids"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, "参数错误", err)
		return
	}

	// 创建房间
	room := &models.ChatRoom{
		ID:          uuid.New().String(),
		Name:        req.Name,
		Description: req.Description,
		Type:        req.Type,
		CreatorID:   userID.(string),
		IsPublic:    req.IsPublic,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	// 生成房间ID
	if room.ID == "" {
		room.ID = uuid.New().String()
	}

	// 设置创建时间
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
	// 生成邀请码以避免唯一索引冲突
	if room.InviteCode == "" {
		room.InviteCode = uuid.New().String()[:8]
	}

	// 添加成员
	if len(req.MemberIDs) > 0 {
		for _, memberID := range req.MemberIDs {
			if memberID == userID.(string) {
				continue
			}
			member := &models.RoomMember{
				ID:       uuid.New().String(),
				RoomID:   room.ID,
				UserID:   memberID,
				Role:     "member",
				JoinedAt: time.Now(),
			}
			room.Members = append(room.Members, member)
		}
	}

	// 创建者自动成为owner
	owner := &models.RoomMember{
		ID:       uuid.New().String(),
		RoomID:   room.ID,
		UserID:   userID.(string),
		Role:     "owner",
		JoinedAt: time.Now(),
	}
	room.Members = append(room.Members, owner)

	if err := h.chatService.CreateRoom(c.Request.Context(), room); err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "创建房间失败", err)
		return
	}

	utils.SuccessResponse(c, room)
}

// JoinRoom allows a user to join an existing room.
//
// Adds the authenticated user to the specified room as a member.
//
// Request:
//
//	POST /api/v1/rooms/{room_id}/join
//
// Response:
//
//	{
//	  "code": 200,
//	  "message": "success",
//	  "data": {
//	    "message": "成功加入房间"
//	  }
//	}
func (h *HTTPHandler) JoinRoom(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		utils.ErrorResponse(c, http.StatusUnauthorized, "未授权", nil)
		return
	}

	roomID := c.Param("room_id")
	if roomID == "" {
		utils.ErrorResponse(c, http.StatusBadRequest, "缺少房间ID", nil)
		return
	}

	if err := h.chatService.JoinRoom(c.Request.Context(), roomID, userID.(string), "member"); err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "加入房间失败", err)
		return
	}

	utils.SuccessResponse(c, gin.H{"message": "成功加入房间"})
}

// LeaveRoom allows a user to leave a room they're currently in.
//
// Removes the authenticated user from the specified room.
//
// Request:
//
//	POST /api/v1/rooms/{room_id}/leave
//
// Response:
//
//	{
//	  "code": 200,
//	  "message": "success",
//	  "data": {
//	    "message": "成功离开房间"
//	  }
//	}
func (h *HTTPHandler) LeaveRoom(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		utils.ErrorResponse(c, http.StatusUnauthorized, "未授权", nil)
		return
	}

	roomID := c.Param("room_id")
	if roomID == "" {
		utils.ErrorResponse(c, http.StatusBadRequest, "缺少房间ID", nil)
		return
	}

	if err := h.chatService.LeaveRoom(c.Request.Context(), roomID, userID.(string)); err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "离开房间失败", err)
		return
	}

	utils.SuccessResponse(c, gin.H{"message": "成功离开房间"})
}

// GetRoomMembers retrieves the list of members in a specific room.
//
// Returns information about all users who are members of the specified room.
//
// Request:
//
//	GET /api/v1/rooms/{room_id}/members
//
// Response:
//
//	{
//	  "code": 200,
//	  "message": "success",
//	  "data": [
//	    {
//	      "id": "membership_uuid",
//	      "room_id": "room_uuid",
//	      "user_id": "user_uuid",
//	      "role": "owner|admin|member",
//	      "joined_at": "2023-01-01T00:00:00Z"
//	    }
//	  ]
//	}
func (h *HTTPHandler) GetRoomMembers(c *gin.Context) {
	roomID := c.Param("room_id")
	if roomID == "" {
		utils.ErrorResponse(c, http.StatusBadRequest, "缺少房间ID", nil)
		return
	}

	members, err := h.chatService.GetRoomMembers(c.Request.Context(), roomID)
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "获取房间成员失败", err)
		return
	}

	utils.SuccessResponse(c, members)
}

// GetUserRooms retrieves the list of rooms that the authenticated user has joined.
//
// Returns information about all rooms the user is a member of.
//
// Request:
//
//	GET /api/v1/user/rooms
//
// Response:
//
//	{
//	  "code": 200,
//	  "message": "success",
//	  "data": [
//	    {
//	      "id": "room_uuid",
//	      "name": "Room Name",
//	      "description": "Optional description",
//	      "type": "private|group|channel",
//	      "creator_id": "user_uuid",
//	      "is_public": true,
//	      "members": [...]
//	    }
//	  ]
//	}
func (h *HTTPHandler) GetUserRooms(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		utils.ErrorResponse(c, http.StatusUnauthorized, "未授权", nil)
		return
	}

	rooms, err := h.chatService.GetUserRooms(c.Request.Context(), userID.(string))
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "获取房间列表失败", err)
		return
	}

	utils.SuccessResponse(c, rooms)
}
