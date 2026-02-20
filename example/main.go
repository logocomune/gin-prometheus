package main

import (
	"github.com/gin-gonic/gin"
	ginprom "github.com/logocomune/gin-prometheus"
	"log/slog"
)

func main() {
	r := gin.Default()

	r.Use(ginprom.Middleware())

	r.GET("/metrics", gin.WrapH(ginprom.GetMetricHandler(ginprom.WithBasicAuth("atest", "test"))))

	r.GET("/", func(c *gin.Context) {
		c.String(200, "Hello World")
	})
	r.GET("/test/:id", func(c *gin.Context) {
		c.String(200, "Hello World")
	})
	r.GET("/ping", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"message": "pong",
		})
	})
	slog.Info("start server http://localhost:8080")
	r.Run() // listen and serve on 0.0.0.0:8080
}
