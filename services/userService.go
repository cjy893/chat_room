package services

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"socket/models"
	"socket/repositories"
	"time"

	"github.com/golang-jwt/jwt/v4"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

var (
	ErrUserNotFound       = errors.New("用户不存在")
	ErrInvalidCredentials = errors.New("用户名或密码错误")
	ErrUserExists         = errors.New("用户已存在")
	ErrInvalidToken       = errors.New("无效的令牌")
	ErrTokenExpired       = errors.New("令牌已过期")
)

type userServiceImpl struct {
	userRepo    repositories.UserRepository
	sessionRepo repositories.SessionRepository
	friendRepo  repositories.FriendRepository
	jwtSecret   []byte
	jwtExpiry   time.Duration
}

func NewUserService(
	userRepo repositories.UserRepository,
	sessionRepo repositories.SessionRepository,
	friendRepo repositories.FriendRepository,
	jwtSecret string,
	jwtExpiryHours int,
) UserService {
	return &userServiceImpl{
		userRepo:    userRepo,
		sessionRepo: sessionRepo,
		friendRepo:  friendRepo,
		jwtSecret:   []byte(jwtSecret),
		jwtExpiry:   time.Duration(jwtExpiryHours) * time.Hour,
	}
}

func (s *userServiceImpl) Register(ctx context.Context, username, password, email string) (*models.User, string, error) {
	// 检查用户是否已存在
	existing, _ := s.userRepo.GetByUsername(ctx, username)
	if existing != nil {
		return nil, "", ErrUserExists
	}

	// 检查邮箱是否已存在
	if email != "" {
		existing, _ = s.userRepo.GetByEmail(ctx, email)
		if existing != nil {
			return nil, "", errors.New("邮箱已被注册")
		}
	}

	// 密码加密
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, "", fmt.Errorf("密码加密失败: %w", err)
	}

	// 创建用户
	user := &models.User{
		ID:        uuid.New().String(),
		Username:  username,
		Password:  string(hashedPassword),
		Email:     email,
		Status:    "offline",
		LastSeen:  time.Now(),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := s.userRepo.Create(ctx, user); err != nil {
		return nil, "", fmt.Errorf("创建用户失败: %w", err)
	}

	// 生成令牌
	token, err := s.generateToken(user)
	if err != nil {
		return user, "", fmt.Errorf("生成令牌失败: %w", err)
	}

	return user, token, nil
}

func (s *userServiceImpl) Login(ctx context.Context, username, password, ip, userAgent string) (*models.User, string, error) {
	// 获取用户
	user, err := s.userRepo.GetByUsername(ctx, username)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, "", ErrInvalidCredentials
		}
		return nil, "", fmt.Errorf("查询用户失败: %w", err)
	}

	// 验证密码
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)); err != nil {
		return nil, "", ErrInvalidCredentials
	}

	// 生成令牌
	token, err := s.generateToken(user)
	if err != nil {
		return nil, "", fmt.Errorf("生成令牌失败: %w", err)
	}

	// 创建会话
	session := &models.UserSession{
		ID:           uuid.New().String(),
		UserID:       user.ID,
		Token:        s.hashToken(token),
		RefreshToken: uuid.New().String(),
		ExpiresAt:    time.Now().Add(s.jwtExpiry),
		IPAddress:    ip,
		UserAgent:    userAgent,
		CreatedAt:    time.Now(),
	}

	if err := s.sessionRepo.Create(ctx, session); err != nil {
		return nil, "", fmt.Errorf("创建会话失败: %w", err)
	}

	// 更新用户状态为在线
	user.Status = "online"
	user.LastSeen = time.Now()
	if err := s.userRepo.Update(ctx, user); err != nil {
		// 日志记录错误，但不影响登录流程
		fmt.Printf("更新用户状态失败: %v\n", err)
	}

	return user, token, nil
}

func (s *userServiceImpl) Logout(ctx context.Context, token string) error {
	tokenHash := s.hashToken(token)

	// 从数据库中删除会话
	if err := s.sessionRepo.DeleteByToken(ctx, tokenHash); err != nil {
		return fmt.Errorf("注销失败: %w", err)
	}

	// 解析令牌获取用户ID
	claims, err := s.parseToken(token)
	if err != nil {
		// 如果令牌无效，仍然认为注销成功
		return nil
	}

	// 更新用户状态为离线
	user, err := s.userRepo.GetByID(ctx, claims.UserID)
	if err == nil && user != nil {
		user.Status = "offline"
		user.LastSeen = time.Now()
		s.userRepo.Update(ctx, user)
	}

	return nil
}

