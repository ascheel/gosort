package main

// The purpose of this application is to accept API requests.
// These requests are intended to supply image data to be stored
// on the server.  These images will be renamed by date and their
// checksums stored in a database.  The goal is to feed it all images
// and let it sort them while discarding duplicates.

// Necessary functions:
// GET /status - return a status of the API, including number of images
// GET /images - return a list of images
// POST /images - accept an image and store it
// POST /images/checksum - accept a checksum and return whether it exists

import (
	//"encoding/json"
	"fmt"
	"net/http"
	"os"
	//"path"
	"path/filepath"
	//"time"

	"github.com/ascheel/gosort/internal/sortengine"
	// m "github.com/ascheel/gosort/internal/media"
	// "github.com/ascheel/gosort/internal/config"
	//"github.com/ascheel/gosort/internal/mediadb"
	"github.com/gin-gonic/gin"
)

type GoSort struct  {
	Settings *sortengine.Config
}

func InitGoSort() *GoSort {
	var err error
	gs := &GoSort{}
	gs.Settings, err = sortengine.LoadConfig()
	if err != nil {
		fmt.Printf("Error loading config: %s\n", err.Error())
		os.Exit(1)
	}
	fmt.Printf("Settings: %+v\n", gs.Settings)
	return gs
}

// type Media struct {
// 	Filename     string            `yaml:"filename"      json:"filename"`
// 	Path         string            `yaml:"path"          json:"path"`
// 	Checksum     string            `yaml:"checksum"      json:"checksum"`
// 	Checksum100k string            `yaml:"checksum100k"  json:"checksum100k"`
// 	Size         int64             `yaml:"size"          json:"size"`
// 	ModifiedDate time.Time         `yaml:"modified_time" json:"modified_time"`
// 	CreationDate time.Time         `yaml:"creation_time" json:"creation_time"`
// 	Metadata     map[string]string `yaml:"metadata"      json:"metadata"`
// }

type Status struct {
	Status string	`json:"status"`
}

var images = []sortengine.Media{
	{Filename: "test1.jpg", Path: "/dir1", Size:1000000},
	{Filename: "test2.jpg", Path: "/dir1", Size:2000000},
}

func getStatus200(c *gin.Context) {
	c.IndentedJSON(http.StatusOK, images)
}

func pushFile(c *gin.Context) {
	// Must bring in the following data:
	// Binary data named "file"
	// Media struct (populated) named "media"
	data, err := c.FormFile("file")
	if err != nil {
		c.String(http.StatusBadRequest, fmt.Sprintf("get form err: %s", err.Error()))
		return
	}

	file := c.PostForm("media")
	media := sortengine.MediaFromJSON(file)
	db := NewDB("./gosort.db")

	// Check if checksum exists
	if db.ChecksumExists(media.Checksum) {
		c.JSON(409, gin.H{"status": "exists"})
		return
	}

	newFilename := db.GetNewFilename(media)

	shortFilename := filepath.Base(data.Filename)
	fmt.Printf("Uploaded file: %s\n", shortFilename)

	// Create temp file for saving
	// But first, you need to get a temporary file name
	// This is to prevent incomplete files from being saved

	if err := c.SaveUploadedFile(data, newFilename); err != nil {
		c.String(http.StatusInternalServerError, fmt.Sprintf("Upload file err: %s\n", err.Error()))
		return
	}

	// On success, move file to true destination

	c.String(http.StatusOK, fmt.Sprintf("File %s uploaded successfully", shortFilename))
}

func checksumExists(checksum string) bool {
	db := NewDB("./gosort.db")	// Clean this up to make it secure if necessary
	return db.ChecksumExists(checksum)
}

func checkFile(c *gin.Context) {
	status := "not found"
	if checksumExists(c.PostForm("checksum")) {
		status = "exists"
	}
	c.IndentedJSON(http.StatusOK, Status{Status: status})
}

func checkChecksums(c *gin.Context) {
	status := "not found"
	checksums := make([]string, 0)
	db := NewDB("./gosort.db")
	for _, checksum := range c.PostFormArray("checksumList") {
		if ! db.ChecksumExists(checksum) {
			checksums = append(checksums, checksum)
		}
	}
	//c.IndentedJSON(http.StatusOK, Status{Status: status})
	c.JSON(http.StatusOK, gin.H{"status": status, "checksums": checksums})
}

func main() {
	gs := InitGoSort()
	fmt.Printf("SaveDir: %s\n", gs.Settings.Server.SaveDir)
	fmt.Printf("Port: %d\n", gs.Settings.Server.Port)
	router := gin.Default()
	router.GET("/status", getStatus200)
	router.POST("/file", pushFile)
	router.GET("/file", checkFile)
	router.POST("/checksums", checkChecksums)
	router.Run("localhost:8080")
}
