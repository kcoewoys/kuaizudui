package domain

import (
	"crypto/rand"
	"fmt"
)

const (
	inviteCodeLength   = 8
	inviteCodeAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
)

// GenerateInviteCode 生成 8 位大写邀请码。字母表去掉易混淆的 0/O/1/I 后
// 恰为 32 个字符，rand 字节取模映射无偏；32^8 的空间对现有用户量足够。
func GenerateInviteCode() (string, error) {
	bytes := make([]byte, inviteCodeLength)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("generate invite code: %w", err)
	}
	code := make([]byte, inviteCodeLength)
	for index, value := range bytes {
		code[index] = inviteCodeAlphabet[int(value)%len(inviteCodeAlphabet)]
	}
	return string(code), nil
}