func (s *userServiceImpl) ValidateToken(ctx context.Context, token string) (*models.User, error) {
	// 验证令牌签名
	claims, err := s.parseToken(token)
	if err != nil {
		return nil, err
	}

	// 检查令牌是否过期
	if claims.ExpiresAt.Before(time.Now()) {
		return nil, ErrTokenExpired
	}

	// 检查会话是否在数据库中
	tokenHash := s.hashToken(token)
	session, err := s.sessionRepo.GetByToken(ctx, tokenHash)
	if err != nil || session == nil {
		return nil, ErrInvalidToken
	}

	// 获取用户信息
	user, err := s.userRepo.GetByID(ctx, claims.UserID)
	if err != nil {
		return nil, ErrUserNotFound
	}

	return user, nil
}

func (s *userServiceImpl) RefreshToken(ctx context.Context, refreshToken string) (string, error) {
	// 根据刷新令牌获取会话
	session, err := s.sessionRepo.GetByRefreshToken(ctx, refreshToken)
	if err != nil || session == nil {
		return "", ErrInvalidToken
	}

	// 检查会话是否过期
	if session.ExpiresAt.Before(time.Now()) {
		s.sessionRepo.Delete(ctx, session.ID)
		return "", ErrTokenExpired
	}

	// 获取用户信息
	user, err := s.userRepo.GetByID(ctx, session.UserID)
	if err != nil {
		return "", ErrUserNotFound
	}

	// 生成新令牌
	newToken, err := s.generateToken(user)
	if err != nil {
		return "", fmt.Errorf("生成新令牌失败: %w", err)
	}

	// 更新会话
	session.Token = s.hashToken(newToken)
	session.ExpiresAt = time.Now().Add(s.jwtExpiry)
	if err := s.sessionRepo.Update(ctx, session); err != nil {
		return "", fmt.Errorf("更新会话失败: %w", err)
	}

	return newToken, nil
}

// ==================== 用户管理 ====================

func (s *userServiceImpl) GetUserByID(ctx context.Context, userID string) (*models.User, error) {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("查询用户失败: %w", err)
	}

	// 隐藏敏感信息
	user.Password = ""
	return user, nil
}

func (s *userServiceImpl) GetUserByUsername(ctx context.Context, username string) (*models.User, error) {
	user, err := s.userRepo.GetByUsername(ctx, username)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("查询用户失败: %w", err)
	}

	user.Password = ""
	return user, nil
}

func (s *userServiceImpl) UpdateUser(ctx context.Context, userID string, updates map[string]interface{}) error {
	// 检查用户是否存在
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return ErrUserNotFound
	}

	// 不允许更新的字段
	restrictedFields := []string{"id", "password", "created_at"}
	for _, field := range restrictedFields {
		delete(updates, field)
	}

	// 如果更新用户名，检查是否重复
	if username, ok := updates["username"].(string); ok && username != user.Username {
		existing, _ := s.userRepo.GetByUsername(ctx, username)
		if existing != nil {
			return errors.New("用户名已存在")
		}
	}

	// 如果更新邮箱，检查是否重复
	if email, ok := updates["email"].(string); ok && email != user.Email {
		existing, _ := s.userRepo.GetByEmail(ctx, email)
		if existing != nil {
			return errors.New("邮箱已被使用")
		}
	}

	updates["updated_at"] = time.Now()

	if err := s.userRepo.UpdateFields(ctx, userID, updates); err != nil {
		return fmt.Errorf("更新用户失败: %w", err)
	}

	return nil
}

func (s *userServiceImpl) ChangePassword(ctx context.Context, userID, oldPassword, newPassword string) error {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return ErrUserNotFound
	}

	// 验证旧密码
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(oldPassword)); err != nil {
		return errors.New("旧密码不正确")
	}

	// 加密新密码
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("密码加密失败: %w", err)
	}

	updates := map[string]interface{}{
		"password":   string(hashedPassword),
		"updated_at": time.Now(),
	}

	if err := s.userRepo.UpdateFields(ctx, userID, updates); err != nil {
		return fmt.Errorf("更新密码失败: %w", err)
	}

	// 使该用户的所有会话失效
	if err := s.sessionRepo.DeleteByUserID(ctx, userID); err != nil {
		// 日志记录错误，但不影响密码修改
		fmt.Printf("删除用户会话失败: %v\n", err)
	}

	return nil
}

