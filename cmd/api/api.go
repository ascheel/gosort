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
	"github.com/gin-gonic/gin"
)

var (
	Version string
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
	engine := sortengine.NewEngine()

	// Check if checksum exists
	if engine.DB.ChecksumExists(media.Checksum) {
		c.JSON(409, gin.H{"status": "exists"})
		return
	}

	newFilename := engine.GetNewFilename(media)

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
	engine := sortengine.NewEngine()
	// db := NewDB("./gosort.db")	// Clean this up to make it secure if necessary
	return engine.DB.ChecksumExists(checksum)
}

func checkFile(c *gin.Context) {
	status := "not found"
	if checksumExists(c.PostForm("checksum")) {
		status = "exists"
	}
	c.IndentedJSON(http.StatusOK, Status{Status: status})
}

func checkChecksums(c *gin.Context) {
	checksums := make(map[string]bool)
	engine := sortengine.NewEngine()
	for _, md5sum := range c.PostFormArray("checksumList") {
		checksums[md5sum] = engine.DB.ChecksumExists(md5sum)
	}
	//c.IndentedJSON(http.StatusOK, Status{Status: status})
	c.JSON(http.StatusOK, gin.H{"checksums": checksums})
}

func printVersion() {
	fmt.Printf("GoSort API Version: %s\n", Version)
}

func main() {
	printVersion()
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
