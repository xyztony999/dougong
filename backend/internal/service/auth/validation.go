package auth

import (
	"fmt"
	"regexp"

	"github.com/dougong/dougong/internal/errcode"
	storeauth "github.com/dougong/dougong/internal/store/auth"
)

// usernameRe 用户名字符集（I-10：非空、≤32、[a-zA-Z0-9_.-]）。
var usernameRe = regexp.MustCompile(`^[a-zA-Z0-9_.-]{1,32}$`)

// validateUsername 返回用户名校验错误（nil=通过）。
func validateUsername(v string) *errcode.FieldError {
	if v == "" {
		return &errcode.FieldError{Field: "username", Message: "用户名不能为空"}
	}
	if len(v) > UsernameMaxLength || !usernameRe.MatchString(v) {
		return &errcode.FieldError{Field: "username", Message: fmt.Sprintf("用户名须为 %d 字符以内的大小写字母、数字、_.- 组合", UsernameMaxLength)}
	}
	return nil
}

// validatePassword 返回密码校验错误（A10：≥8；bcrypt 上限 72 字节）。
func validatePassword(v string) *errcode.FieldError {
	if len(v) < PasswordMinLength {
		return &errcode.FieldError{Field: "password", Message: fmt.Sprintf("密码长度不能少于 %d 位", PasswordMinLength)}
	}
	if len(v) > PasswordMaxLength {
		return &errcode.FieldError{Field: "password", Message: fmt.Sprintf("密码长度不能超过 %d 字节", PasswordMaxLength)}
	}
	return nil
}

// validateRole 返回角色校验错误（panel_role 枚举）。
func validateRole(v string) *errcode.FieldError {
	if v != storeauth.RoleAdmin && v != storeauth.RoleReadonly {
		return &errcode.FieldError{Field: "role", Message: "角色必须是 admin 或 readonly"}
	}
	return nil
}