func (s *userServiceImpl) DeleteUser(ctx context.Context, userID string) error {
	// 删除用户的所有会话
	if err := s.sessionRepo.DeleteByUserID(ctx, userID); err != nil {
		return fmt.Errorf("删除用户会话失败: %w", err)
	}

	// 删除用户的好友关系
	if err := s.friendRepo.DeleteByUserID(ctx, userID); err != nil {
		return fmt.Errorf("删除好友关系失败: %w", err)
	}

	// 删除用户
	if err := s.userRepo.Delete(ctx, userID); err != nil {
		return fmt.Errorf("删除用户失败: %w", err)
	}

	return nil
}

// ==================== 在线状态管理 ====================

func (s *userServiceImpl) SetOnline(ctx context.Context, userID string) error {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return ErrUserNotFound
	}

	user.Status = "online"
	user.LastSeen = time.Now()

	if err := s.userRepo.Update(ctx, user); err != nil {
		return fmt.Errorf("设置在线状态失败: %w", err)
	}

	return nil
}

func (s *userServiceImpl) SetOffline(ctx context.Context, userID string) error {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return ErrUserNotFound
	}

	user.Status = "offline"
	user.LastSeen = time.Now()

	if err := s.userRepo.Update(ctx, user); err != nil {
		return fmt.Errorf("设置离线状态失败: %w", err)
	}

	return nil
}

func (s *userServiceImpl) SetStatus(ctx context.Context, userID, status string) error {
	// 验证状态值
	validStatuses := []string{"online", "offline", "busy", "away"}
	isValid := false
	for _, s := range validStatuses {
		if s == status {
			isValid = true
			break
		}
	}
	if !isValid {
		return errors.New("无效的状态值")
	}

	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return ErrUserNotFound
	}

	user.Status = status
	user.LastSeen = time.Now()

	if err := s.userRepo.Update(ctx, user); err != nil {
		return fmt.Errorf("设置状态失败: %w", err)
	}

	return nil
}

func (s *userServiceImpl) GetOnlineUsers(ctx context.Context) ([]*models.User, error) {
	users, err := s.userRepo.GetByStatus(ctx, "online")
	if err != nil {
		return nil, fmt.Errorf("获取在线用户失败: %w", err)
	}

	// 隐藏敏感信息
	for i := range users {
		users[i].Password = ""
	}

	return users, nil
}

func (s *userServiceImpl) GetUserStatus(ctx context.Context, userID string) (string, error) {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", ErrUserNotFound
		}
		return "", fmt.Errorf("获取用户状态失败: %w", err)
	}

	return user.Status, nil
}

// ==================== 好友管理 ====================

func (s *userServiceImpl) AddFriend(ctx context.Context, userID, friendID string) error {
	// 检查是否是自己
	if userID == friendID {
		return errors.New("不能添加自己为好友")
	}

	// 检查对方用户是否存在
	_, err := s.userRepo.GetByID(ctx, friendID)
	if err != nil {
		return ErrUserNotFound
	}

	// 检查是否已经是好友
	existing, err := s.friendRepo.GetFriendship(ctx, userID, friendID)
	if err == nil && existing != nil {
		if existing.Status == "accepted" {
			return errors.New("已经是好友")
		} else if existing.Status == "pending" {
			return errors.New("好友请求已发送，等待对方接受")
		} else if existing.Status == "blocked" {
			return errors.New("用户已被屏蔽")
		}
	}

	// 创建好友请求
	friendship := &models.Friend{
		ID:        uuid.New().String(),
		UserID:    userID,
		FriendID:  friendID,
		Status:    "pending",
		CreatedAt: time.Now(),
	}

	if err := s.friendRepo.Create(ctx, friendship); err != nil {
		return fmt.Errorf("发送好友请求失败: %w", err)
	}

	return nil
}

func (s *userServiceImpl) RemoveFriend(ctx context.Context, userID, friendID string) error {
	// 删除双向的好友关系
	if err := s.friendRepo.DeleteFriendship(ctx, userID, friendID); err != nil {
		return fmt.Errorf("删除好友失败: %w", err)
	}

	return nil
}

func (s *userServiceImpl) GetFriends(ctx context.Context, userID string) ([]*models.User, error) {
	// 获取所有接受的好友关系
	friendships, err := s.friendRepo.GetAcceptedFriends(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("获取好友列表失败: %w", err)
	}

	// 提取好友ID
	friendIDs := make([]string, 0, len(friendships))
	for _, f := range friendships {
		if f.UserID == userID {
			friendIDs = append(friendIDs, f.FriendID)
		} else {
			friendIDs = append(friendIDs, f.UserID)
		}
	}

	// 获取好友详细信息
	users, err := s.userRepo.GetByIDs(ctx, friendIDs)
	if err != nil {
		return nil, fmt.Errorf("获取好友信息失败: %w", err)
	}

	// 隐藏敏感信息
	for i := range users {
		users[i].Password = ""
	}

	return users, nil
}

