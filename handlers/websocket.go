// Package handlers provides WebSocket connection handlers for real-time communication.
//
// This package manages real-time communication between clients using WebSocket protocol.
// It handles connection upgrades, message broadcasting, and room management for live chat.
package handlers

import (
	"net/http"
	"socket/manager"
	"socket/models"
	"socket/services"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

// WebSocketHandler handles WebSocket connection requests.
//
// It manages upgrading HTTP connections to WebSocket connections,
// authenticates users, and registers them with the connection manager.
type WebSocketHandler struct {
	upgrader    websocket.Upgrader
	chatService services.ChatService
	userService services.UserService
	connManager *manager.ConnectionManager
}

// NewWebSocketHandler creates a new WebSocketHandler instance.
//
// Parameters:
//   - chatService: Service for chat-related operations
//   - userService: Service for user-related operations
//   - connManager: Manager for handling active WebSocket connections
//
// Returns:
//   - *WebSocketHandler: New WebSocketHandler instance
func NewWebSocketHandler(
	chatService services.ChatService,
	userService services.UserService,
	connManager *manager.ConnectionManager,
) *WebSocketHandler {
	return &WebSocketHandler{
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			CheckOrigin: func(r *http.Request) bool {
				// 生产环境需要更严格的检查
				return true
			},
		},
		chatService: chatService,
		userService: userService,
		connManager: connManager,
	}
}

// Connect handles general WebSocket connection requests.
//
// Upgrades HTTP connection to WebSocket and registers the client with the connection manager.
// User authentication can be provided either through query parameters or JWT token in the Authorization header.
//
// Request:
//
//	GET /ws?user_id={user_id}&username={username}
//	OR
//	GET /ws
//	Authorization: Bearer {jwt_token}
//
// Response:
//
//	101 Switching Protocols (on successful upgrade)
func (h *WebSocketHandler) Connect(c *gin.Context) {
	// 获取用户身份（从JWT或查询参数）
	userID := c.Query("user_id")
	username := c.Query("username")

	// 如果查询参数中没有用户信息，则从 JWT token 中获取
	if userID == "" || username == "" {
		// 从 Authorization header 中获取 token
		token := c.Query("token")

		if token == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "缺少认证信息"})
			return
		}

		// 解析 token 获取用户信息
		ctx := c.Request.Context()
		user, err := h.userService.ValidateToken(ctx, token)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "无效的认证令牌"})
			return
		}

		userID = user.ID
		username = user.Username
	}

	if userID == "" || username == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少用户信息"})
		return
	}

	// 升级到 WebSocket 连接
	conn, err := h.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "WebSocket升级失败"})
		return
	}

	// 创建客户端对象
	client := &models.Client{
		UserID:   userID,
		Username: username,
		Conn:     conn,
		Send:     make(chan models.Message, 256),
		OnlineAt: time.Now(),
		IsOnline: true,
		IP:       c.ClientIP(),
	}

	// 注册客户端连接到连接管理器
	h.connManager.RegisterClient(client)

	// 连接管理和消息处理将在 ConnectionManager 中完成
}

// JoinRoom handles WebSocket connection requests to a specific chat room.
//
// Upgrades HTTP connection to WebSocket, authenticates the user via JWT token,
// registers the client, and automatically joins the specified room.
//
// Request:
//
//	GET /ws/rooms/{room_id}
//	Authorization: Bearer {jwt_token}
//
// Response:
//
//	101 Switching Protocols (on successful upgrade)
func (h *WebSocketHandler) JoinRoom(c *gin.Context) {
	token := c.Query("token")

	if token == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少认证信息"})
		return
	}

	// 解析 token 获取用户信息
	ctx := c.Request.Context()
	user, err := h.userService.ValidateToken(ctx, token)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "无效的认证令牌"})
		return
	}

	userID := user.ID
	username := user.Username

	roomID := c.Param("room_id")
	if roomID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少房间ID"})
		return
	}

	conn, err := h.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "WebSocket升级失败"})
		return
	}

	// 创建客户端对象
	client := &models.Client{
		UserID:   userID,
		Username: username,
		Conn:     conn,
		Send:     make(chan models.Message, 256),
		OnlineAt: time.Now(),
		IsOnline: true,
		IP:       c.ClientIP(),
	}

	// 注册客户端连接到连接管理器
	h.connManager.RegisterClient(client)

	// 加入指定房间
	if err := h.connManager.JoinRoom(roomID, userID, "member"); err != nil {
		conn.WriteJSON(gin.H{"error": err.Error()})
		conn.Close()
		return
	}

	// 发送加入房间成功的通知
	joinMsg := models.Message{
		ID:            uuid.New().String(),
		MessageType:   models.MsgTypeSystem,
		SenderID:      "system",
		SenderName:    "系统",
		Content:       "成功加入房间",
		RoomID:        roomID,
		RecipientType: models.RecipientUser,
		To:            []string{userID},
		Status:        models.MsgStatusSent,
		CreatedAt:     time.Now(),
	}

	client.Send <- joinMsg
}
