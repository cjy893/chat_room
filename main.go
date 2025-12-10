// Package main is the entry point for the socket application.
//
// This is a real-time chat application with the following features:
// 1. User registration and authentication with JWT tokens
// 2. Room-based messaging with support for private, group, and channel chats
// 3. Real-time communication via WebSocket connections
// 4. Message history and search capabilities
// 5. Online presence detection
// 6. Message recall functionality
//
// API Documentation:
// See docs/api.md for detailed API documentation
// Or check router/router.go for inline API documentation
package main

import (
	"fmt"
	"log"
	"socket/config"
	"socket/models"
	"socket/router"

	"github.com/gin-contrib/cors"
	"github.com/redis/go-redis/v9"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// main initializes the application components and starts the HTTP server.
//
// Initialization sequence:
// 1. Load configuration from environment variables or config files
// 2. Connect to the database
// 3. Initialize Redis connection (if needed)
// 4. Set up routing with all API endpoints
// 5. Start the HTTP server
func main() {
	// 加载配置
	config.LoadConfig()

	// 初始化数据库连接
	db, err := initDB()
	if err != nil {
		log.Fatal("Failed to connect to database:", err)
	}

	// 自动迁移数据库模型
	if err := autoMigrate(db); err != nil {
		log.Fatal("Failed to migrate database:", err)
	}

	redisClient := redis.NewClient(&redis.Options{
		Addr:     config.Conf.RedisAddr,
		Password: config.Conf.RedisPassword,
		DB:       config.Conf.RedisDB,
	})

	// 初始化路由
	r := router.RouterConfig(db, redisClient)

	r.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"http://localhost:3000"}, // 前端地址
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * 60 * 60,
	}))

	// 启动服务器
	log.Printf("Server starting on port %s", config.Conf.ServerPort)
	if err := r.Run(config.Conf.ServerPort); err != nil {
		log.Fatal("Failed to start server:", err)
	}
}

// initDB initializes the database connection using configuration settings.
//
// Returns:
//   - *gorm.DB: Database connection instance
//   - error: Error if connection failed, nil otherwise
func initDB() (*gorm.DB, error) {
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&loc=PRC&parseTime=true&allowNativePasswords=true",
		config.Conf.DBUser,
		config.Conf.DBPassword,
		config.Conf.DBHost,
		config.Conf.DBPort,
		config.Conf.DBName,
	)

	return gorm.Open(mysql.Open(dsn), &gorm.Config{})
}

// autoMigrate performs automatic database schema migration for all models.
//
// This function ensures that all database tables exist and are up to date
// with the current model definitions. It creates tables if they don't exist
// and adds columns if new fields are added to models.
//
// Parameters:
//   - db: Active GORM database connection
//
// Returns:
//   - error: Any error encountered during migration, or nil if successful
func autoMigrate(db *gorm.DB) error {
	// 迁移所有模型
	// 注意：迁移顺序很重要，需要先迁移被其他模型依赖的表
	err := db.AutoMigrate(
		&models.User{},
		&models.UserSession{},
		&models.Friend{},
		&models.ChatRoom{},
		&models.RoomMember{},
		&models.Message{},
		&models.ReadReceipt{},
		&models.TokenBlacklist{},
	)

	if err != nil {
		return fmt.Errorf("error during auto migration: %w", err)
	}

	log.Println("Database migration completed successfully")
	return nil
}
