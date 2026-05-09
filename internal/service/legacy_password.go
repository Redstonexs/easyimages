package service

import (
	"crypto/sha256"
	"encoding/hex"
)

// legacy_password.go
// 此文件包含旧格式密码验证逻辑，仅用于向后兼容PHP版本
// 新密码应使用bcrypt进行哈希（见service.go中的HashPassword函数）

// LegacyPasswordHasher 旧格式密码哈希器（仅用于向后兼容）
type LegacyPasswordHasher struct{}

// NewLegacyPasswordHasher 创建旧格式密码哈希器
func NewLegacyPasswordHasher() *LegacyPasswordHasher {
	return &LegacyPasswordHasher{}
}

// VerifyHash 验证旧格式的SHA256哈希密码
// 注意：此方法仅用于兼容从PHP版本迁移的密码
// 新密码应使用bcrypt进行验证
func (h *LegacyPasswordHasher) VerifyHash(password, hashedPassword string) bool {
	// 计算SHA256哈希（用于兼容PHP版本的密码格式）
	passwordHash := sha256.Sum256([]byte(password))
	expectedHash := hex.EncodeToString(passwordHash[:])

	// 使用常量时间比较防止时序攻击
	return subtleCompare(expectedHash, hashedPassword)
}

// GetHash 获取旧格式的SHA256哈希值（仅用于迁移）
func (h *LegacyPasswordHasher) GetHash(password string) string {
	passwordHash := sha256.Sum256([]byte(password))
	return hex.EncodeToString(passwordHash[:])
}

// subtleCompare 使用常量时间比较两个字符串，防止时序攻击
func subtleCompare(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	result := 0
	for i := 0; i < len(a); i++ {
		result |= int(a[i] ^ b[i])
	}
	return result == 0
}
