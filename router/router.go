// Package router provides HTTP routing configuration for the socket application.
//
// The API is structured as follows:
// Base URL: /api/v1
//
// Authentication:
// Most APIs require authentication via JWT token in the Authorization header:
// Authorization: Bearer <token>
package router

import (
	"socket/config"
	"socket/handlers"
	"socket/manager"
	"socket/middleware"
	"socket/repositories"
	"socket/services"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

// RouterConfig configures and returns a Gin engine with all routes registered.
//
// Endpoints:
//
// Public Routes:
//   POST /api/v1/register - User registration
//   POST /api/v1/login - User login
//
// Protected Routes (require authentication):
//   GET /api/v1/users - Get online users list
//   GET /api/v1/users/online - Get online users list
//   GET /api/v1/rooms/:room_id/messages - Get chat history for a room
//   GET /api/v1/rooms/:room_id/messages/search - Search messages in a room
//   GET /api/v1/rooms/:room_id/unread - Get unread message count for a room
//   POST /api/v1/rooms/:room_id/read - Mark messages as read
//   DELETE /api/v1/messages/:message_id - Recall a message
//   POST /api/v1/rooms - Create a new chat room
//   POST /api/v1/rooms/:room_id/join - Join a room
//   POST /api/v1/rooms/:room_id/leave - Leave a room
//   GET /api/v1/rooms/:room_id/members - Get room members
//
// WebSocket Routes:
//   GET /ws - Establish general WebSocket connection
//   GET /ws/rooms/:room_id - Establish WebSocket connection to a specific room
//
// Middlewares:
//   CORS - Cross-Origin Resource Sharing support
//   Auth - JWT token authentication
func RouterConfig(db *gorm.DB, redisClient interface{}) *gin.Engine {
	router := gin.Default()

	// 使用CORS中间件
	router.Use(middleware.CORS())

	// 初始化依赖
	userRepo := repositories.NewUserRepository(db)
	sessionRepo := repositories.NewSessionRepository(db, redisClient.(*redis.Client))
	friendRepo := repositories.NewFriendRepository(db)
	messageRepo := repositories.NewMessageRepository(db)
	roomRepo := repositories.NewRoomRepository(db)

	// 初始化服务
	userService := services.NewUserService(
		userRepo,
		sessionRepo,
		friendRepo,
		config.Conf.JWTSecret,
		config.Conf.JWTExpireHour,
	)

	chatService := services.NewChatService(
		messageRepo,
		userRepo,
		roomRepo,
	)

	// 初始化连接管理器
	connManager := manager.NewConnectionManager(
		userService,
		chatService,
		messageRepo,
		roomRepo,
	)

	// 初始化处理器
	httpHandler := handlers.NewHTTPHandler(userService, chatService)
	wsHandler := handlers.NewWebSocketHandler(chatService, userService, connManager)

	// API路由组
	api := router.Group("/api/v1")
	{
		// 用户相关路由
		// @Summary 用户注册
		// @Description 注册新用户
		// @Tags 用户
		// @Accept json
		// @Produce json
		// @Param request body object{username=string,password=string,email=string} true "注册信息"
		// @Success 200 {object} object{data=object{user_id=string,username=string,token=string}}
		// @Failure 400 {object} object{error=string,message=string}
		// @Failure 409 {object} object{error=string,message=string}
		// @Router /api/v1/register [post]
		api.POST("/register", httpHandler.Register)//
		
		// @Summary 用户登录
		// @Description 用户登录获取访问令牌
		// @Tags 用户
		// @Accept json
		// @Produce json
		// @Param request body object{username=string,password=string,ip=string,user_agent=string} true "登录信息"
		// @Success 200 {object} object{data=object{user_id=string,username=string,token=string}}
		// @Failure 400 {object} object{error=string,message=string}
		// @Failure 401 {object} object{error=string,message=string}
		// @Router /api/v1/login [post]
		api.POST("/login", httpHandler.Login)

		// 需要认证的路由
		auth := api.Group("/")
		auth.Use(middleware.Auth())
		{
			// @Summary 获取在线用户列表
			// @Description 获取当前在线的所有用户
			// @Tags 用户
			// @Produce json
			// @Success 200 {object} object{data=[]object{user_id=string,username=string,is_online=bool,last_seen=time.Time}}
			// @Failure 500 {object} object{error=string,message=string}
			// @Router /api/v1/users [get]
			// @Security Bearer
			auth.GET("/users", httpHandler.GetUsers)
			
			// @Summary 获取在线用户列表
			// @Description 获取当前在线的所有用户(同/api/v1/users)
			// @Tags 用户
			// @Produce json
			// @Success 200 {object} object{data=[]object{user_id=string,username=string,is_online=bool,last_seen=time.Time}}
			// @Failure 500 {object} object{error=string,message=string}
			// @Router /api/v1/users/online [get]
			// @Security Bearer
			auth.GET("/users/online", httpHandler.GetUsers)

			// @Summary 获取聊天记录
			// @Description 获取指定房间的历史聊天记录
			// @Tags 聊天
			// @Produce json
			// @Param room_id path string true "房间ID"
			// @Param page query string false "页码，默认为1" default(1)
			// @Param limit query string false "每页数量，默认为50" default(50)
			// @Success 200 {object} object{data=[]object{id=string,sender_id=string,content=string,created_at=time.Time}}
			// @Failure 404 {object} object{error=string,message=string}
			// @Router /api/v1/rooms/{room_id}/messages [get]
			// @Security Bearer
			auth.GET("/rooms/:room_id/messages", httpHandler.GetHistory)
			
			// @Summary 搜索消息
			// @Description 在指定房间中搜索包含关键词的消息
			// @Tags 聊天
			// @Produce json
			// @Param room_id path string true "房间ID"
			// @Param keyword query string true "搜索关键词"
			// @Param limit query string false "返回结果数量限制，默认为50" default(50)
			// @Success 200 {object} object{data=[]object{id=string,sender_id=string,content=string,created_at=time.Time}}
			// @Failure 400 {object} object{error=string,message=string}
			// @Failure 500 {object} object{error=string,message=string}
			// @Router /api/v1/rooms/{room_id}/messages/search [get]
			// @Security Bearer
			auth.GET("/rooms/:room_id/messages/search", httpHandler.SearchMessages)
			
			// @Summary 获取未读消息数
			// @Description 获取指定房间中的未读消息数量
			// @Tags 聊天
			// @Produce json
			// @Param room_id path string true "房间ID"
			// @Success 200 {object} object{data=object{room_id=string,unread_count=int}}
			// @Failure 401 {object} object{error=string,message=string}
			// @Failure 500 {object} object{error=string,message=string}
			// @Router /api/v1/rooms/{room_id}/unread [get]
			// @Security Bearer
			auth.GET("/rooms/:room_id/unread", httpHandler.GetUnreadCount)
			
			// @Summary 标记消息为已读
			// @Description 将指定消息标记为已读
			// @Tags 聊天
			// @Accept json
			// @Produce json
			// @Param room_id path string true "房间ID"
			// @Param request body object{message_ids=[]string} true "要标记为已读的消息ID数组"
			// @Success 200 {object} object{data=nil}
			// @Failure 400 {object} object{error=string,message=string}
			// @Failure 401 {object} object{error=string,message=string}
			// @Failure 500 {object} object{error=string,message=string}
			// @Router /api/v1/rooms/{room_id}/read [post]
			// @Security Bearer
			auth.POST("/rooms/:room_id/read", httpHandler.MarkAsRead)
			
			// @Summary 撤回消息
			// @Description 撤回已发送的消息
			// @Tags 聊天
			// @Accept json
			// @Produce json
			// @Param message_id path string true "消息ID"
			// @Param request body object{reason=string} false "撤回原因"
			// @Success 200 {object} object{data=nil}
			// @Failure 400 {object} object{error=string,message=string}
			// @Failure 401 {object} object{error=string,message=string}
			// @Failure 500 {object} object{error=string,message=string}
			// @Router /api/v1/messages/{message_id} [delete]
			// @Security Bearer
			auth.DELETE("/messages/:message_id", httpHandler.RecallMessage)

			// @Summary 创建聊天室
			// @Description 创建一个新的聊天室
			// @Tags 房间
			// @Accept json
			// @Produce json
			// @Param request body object{name=string,description=string,type=string,is_public=bool,member_ids=[]string} true "房间信息"
			// @Success 200 {object} object{data=object{id=string,name=string,description=string,type=string,creator_id=string,is_public=bool,members=[]object{id=string,room_id=string,user_id=string,role=string}}}
			// @Failure 400 {object} object{error=string,message=string}
			// @Failure 401 {object} object{error=string,message=string}
			// @Failure 500 {object} object{error=string,message=string}
			// @Router /api/v1/rooms [post]
			// @Security Bearer
			auth.POST("/rooms", httpHandler.CreateRoom)
			
			// @Summary 加入房间
			// @Description 加入指定的聊天室
			// @Tags 房间
			// @Produce json
			// @Param room_id path string true "房间ID"
			// @Success 200 {object} object{data=object{message=string}}
			// @Failure 400 {object} object{error=string,message=string}
			// @Failure 401 {object} object{error=string,message=string}
			// @Failure 500 {object} object{error=string,message=string}
			// @Router /api/v1/rooms/{room_id}/join [post]
			// @Security Bearer
			auth.POST("/rooms/:room_id/join", httpHandler.JoinRoom)
			
			// @Summary 离开房间
			// @Description 离开指定的聊天室
			// @Tags 房间
			// @Produce json
			// @Param room_id path string true "房间ID"
			// @Success 200 {object} object{data=object{message=string}}
			// @Failure 400 {object} object{error=string,message=string}
			// @Failure 401 {object} object{error=string,message=string}
			// @Failure 500 {object} object{error=string,message=string}
			// @Router /api/v1/rooms/{room_id}/leave [post]
			// @Security Bearer
			auth.POST("/rooms/:room_id/leave", httpHandler.LeaveRoom)
			
			// @Summary 获取房间成员
			// @Description 获取指定房间的所有成员
			// @Tags 房间
			// @Produce json
			// @Param room_id path string true "房间ID"
			// @Success 200 {object} object{data=[]object{id=string,room_id=string,user_id=string,role=string,joined_at=time.Time}}
			// @Failure 400 {object} object{error=string,message=string}
			// @Failure 401 {object} object{error=string,message=string}
			// @Failure 500 {object} object{error=string,message=string}
			// @Router /api/v1/rooms/{room_id}/members [get]
			// @Security Bearer
			auth.GET("/rooms/:room_id/members", httpHandler.GetRoomMembers)
		}
	}

	// WebSocket路由
	// @Summary 建立WebSocket连接
	// @Description 建立通用WebSocket连接
	// @Tags WebSocket
	// @Param user_id query string false "用户ID(可选，如果不在查询参数则通过JWT获取)"
	// @Param username query string false "用户名(可选，如果不在查询参数则通过JWT获取)"
	// @Param Authorization header string false "Bearer Token (当user_id和username不在查询参数时必需)"
	// @Success 101 {string} string "Switching Protocols"
	// @Failure 400 {object} object{error=string}
	// @Failure 401 {object} object{error=string}
	// @Router /ws [get]
	router.GET("/ws", wsHandler.Connect)
	
	// @Summary 建立房间WebSocket连接
	// @Description 建立与特定房间的WebSocket连接并自动加入该房间
	// @Tags WebSocket
	// @Param room_id path string true "房间ID"
	// @Param Authorization header string true "Bearer Token"
	// @Success 101 {string} string "Switching Protocols"
	// @Failure 400 {object} object{error=string}
	// @Failure 401 {object} object{error=string}
	// @Router /ws/rooms/{room_id} [get]
	router.GET("/ws/rooms/:room_id", wsHandler.JoinRoom)

	return router
}