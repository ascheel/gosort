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
	"context"
	"encoding/json"
	"bytes"
	"crypto/md5"
	"flag"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"time"

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
var uploadQueue *UploadQueue
var batchInsertBuffer *BatchInsertBuffer

// BatchInsertBuffer collects files for batch database insertion
type BatchInsertBuffer struct {
	buffer    []*sortengine.Media
	batchSize int
	mu        sync.Mutex
}

// NewBatchInsertBuffer creates a new batch insert buffer
func NewBatchInsertBuffer(batchSize int) *BatchInsertBuffer {
	return &BatchInsertBuffer{
		buffer:    make([]*sortengine.Media, 0, batchSize),
		batchSize: batchSize,
	}
}

// Add adds a media file to the buffer and flushes if buffer is full
func (b *BatchInsertBuffer) Add(media *sortengine.Media) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	
	b.buffer = append(b.buffer, media)
	
	// Flush if buffer is full
	if len(b.buffer) >= b.batchSize {
		return b.flush()
	}
	
	return nil
}

// Flush inserts all buffered files into the database
func (b *BatchInsertBuffer) Flush() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.flush()
}

// flush performs the actual batch insert (must be called with lock held)
func (b *BatchInsertBuffer) flush() error {
	if len(b.buffer) == 0 {
		return nil
	}
	
	// Create a copy of the buffer for insertion
	batch := make([]*sortengine.Media, len(b.buffer))
	copy(batch, b.buffer)
	
	// Clear the buffer
	b.buffer = b.buffer[:0]
	
	// Release lock before database operation
	b.mu.Unlock()
	err := engine.DB.AddFilesToDBBatch(batch, b.batchSize)
	b.mu.Lock()
	
	return err
}

// UploadRequest represents a file upload request in the queue
type UploadRequest struct {
	Context      *gin.Context
	Media        sortengine.Media
	FileData     *multipart.FileHeader
	ResponseChan chan bool // Channel to signal when processing is complete
}

// RateLimiter implements a token bucket rate limiter
// This controls how many requests can be processed per second
type RateLimiter struct {
	tokens       chan struct{}
	refillTicker *time.Ticker
	rate         int // Requests per second
	capacity     int // Maximum tokens (burst capacity)
}

// NewRateLimiter creates a new rate limiter
// rate: requests per second allowed
// capacity: maximum burst capacity (how many can be queued)
func NewRateLimiter(rate int, capacity int) *RateLimiter {
	rl := &RateLimiter{
		tokens:   make(chan struct{}, capacity),
		rate:     rate,
		capacity: capacity,
	}
	
	// Fill initial tokens
	for i := 0; i < capacity; i++ {
		rl.tokens <- struct{}{}
	}
	
	// Start refill ticker
	refillInterval := time.Second / time.Duration(rate)
	rl.refillTicker = time.NewTicker(refillInterval)
	
	go rl.refill()
	
	return rl
}

// refill periodically adds tokens to the bucket
func (rl *RateLimiter) refill() {
	for range rl.refillTicker.C {
		select {
		case rl.tokens <- struct{}{}:
			// Token added
		default:
			// Bucket is full, skip
		}
	}
}

// Allow checks if a request is allowed (has token available)
func (rl *RateLimiter) Allow() bool {
	select {
	case <-rl.tokens:
		return true
	default:
		return false
	}
}

// UploadQueue manages a queue of upload requests with worker pool
type UploadQueue struct {
	queue      chan UploadRequest
	workers    int
	wg         sync.WaitGroup
	rateLimiter *RateLimiter
}

// NewUploadQueue creates a new upload queue with worker pool
func NewUploadQueue(workers int, rateLimit int) *UploadQueue {
	uq := &UploadQueue{
		queue:       make(chan UploadRequest, workers*2), // Buffered queue
		workers:     workers,
		rateLimiter: NewRateLimiter(rateLimit, rateLimit*2), // Allow burst of 2x rate
	}
	
	// Start worker pool
	for i := 0; i < workers; i++ {
		uq.wg.Add(1)
		go uq.worker(i)
	}
	
	return uq
}

// worker processes upload requests from the queue
func (uq *UploadQueue) worker(id int) {
	defer uq.wg.Done()
	
	for req := range uq.queue {
		// Apply rate limiting - wait for token if needed
		// This controls throughput (requests per second)
		if !uq.rateLimiter.Allow() {
			// Rate limit exceeded, return 429 Too Many Requests
			req.Context.JSON(http.StatusTooManyRequests, gin.H{
				"status": "rate_limited",
				"reason": "Too many requests, please try again later",
			})
			// Signal that processing is complete
			if req.ResponseChan != nil {
				req.ResponseChan <- false
			}
			continue
		}
		
		// Process the upload
		processUploadRequest(req)
		
		// Signal that processing is complete
		if req.ResponseChan != nil {
			req.ResponseChan <- true
		}
	}
}