func (s *userServiceImpl) GetFriendRequests(ctx context.Context, userID string) ([]*models.Friend, error) {
	// 获取所有待处理的好友请求（别人发给我的）
	requests, err := s.friendRepo.GetPendingRequests(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("获取好友请求失败: %w", err)
	}

	return requests, nil
}

func (s *userServiceImpl) AcceptFriendRequest(ctx context.Context, userID, requestID string) error {
	// 获取好友请求
	friendship, err := s.friendRepo.GetByID(ctx, requestID)
	if err != nil {
		return errors.New("好友请求不存在")
	}

	// 检查是否有权限接受
	if friendship.FriendID != userID {
		return errors.New("无权接受此请求")
	}

	// 检查状态
	if friendship.Status != "pending" {
		return errors.New("请求状态无效")
	}

	// 更新状态为已接受
	friendship.Status = "accepted"
	now := time.Now()
	friendship.AcceptedAt = &now

	if err := s.friendRepo.Update(ctx, friendship); err != nil {
		return fmt.Errorf("接受好友请求失败: %w", err)
	}

	return nil
}

func (s *userServiceImpl) BlockUser(ctx context.Context, userID, blockUserID string) error {
	// 检查是否是自己
	if userID == blockUserID {
		return errors.New("不能屏蔽自己")
	}

	// 检查对方用户是否存在
	_, err := s.userRepo.GetByID(ctx, blockUserID)
	if err != nil {
		return ErrUserNotFound
	}

	// 检查是否已存在关系
	existing, err := s.friendRepo.GetFriendship(ctx, userID, blockUserID)
	if err == nil && existing != nil {
		// 更新现有关系为屏蔽
		existing.Status = "blocked"
		if err := s.friendRepo.Update(ctx, existing); err != nil {
			return fmt.Errorf("屏蔽用户失败: %w", err)
		}
	} else {
		// 创建屏蔽关系
		friendship := &models.Friend{
			ID:        uuid.New().String(),
			UserID:    userID,
			FriendID:  blockUserID,
			Status:    "blocked",
			CreatedAt: time.Now(),
		}

		if err := s.friendRepo.Create(ctx, friendship); err != nil {
			return fmt.Errorf("屏蔽用户失败: %w", err)
		}
	}

	return nil
}

// ==================== 统计信息 ====================

func (s *userServiceImpl) GetUserStats(ctx context.Context, userID string) (*models.UserStats, error) {
	// 获取用户信息
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, ErrUserNotFound
	}

	// 计算在线时长（示例，实际需要从日志中计算）
	onlineHours := 0.0
	if user.Status == "online" {
		// 如果在线，计算从最后活动时间到现在的时间差
		onlineHours = time.Since(user.LastSeen).Hours()
	}

	stats := &models.UserStats{
		UserID:        userID,
		TotalMessages: 0, // 需要从消息表中统计
		OnlineHours:   onlineHours,
		LastActive:    user.LastSeen,
	}

	return stats, nil
}

func (s *userServiceImpl) RecordUserActivity(ctx context.Context, userID string) error {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return ErrUserNotFound
	}

	user.LastSeen = time.Now()

	if err := s.userRepo.Update(ctx, user); err != nil {
		return fmt.Errorf("记录用户活动失败: %w", err)
	}

	return nil
}

// ==================== 私有辅助方法 ====================

type jwtClaims struct {
	UserID   string `json:"user_id"`
	Username string `json:"username"`
	jwt.RegisteredClaims
}

func (s *userServiceImpl) generateToken(user *models.User) (string, error) {
	claims := jwtClaims{
		UserID:   user.ID,
		Username: user.Username,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(s.jwtExpiry)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Subject:   user.ID,
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(s.jwtSecret)
}

func (s *userServiceImpl) parseToken(tokenString string) (*jwtClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &jwtClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return s.jwtSecret, nil
	})

	if err != nil {
		return nil, err
	}

	if claims, ok := token.Claims.(*jwtClaims); ok && token.Valid {
		return claims, nil
	}

	return nil, ErrInvalidToken
}

func (s *userServiceImpl) hashToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}
