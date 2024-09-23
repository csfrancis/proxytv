//go:build release

package proxytv

import "github.com/gin-gonic/gin"

func SetGinMode() {
	gin.SetMode(gin.ReleaseMode)
}
