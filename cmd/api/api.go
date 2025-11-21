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
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"

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

type Stats struct {
	Count int `json:"count"`
}

var engine *sortengine.Engine

var stats = Stats{Count: 0}

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

func giveVersion(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"version": Version})
}

func pushFile(c *gin.Context) {
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
		fmt.Printf("Checksum exists: %s\n", media.Checksum)
		c.JSON(409, gin.H{"status": "exists"})
		return
	}

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

	// Recalculate checksum from the saved file to verify integrity
	// This prevents false duplicates from partial uploads or corrupted data
	actualChecksum, err := sortengine.Checksum(newFilename, false)
	if err != nil {
		err2 := os.Remove(newFilename)
		if err2 != nil {
			fmt.Printf("Error removing file: %s\n", err2.Error())
		}
		fmt.Printf("Error calculating checksum for saved file: %s\n", err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"status": "failed", "reason": "failed to verify file integrity"})
		return
	}

	// Verify the checksum matches what the client sent
	if actualChecksum != media.Checksum {
		err2 := os.Remove(newFilename)
		if err2 != nil {
			fmt.Printf("Error removing file: %s\n", err2.Error())
		}
		fmt.Printf("Checksum mismatch: client sent %s, but file has %s\n", media.Checksum, actualChecksum)
		c.JSON(http.StatusBadRequest, gin.H{"status": "failed", "reason": "checksum mismatch - file may be corrupted"})
		return
	}

	// Recalculate checksum100k from the saved file
	actualChecksum100k, err := sortengine.Checksum(newFilename, true)
	if err != nil {
		err2 := os.Remove(newFilename)
		if err2 != nil {
			fmt.Printf("Error removing file: %s\n", err2.Error())
		}
		fmt.Printf("Error calculating checksum100k for saved file: %s\n", err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"status": "failed", "reason": "failed to verify file integrity"})
		return
	}
	media.Checksum100k = actualChecksum100k

	// Double-check the checksum doesn't exist (race condition protection)
	if engine.DB.ChecksumExists(actualChecksum) {
		err2 := os.Remove(newFilename)
		if err2 != nil {
			fmt.Printf("Error removing file: %s\n", err2.Error())
		}
		fmt.Printf("Checksum exists (race condition detected): %s\n", actualChecksum)
		c.JSON(409, gin.H{"status": "exists"})
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

	shortFilename := filepath.Base(data.Filename)
	stats.Count += 1
	fmt.Printf("(%03d) Uploaded file: %s\n", stats.Count, shortFilename)
}

func checksumExists(checksum string) bool {
	// db := NewDB("./gosort.db")	// Clean this up to make it secure if necessary
	return engine.DB.ChecksumExists(checksum)
}

func checksum100kExists(checksum string) bool {
	// db := NewDB("./gosort.db")	// Clean this up to make it secure if necessary
	return engine.DB.Checksum100kExists(checksum)
}

func checkFile(c *gin.Context) {
	status := "not found"
	if checksumExists(c.PostForm("checksum")) {
		status = "exists"
	}
	c.IndentedJSON(http.StatusOK, Status{Status: status})
}

func checkChecksums(c *gin.Context) {
	//fmt.Printf("Request: %+v\n", c.Request)

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

func checkChecksum100k(c *gin.Context) {
	//fmt.Printf("Request: %+v\n", c.Request)

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
		results[md5sum] = checksum100kExists(md5sum)
	}

	c.JSON(http.StatusOK, gin.H{"results": results})
}

func printVersion() {
	fmt.Printf("GoSort API Version: %s\n", Version)
}

func checkSaveDir() {
	if _, err := os.Stat(engine.Config.Server.SaveDir); os.IsNotExist(err) {
		fmt.Printf("Save directory does not exist: %s\n", engine.Config.Server.SaveDir)
		os.Exit(1)
	}
}

func main() {
	printVersion()

	// Parse command-line flags
	flags := &sortengine.ConfigFlags{}
	flag.StringVar(&flags.ConfigFile, "config", "", "Path to config file (default: ~/.gosort.yml)")
	flag.StringVar(&flags.DBFile, "database-file", "", "Database file path (overrides config)")
	flag.StringVar(&flags.SaveDir, "savedir", "", "Directory to save files (overrides config)")
	flag.StringVar(&flags.IP, "ip", "", "IP address to bind to (overrides config)")
	flag.IntVar(&flags.Port, "port", 0, "Port to listen on (overrides config)")
	flag.BoolVar(&flags.InitConfig, "init", false, "Create default config file and exit")
	flag.Parse()

	// Handle -init flag
	if flags.InitConfig {
		configPath := flags.ConfigFile
		if configPath == "" {
			var err error
			configPath, err = sortengine.GetDefaultConfigPath()
			if err != nil {
				fmt.Printf("Error getting default config path: %s\n", err.Error())
				os.Exit(1)
			}
		}
		if err := sortengine.CreateDefaultConfig(configPath); err != nil {
			fmt.Printf("Error creating config file: %s\n", err.Error())
			os.Exit(1)
		}
		os.Exit(0)
	}

	// Load config
	configPath := flags.ConfigFile
	if configPath == "" {
		var err error
		configPath, err = sortengine.GetDefaultConfigPath()
		if err != nil {
			fmt.Printf("Error getting default config path: %s\n", err.Error())
			os.Exit(1)
		}
	}

	config, err := sortengine.LoadConfig(configPath)
	if err != nil {
		fmt.Printf("Error loading config: %s\n", err.Error())
		fmt.Printf("Use -init to create a default config file\n")
		os.Exit(1)
	}

	// Apply command-line flags to override config values
	config.ApplyFlags(flags)

	// Create engine with the config
	engine = sortengine.NewEngineWithConfig(config)

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	go func() {
		for sig := range c {
			fmt.Printf("Received SIGINT: %v\n", sig)
			sortengine.GetExiftool().Close()
			os.Exit(1)
		}
	}()

	ip := engine.Config.Server.IP
	port := engine.Config.Server.Port
	checkSaveDir()
	router := gin.Default()
	//router.Use(logRequestMiddleware)
	router.POST("/file", pushFile)
	router.GET("/file", checkFile)
	router.POST("/checksums", checkChecksums)
	router.POST("/checksum100k", checkChecksum100k)
	router.GET("/version", giveVersion)
	router.Run(fmt.Sprintf("%s:%d", ip, port))
}