// Enqueue adds an upload request to the queue
// Returns true if enqueued, false if queue is full
// If blocking is true, waits for space in queue (up to timeout)
func (uq *UploadQueue) Enqueue(req UploadRequest, blocking bool, timeout time.Duration) bool {
	if blocking {
		// Block until there's space in the queue or timeout
		select {
		case uq.queue <- req:
			return true
		case <-time.After(timeout):
			return false // Timeout
		}
	} else {
		// Non-blocking: return immediately if queue is full
		select {
		case uq.queue <- req:
			return true
		default:
			return false // Queue is full
		}
	}
}

// Shutdown gracefully shuts down the queue
func (uq *UploadQueue) Shutdown() {
	close(uq.queue)
	uq.wg.Wait()
	uq.rateLimiter.refillTicker.Stop()
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

func giveVersion(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"version": Version})
}

// pushFile handles incoming file upload requests
// It validates the request and enqueues it for processing by the worker pool
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

	err = json.Unmarshal([]byte(mediaString), &media)
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

	// Quick check if checksum exists (before queuing)
	// This prevents unnecessary queueing of duplicate files
	if engine.DB.ChecksumExists(media.Checksum) {
		fmt.Printf("Checksum exists: %s\n", media.Checksum)
		c.JSON(409, gin.H{"status": "exists"})
		return
	}

	// Enqueue the request for processing by worker pool
	// We use blocking enqueue with timeout so the handler waits for a worker
	// This provides concurrency control while still allowing the response to be sent
	responseChan := make(chan bool, 1)
	req := UploadRequest{
		Context:      c,
		Media:        media,
		FileData:     data,
		ResponseChan: responseChan,
	}

	// Try to enqueue the request (blocking with 30 second timeout)
	// This allows the handler to wait for a worker while still providing
	// concurrency control and rate limiting
	if !uploadQueue.Enqueue(req, true, 30*time.Second) {
		// Queue timeout - return 503 Service Unavailable
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"status": "queue_full",
			"reason": "Server is busy, please try again later",
		})
		return
	}

	// Wait for worker to finish processing
	// The response will be sent by processUploadRequest()
	// We wait here to ensure the HTTP connection stays open
	select {
	case <-responseChan:
		// Processing complete, response already sent
		return
	case <-time.After(5 * time.Minute):
		// Timeout waiting for response (shouldn't happen, but safety check)
		c.JSON(http.StatusRequestTimeout, gin.H{
			"status": "timeout",
			"reason": "Request processing timed out",
		})
		return
	}
}

