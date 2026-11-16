package errcode

import (
	"errors"
	"fmt"
	"testing"
)

// authCodes 汇总 AUTH_001~014（与 auth.go 定义同步断言；Gate 2 E1 修复后
// 契约通道为符号串，不再有数值码）。
var authCodes = []*Error{
	ErrUnauthenticated, ErrSessionExpired, ErrCSRF, ErrAccountLocked,
	ErrBadCredentials, ErrTotpInvalid, ErrTotpRequired, ErrForbidden,
	ErrAlreadyInitialized, ErrLastAdmin, ErrUsernameTaken, ErrValidation,
	ErrTicketInvalid, ErrSelfDelete,
}

func TestAuthCodeRegistry(t *testing.T) {
	wantSymbols := []string{
		"AUTH_001", "AUTH_002", "AUTH_003", "AUTH_004", "AUTH_005", "AUTH_006", "AUTH_007",
		"AUTH_008", "AUTH_009", "AUTH_010", "AUTH_011", "AUTH_012", "AUTH_013", "AUTH_014",
	}
	seen := map[string]bool{}
	for i, ec := range authCodes {
		if ec.Symbol() != wantSymbols[i] {
			t.Fatalf("authCodes[%d].symbol = %s, want %s", i, ec.Symbol(), wantSymbols[i])
		}
		if seen[ec.Symbol()] {
			t.Fatalf("duplicate symbol %s", ec.Symbol())
		}
		seen[ec.Symbol()] = true
		if ec.Message() == "" {
			t.Fatalf("%s empty message", ec.Symbol())
		}
	}
	// 公共码不与 AUTH 段冲突。
	for _, ec := range []*Error{ErrNotFound, ErrInternal} {
		if seen[ec.Symbol()] {
			t.Fatalf("public code %s collides with AUTH segment", ec.Symbol())
		}
	}
}

func TestErrorWrappingAndClone(t *testing.T) {
	base := ErrAccountLocked.WithData(map[string]int{"remainingSeconds": 42})
	// WithData 返回副本：sentinel 本体不被污染。
	if ErrAccountLocked.Data() != nil {
		t.Fatal("sentinel mutated by WithData")
	}
	wrapped := fmt.Errorf("login: %w", base)
	var ec *Error
	if !errors.As(wrapped, &ec) {
		t.Fatal("errors.As cannot unwrap errcode")
	}
	if ec.Symbol() != "AUTH_004" {
		t.Fatalf("unwrapped symbol = %s, want AUTH_004", ec.Symbol())
	}
	if ec.Error() != "AUTH_004: "+ErrAccountLocked.Message() {
		t.Fatalf("Error() = %q", ec.Error())
	}
	// WithFields 生成 R-14 逐字段载荷。
	ve := ErrValidation.WithFields(FieldError{Field: "username", Message: "必填"})
	fields, ok := ve.Data().(ValidationData)
	if !ok || len(fields.Fields) != 1 || fields.Fields[0].Field != "username" {
		t.Fatal("WithFields payload broken")
	}
}
