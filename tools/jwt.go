package tools

import (
	"errors"
	"fmt"
	"github.com/golang-jwt/jwt/v5"
	"strings"
	"tgbot/config"
	"time"
)

// 角色标识：写入 token 载荷并在校验时比对，前后台共用同一套签发/解析逻辑
const (
	RoleAdmin = "admin" // 管理后台
	RoleUser  = "user"  // H5 用户端
)

// Claims 通用 JWT 载荷，前后台共用；嵌入标准字段以携带过期时间等
type Claims struct {
	UserID int    `json:"user_id"` // admin 表 id 或 user 表 id
	Role   string `json:"role"`    // RoleAdmin / RoleUser
	jwt.RegisteredClaims
}

// jwtSecretKey 读取签名密钥（来自 config.yaml 的 jwt.secretKey）
func jwtSecretKey() []byte {
	key := config.Conf.GetString("jwt.secretKey")
	if key == "" {
		key = "b34253853b8466c73d232c2f716c5c0e417b9f79a44a3ae6ad695914f25d87f"
	}
	return []byte(key)
}

// jwtExpireHours 读取 token 有效期（小时），默认 12 小时
func jwtExpireHours() int {
	if h := config.Conf.GetInt("jwt.expireHours"); h > 0 {
		return h
	}
	return 12
}

// GenerateToken 生成一个 HS256 签名的通用 JWT，前后台共用
func GenerateToken(userID int, role string) (string, error) {
	now := time.Now()
	claims := Claims{
		UserID: userID,
		Role:   role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Duration(jwtExpireHours()) * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(now),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(jwtSecretKey())
}

// ParseToken 解析并校验 JWT，返回其中的载荷
func ParseToken(tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		// 校验签名算法，防止算法混淆攻击
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return jwtSecretKey(), nil
	})
	if err != nil {
		return nil, err
	}
	if claims, ok := token.Claims.(*Claims); ok && token.Valid {
		return claims, nil
	}
	return nil, errors.New("invalid token")
}

// ParseAuthorization 从 Authorization 头解析 JWT，兼容 "Bearer <token>" 与裸 "<token>" 两种写法
func ParseAuthorization(header string) (*Claims, error) {
	tokenStr := strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
	return ParseToken(tokenStr)
}
