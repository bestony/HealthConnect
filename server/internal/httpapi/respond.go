package httpapi

import "github.com/gin-gonic/gin"

func writeError(c *gin.Context, status int, code string) {
	c.AbortWithStatusJSON(status, gin.H{"error": code})
}
