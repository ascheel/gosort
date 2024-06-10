package main

import (
	"net/http"
	"github.com/gin-gonic/gin"
	"github.com/ascheel/gosort/internal/media"
	//"github.com/ascheel/gosort/internal/mediadb"
)

func getImages(c *gin.Context) {
	images := []media.Media {
		{ Filename: "test1.jpg", Size: 1000000 },
		{ Filename: "test2.jpg", Size: 2000000 },
		{ Filename: "test3.jpg", Size: 3000000 },
	}
	c.IndentedJSON(http.StatusOK, images)
}

func main() {
	router := gin.Default()
	router.GET("/images", getImages)
	router.Run("localhost:8080")
}
