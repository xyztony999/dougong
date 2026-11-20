package appmgmt

import (
	"errors"
	"reflect"
	"strconv"

	"github.com/dougong/dougong/internal/api/middleware"
	"github.com/dougong/dougong/internal/api/response"
	"github.com/dougong/dougong/internal/errcode"
	authsvc "github.com/dougong/dougong/internal/service/auth"
	"github.com/gin-gonic/gin"
)

// identity 取认证身份（AuthRequired 链路下必有；防御双保险）。
func identity(c *gin.Context) *authsvc.Identity {
	id, ok := middleware.Identity(c)
	if !ok {
		return nil
	}
	return id
}

// bindError 绑定错误 → AUTH_012（env-mgmt 同法）。
func bindError(err error) *errcode.Error {
	return errcode.ErrValidation.WithFields(errcode.FieldError{
		Field: "body", Message: err.Error(),
	})
}

// queryInt query 整数参数（缺省/非法取 def）。
func queryInt(c *gin.Context, key string, def int) int {
	v := c.Query(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return def
	}
	return n
}

// respond 统一收尾（env-mgmt 同法：错误自带 data 载荷保留；typed-nil 不误判）。
func respond(c *gin.Context, data any, err error) {
	if err != nil {
		if hasValue(data) {
			var ec *errcode.Error
			if errors.As(err, &ec) {
				c.JSON(200, response.Envelope{Code: ec.Symbol(), Message: ec.Message(), Data: data})
				return
			}
		}
		response.Fail(c, err)
		return
	}
	response.OK(c, data)
}

func hasValue(data any) bool {
	if data == nil {
		return false
	}
	rv := reflect.ValueOf(data)
	switch rv.Kind() {
	case reflect.Pointer, reflect.Map, reflect.Slice, reflect.Interface:
		return !rv.IsNil()
	}
	return true
}