// processUploadRequest processes a file upload request
// This is called by worker goroutines from the upload queue
func processUploadRequest(req UploadRequest) {
	c := req.Context
	media := req.Media
	data := req.FileData

	newFilename := engine.GetNewFilename(&media)
	tmpFilename := fmt.Sprintf("%s.download", newFilename)

	// Create temp file for saving
	// This prevents incomplete files from being saved

	// Open the uploaded file
	src, err := data.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "failed", "reason": err.Error()})
		fmt.Printf("Error opening uploaded file: %s\n", err.Error())
		return
	}
	defer src.Close()

	// Create the destination file
	dst, err := os.Create(tmpFilename)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "failed", "reason": err.Error()})
		fmt.Printf("Error creating temp file: %s\n", err.Error())
		return
	}
	defer dst.Close()

	// Create hash functions for checksum calculation during file save
	// This allows us to calculate checksums while saving, avoiding a second file read
	fullHash := md5.New()
	hash100k := md5.New()

	// Create a custom reader that feeds data to both hashes during the first 100KB
	// After 100KB, only feed to fullHash
	var BUFSIZE int64 = 102400
	var bytesRead int64 = 0
	
	buf := make([]byte, 32*1024) // 32KB buffer for efficient copying
	
	for {
		nr, er := src.Read(buf)
		if nr > 0 {
			bytesRead += int64(nr)
			
			// Write to file
			nw, ew := dst.Write(buf[0:nr])
			if nw < 0 || nr < nw {
				nw = 0
				if ew == nil {
					ew = fmt.Errorf("invalid write result")
				}
			}
			if ew != nil {
				dst.Close()
				safeRemoveFile(tmpFilename, 3)
				c.JSON(http.StatusInternalServerError, gin.H{"status": "failed", "reason": ew.Error()})
				fmt.Printf("Error writing to file: %s\n", ew.Error())
				return
			}
			
			// Always update full hash
			fullHash.Write(buf[0:nr])
			
			// Only update 100k hash for first 100KB
			if bytesRead <= BUFSIZE {
				hash100k.Write(buf[0:nr])
			} else if bytesRead-int64(nr) < BUFSIZE {
				// Handle case where we cross the 100KB boundary mid-buffer
				// Only hash the portion up to 100KB
				remaining := BUFSIZE - (bytesRead - int64(nr))
				if remaining > 0 {
					hash100k.Write(buf[0:remaining])
				}
			}
		}
		if er != nil {
			if er != io.EOF {
				dst.Close()
				safeRemoveFile(tmpFilename, 3)
				c.JSON(http.StatusInternalServerError, gin.H{"status": "failed", "reason": er.Error()})
				fmt.Printf("Error reading from upload: %s\n", er.Error())
				return
			}
			break
		}
	}

	// Close the destination file
	if err := dst.Close(); err != nil {
		safeRemoveFile(tmpFilename, 3)
		c.JSON(http.StatusInternalServerError, gin.H{"status": "failed", "reason": err.Error()})
		fmt.Printf("Error closing temp file: %s\n", err.Error())
		return
	}

	// Calculate checksums from the hashes
	actualChecksum := fmt.Sprintf("%x", fullHash.Sum(nil))
	actualChecksum100k := fmt.Sprintf("%x", hash100k.Sum(nil))

	// Verify the full checksum matches what the client sent
	// This ensures file integrity without reading the file twice
	if actualChecksum != media.Checksum {
		safeRemoveFile(tmpFilename, 3)
		fmt.Printf("Checksum mismatch: client sent %s, but file has %s\n", media.Checksum, actualChecksum)
		c.JSON(http.StatusBadRequest, gin.H{"status": "failed", "reason": "checksum mismatch - file may be corrupted"})
		return
	}

	// Update the checksum100k in media struct
	media.Checksum100k = actualChecksum100k

	// Check for duplicate BEFORE database insert and file rename
	// This prevents creating files that will be removed due to duplicates
	if engine.DB.ChecksumExists(actualChecksum) {
		safeRemoveFile(tmpFilename, 3)
		fmt.Printf("Checksum exists: %s\n", actualChecksum)
		c.JSON(409, gin.H{"status": "exists"})
		return
	}

	// CRITICAL FIX: Insert into database FIRST, before renaming file
	// This ensures database consistency - if DB insert fails, file remains in temp location
	// and can be cleaned up. If we rename first and DB fails, we have orphaned files.
	
	// Add to batch insert buffer and flush immediately to ensure DB insert completes
	// before file is moved to final location
	err = batchInsertBuffer.Add(&media)
	if err != nil {
		safeRemoveFile(tmpFilename, 3)
		fmt.Printf("Error adding file to DB batch: %s\n", err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"status": "failed", "reason": err.Error()})
		return
	}
	
	// Force immediate flush to ensure database insert completes before file rename
	// This prevents the scenario where file is renamed but DB insert is still pending
	err = batchInsertBuffer.Flush()
	if err != nil {
		safeRemoveFile(tmpFilename, 3)
		fmt.Printf("Error flushing DB batch: %s\n", err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"status": "failed", "reason": err.Error()})
		return
	}

	// Only after successful database insert, move file to final destination
	// This ensures atomicity: either both DB insert and file rename succeed, or neither does
	if err := os.Rename(tmpFilename, newFilename); err != nil {
		// DB insert succeeded but file rename failed - this is a critical error
		// The file is in temp location, but DB has the record
		// Attempt to remove from DB to maintain consistency
		// Note: This is best-effort - if DB removal fails, manual recovery is needed
		fmt.Printf("CRITICAL: DB insert succeeded but file rename failed for %s: %s\n", tmpFilename, err.Error())
		fmt.Printf("File remains in temp location: %s\n", tmpFilename)
		fmt.Printf("Attempting to remove DB record to maintain consistency...\n")
		// Note: We don't have a DeleteFile method, so this would need to be added
		// For now, we log it for manual recovery
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

// cleanupTempFiles removes orphaned .download temp files on startup
// This prevents accumulation of temp files from crashes or interrupted uploads
func cleanupTempFiles(saveDir string) {
	count := 0
	err := filepath.Walk(saveDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Continue on errors
		}
		if !info.IsDir() && strings.HasSuffix(path, ".download") {
			if err := os.Remove(path); err == nil {
				count++
			}
		}
		return nil
	})
	if err != nil {
		fmt.Printf("Warning: Error during temp file cleanup: %v\n", err)
	} else if count > 0 {
		fmt.Printf("Cleaned up %d orphaned temp files\n", count)
	}
}

