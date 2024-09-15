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
	"encoding/json"
	"bytes"
	"fmt"
	"io"
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

func logRequestMiddleware(c *gin.Context) {
	bodyBytes, err := io.ReadAll(c.Request.Body)
	if err != nil {
		fmt.Printf("Error reading body: %s\n", err)
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

	if len(bodyBytes) < 1000 {
		fmt.Printf("\nRequest Body: %s\n", string(bodyBytes))
	} else {
		fmt.Printf("\nRequest Body: %s\n", string(bodyBytes[:256]))
	}
	//fmt.Printf("Request Body: %s\n", string(bodyBytes))
	fmt.Printf("Request Method: %s\n", c.Request.Method)
	fmt.Printf("Request URL: %s\n", c.Request.URL)
	fmt.Printf("Request Headers: %v\n\n", c.Request.Header)
}

func pushFile(c *gin.Context) {
	engine := sortengine.NewEngine()
	// Must bring in the following data:
	// Binary data named "file"
	// Media struct (populated) named "media"

	form, err := c.MultipartForm()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "failed", "reason": err.Error()})
		return
	}

	var media sortengine.Media

	mediaString := form.Value["media"][0]

	err	= json.Unmarshal([]byte(mediaString), &media)
	if err != nil {
		fmt.Printf("Error unmarshalling JSON: %s\n", err.Error())
		c.JSON(http.StatusBadRequest, gin.H{"status": "failed", "reason": err.Error()})
		return
	}

	data, err := c.FormFile("file")
	if err != nil {
		fmt.Printf("Error getting form file: %s\n", err.Error())
		c.JSON(http.StatusBadRequest, gin.H{"status": "failed", "reason": err.Error()})
		return
	}

	// Check if checksum exists
	if engine.DB.ChecksumExists(media.Checksum) {
		c.JSON(409, gin.H{"status": "exists"})
		return
	}

	shortFilename := filepath.Base(data.Filename)
	fmt.Printf("Uploaded file: %s\n", shortFilename)

	newFilename := engine.GetNewFilename(&media)
	tmpFilename := fmt.Sprintf("%s.download", newFilename)

	// Create temp file for saving
	// But first, you need to get a temporary file name
	// This is to prevent incomplete files from being saved

	if err := c.SaveUploadedFile(data, tmpFilename); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "failed", "reason": err.Error()})
		fmt.Printf("Error saving file: %s\n", err.Error())
		return
	}

	// On success, move file to true destination
	if err := os.Rename(tmpFilename, newFilename); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "failed", "reason": err.Error()})
		fmt.Printf("Error renaming file: %s\n", err.Error())
		return
	}
	err = engine.DB.AddFileToDB(&media)
	if err != nil {
		err2 := os.Remove(newFilename)
		if err2 != nil {
			fmt.Printf("Error removing file: %s\n", err.Error())
		}
		fmt.Printf("Error adding file to DB: %s\n", err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"status": "failed", "reason": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success"})
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
	fmt.Printf("Request: %+v\n", c.Request)

	form, err := c.MultipartForm()
	if err != nil {
		fmt.Printf("Error getting form: %s\n", err.Error())
		c.String(http.StatusBadRequest, fmt.Sprintf("get multipart form err: %s", err.Error()))
		return
	}

	var results = make(map[string]bool)
	var checksumData map[string][]string

	err = json.Unmarshal([]byte(form.Value["checksums"][0]), &checksumData)
	if err != nil {
		fmt.Printf("Error unmarshalling JSON: %s\n", err.Error())
		c.String(http.StatusBadRequest, fmt.Sprintf("Error unmarshalling JSON: %s", err.Error()))
		return
	}
	checksumList := checksumData["checksums"]
	for _, md5sum := range checksumList {
		results[md5sum] = checksumExists(md5sum)
	}

	c.JSON(http.StatusOK, gin.H{"results": results})
}

func printVersion() {
	fmt.Printf("GoSort API Version: %s\n", Version)
}

func checkSaveDir() {
	engine := sortengine.NewEngine()
	if _, err := os.Stat(engine.Config.Server.SaveDir); os.IsNotExist(err) {
		fmt.Printf("Save directory does not exist: %s\n", engine.Config.Server.SaveDir)
		os.Exit(1)
	}
}

func main() {
	printVersion()
	checkSaveDir()
	router := gin.Default()
	router.Use(logRequestMiddleware)
	router.GET("/status", getStatus200)
	router.POST("/file", pushFile)
	router.GET("/file", checkFile)
	router.POST("/checksums", checkChecksums)
	router.Run("localhost:8080")
}
