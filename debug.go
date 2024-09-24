//go:build debug

package proxytv

import "github.com/gin-gonic/gin"

func SetGinMode() {
	gin.SetMode(gin.DebugMode)
}