// safeRemoveFile removes a file with retry logic to handle transient errors
// This addresses silent file removal failures
func safeRemoveFile(filename string, maxRetries int) error {
	var lastErr error
	for i := 0; i < maxRetries; i++ {
		err := os.Remove(filename)
		if err == nil {
			return nil
		}
		lastErr = err
		// Exponential backoff: 10ms, 20ms, 40ms
		if i < maxRetries-1 {
			time.Sleep(time.Duration(10*(1<<uint(i))) * time.Millisecond)
		}
	}
	// Log persistent failures for manual intervention
	fmt.Printf("Warning: Failed to remove file %s after %d retries: %v\n", filename, maxRetries, lastErr)
	return lastErr
}

func main() {
	printVersion()

	// Parse command-line flags
	flags := &sortengine.ConfigFlags{}
	var uploadWorkers int
	var rateLimit int
	flag.StringVar(&flags.ConfigFile, "config", "", "Path to config file (default: ~/.gosort.yml)")
	flag.StringVar(&flags.DBFile, "database-file", "", "Database file path (overrides config)")
	flag.StringVar(&flags.SaveDir, "savedir", "", "Directory to save files (overrides config)")
	flag.StringVar(&flags.IP, "ip", "", "IP address to bind to (overrides config)")
	flag.IntVar(&flags.Port, "port", 0, "Port to listen on (overrides config)")
	flag.BoolVar(&flags.InitConfig, "init", false, "Create default config file and exit")
	flag.IntVar(&uploadWorkers, "upload-workers", 10, "Number of concurrent upload workers")
	flag.IntVar(&rateLimit, "rate-limit", 50, "Maximum uploads per second (rate limiting)")
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

	// Initialize upload queue with worker pool and rate limiting
	// This prevents the server from being overwhelmed by too many concurrent uploads
	uploadQueue = NewUploadQueue(uploadWorkers, rateLimit)
	fmt.Printf("Upload queue initialized: %d workers, %d requests/second rate limit\n", uploadWorkers, rateLimit)
	
	// Initialize batch insert buffer for efficient database writes
	// Batch size of 100 provides good balance between performance and memory usage
	batchInsertBuffer = NewBatchInsertBuffer(100)
	fmt.Printf("Batch insert buffer initialized: batch size %d\n", 100)

	ip := engine.Config.Server.IP
	port := engine.Config.Server.Port
	checkSaveDir()
	
	// Cleanup temp files on startup
	cleanupTempFiles(engine.Config.Server.SaveDir)
	
	router := gin.Default()
	//router.Use(logRequestMiddleware)
	router.POST("/file", pushFile)
	router.GET("/file", checkFile)
	router.POST("/checksums", checkChecksums)
	router.POST("/checksum100k", checkChecksum100k)
	router.GET("/version", giveVersion)
	
	// Create HTTP server with graceful shutdown support
	srv := &http.Server{
		Addr:    fmt.Sprintf("%s:%d", ip, port),
		Handler: router,
		// Set timeouts to prevent resource exhaustion
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Minute, // Large files may take time to upload
		IdleTimeout:  120 * time.Second,
	}
	
	// Start server in goroutine
	go func() {
		fmt.Printf("Starting server on %s:%d\n", ip, port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Printf("Server error: %v\n", err)
			os.Exit(1)
		}
	}()
	
	// Setup graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt) // SIGTERM is handled by the system, SIGINT is sufficient
	<-quit
	
	fmt.Printf("\nReceived shutdown signal, starting graceful shutdown...\n")
	
	// Stop accepting new connections (give 30 seconds for in-flight requests)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	
	// Shutdown HTTP server (stops accepting new requests, waits for in-flight to complete)
	if err := srv.Shutdown(ctx); err != nil {
		fmt.Printf("Error during server shutdown: %v\n", err)
	}
	
	fmt.Printf("Shutting down upload queue (waiting for in-flight uploads)...\n")
	uploadQueue.Shutdown()
	
	fmt.Printf("Flushing batch insert buffer...\n")
	if err := batchInsertBuffer.Flush(); err != nil {
		fmt.Printf("Error flushing batch insert buffer: %v\n", err)
	}
	
	sortengine.GetExiftool().Close()
	fmt.Printf("Graceful shutdown complete.\n")
	os.Exit(0)
}
