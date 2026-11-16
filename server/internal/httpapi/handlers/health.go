// Package handlers implements HTTP handlers for the HealthConnect API.
package handlers

import "github.com/gin-gonic/gin"

// Health reports that the process is running. It does not check the databases.
func Health(c *gin.Context) {
	c.JSON(200, gin.H{"status": "ok"})
}
