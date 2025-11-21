package main

// This application is the client for the GoSort application.
// It will send images and videos to the GoSort API for sorting.

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	//"net/url"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ascheel/gosort/internal/sortengine"
	//"github.com/veandco/go-sdl2/img"
)

var (
	Version string
)

type Client struct {
	Directory string
	Host string
	//Engine *sortengine.Engine
	config *sortengine.Config
	FileList []FileList
	httpClient *http.Client // Reused HTTP client for connection pooling
}

type FileList struct {
	Filename string
	Media sortengine.Media
	Upload bool
}

func NewClient(configPath string, flags *sortengine.ConfigFlags) *Client {
	client := &Client{}
	var err error
	
	// Load config if path is provided or use default
	if configPath == "" {
		configPath, err = sortengine.GetDefaultConfigPath()
		if err != nil {
			fmt.Printf("Error getting default config path: %s\n", err.Error())
			os.Exit(1)
		}
	}

	client.config, err = sortengine.LoadConfig(configPath)
	if err != nil {
		fmt.Printf("Error loading config: %s\n", err.Error())
		fmt.Printf("Use -init to create a default config file\n")
		os.Exit(1)
	}

	// Apply command-line flags to override config values
	if flags != nil {
		client.config.ApplyFlags(flags)
	}

	// Create a reusable HTTP client with connection pooling
	// This allows multiple requests to reuse TCP connections, reducing latency
	// Connection reuse eliminates TCP handshake overhead for subsequent requests
	transport := &http.Transport{
		// Connection pool settings
		MaxIdleConns:        100,              // Maximum idle connections across all hosts
		MaxIdleConnsPerHost: 10,               // Maximum idle connections per host (important for reuse)
		IdleConnTimeout:     90 * time.Second, // How long idle connections are kept alive
		
		// Keep-alive settings for better connection reuse
		DisableKeepAlives:   false,            // Enable HTTP keep-alive (default, but explicit)
		
		// Timeouts for connection establishment and responses
		ResponseHeaderTimeout: 30 * time.Second, // Timeout for reading response headers
		
		// TLS settings (if HTTPS is used in future)
		TLSHandshakeTimeout: 10 * time.Second,
		
		// ExpectContinueTimeout for 100-continue requests
		ExpectContinueTimeout: 1 * time.Second,
		
		// Force attempt HTTP/2 (if server supports it)
		// HTTP/2 supports multiplexing and better connection reuse
		ForceAttemptHTTP2: true,
	}
	
	client.httpClient = &http.Client{
		Transport: transport,
		Timeout:   30 * time.Minute, // Overall timeout for requests (including body read)
	}

	client.FileList = make([]FileList, 0)
	return client
}

var client *Client

func (c *Client) AddFile(media *sortengine.Media) {
	media.SetChecksum()
	c.FileList = append(c.FileList, FileList{Filename: media.Filename, Media: *media, Upload: false})
}

func (c *Client) GetVersion() (string, error) {
	var body bytes.Buffer
	request, err := http.NewRequest("GET", fmt.Sprintf("http://%s/version", c.config.Client.Host), &body)
	if err != nil {
		fmt.Printf("Error creating request: %s\n", err.Error())
		return "", err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := c.httpClient.Do(request)
	if err != nil {
		fmt.Printf("Error sending request: %s\n", err)
		return "", err
	}
	defer response.Body.Close()

	// Read response body completely to allow connection reuse
	// If body is not fully read, connection cannot be reused
	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		return "", fmt.Errorf("error reading response body: %v", err)
	}

	type ServerVersion struct {
		Version string `json:"version"`
	}
	var sver ServerVersion

	err = json.Unmarshal(responseBody, &sver)
	if err != nil {
		fmt.Printf("Error unmarshalling response: %s\n", err.Error())
		return "", err
	}
	return sver.Version, nil
}

func (c *Client) CheckForChecksums(medias []sortengine.Media) (map[string]bool, error) {
	// Create buffer to hold multipart form data
	
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	// Create a map of checksums to filenames
	fileMap := make(map[string]sortengine.Media)

	type ChecksumList struct {
		Checksums []string `json:"checksums"`
	}

	checksumList := ChecksumList{Checksums: make([]string, 0)}

	for _, media := range medias {
		md5sum, err := checksum(media.Filename)
		if err != nil {
			fmt.Printf("Error calculating checksum for %s: %s\n", media.Filename, err.Error())
			return make(map[string]bool, 0), err
		}
		fileMap[md5sum] = media
		checksumList.Checksums = append(checksumList.Checksums, md5sum)
	}

	dataBytes, err := json.Marshal(checksumList)
	if err != nil {
		fmt.Printf("Error marshalling checksums: %s\n", err.Error())
		return make(map[string]bool, 0), err
	}

	dataPart, err := writer.CreateFormField("checksums")
	if err != nil {
		fmt.Printf("Error creating form field: %s\n", err.Error())
		return make(map[string]bool, 0), err
	}

	dataPart.Write(dataBytes)

	writer.Close()

	request, err := http.NewRequest("POST", fmt.Sprintf("http://%s/checksums", c.config.Client.Host), &body)
	if err != nil {
		fmt.Printf("Error creating request: %s\n", err.Error())
		return make(map[string]bool, 0), err
	}

	request.Header.Set("Content-Type", writer.FormDataContentType())

	// Send it using the shared HTTP client
	response, err := c.httpClient.Do(request)
	if err != nil {
		fmt.Printf("Error sending request: %s\n", err.Error())
		return make(map[string]bool, 0), err
	}
	defer response.Body.Close()

	// Read response body completely to allow connection reuse
	// If body is not fully read, connection cannot be reused
	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response body: %v", err)
	}
	
	var responseData map[string]map[string]bool
	err = json.Unmarshal(responseBody, &responseData)
	if err != nil {
		return nil, fmt.Errorf("error unmarshalling response: %v", err)
	}
	
	return responseData["results"], nil
}

func (c *Client) CheckForChecksum100ks(medias []sortengine.Media) (map[string]bool, error) {
	// Create buffer to hold multipart form data
	
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	// Create a map of checksums to filenames
	fileMap := make(map[string]sortengine.Media)

	type ChecksumList struct {
		Checksums []string `json:"checksums"`
	}

	checksumList := ChecksumList{Checksums: make([]string, 0)}

	for _, media := range medias {
		md5sum, err := checksum100k(media.Filename)
		if err != nil {
			fmt.Printf("Error calculating checksum for %s: %s\n", media.Filename, err.Error())
			return make(map[string]bool, 0), err
		}
		fileMap[md5sum] = media
		checksumList.Checksums = append(checksumList.Checksums, md5sum)
	}

	dataBytes, err := json.Marshal(checksumList)
	if err != nil {
		fmt.Printf("Error marshalling checksums: %s\n", err.Error())
		return make(map[string]bool, 0), err
	}

	dataPart, err := writer.CreateFormField("checksums")
	if err != nil {
		fmt.Printf("Error creating form field: %s\n", err.Error())
		return make(map[string]bool, 0), err
	}

	dataPart.Write(dataBytes)

	writer.Close()

	request, err := http.NewRequest("POST", fmt.Sprintf("http://%s/checksum100k", c.config.Client.Host), &body)
	if err != nil {
		fmt.Printf("Error creating request: %s\n", err.Error())
		return make(map[string]bool, 0), err
	}

	request.Header.Set("Content-Type", writer.FormDataContentType())

	// Send it using the shared HTTP client
	response, err := c.httpClient.Do(request)
	if err != nil {
		fmt.Printf("Error sending request: %s\n", err.Error())
		return make(map[string]bool, 0), err
	}
	defer response.Body.Close()

	// Read response body completely to allow connection reuse
	// If body is not fully read, connection cannot be reused
	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response body: %v", err)
	}
	
	var responseData map[string]map[string]bool
	err = json.Unmarshal(responseBody, &responseData)
	if err != nil {
		return nil, fmt.Errorf("error unmarshalling response: %v", err)
	}
	
	return responseData["results"], nil
}

func (c *Client) ChecksumExists(media *sortengine.Media) bool {
	checksums, err := c.CheckForChecksums([]sortengine.Media{*media})
	if err != nil {
		fmt.Printf("Error checking for checksums: %s\n", err.Error())
		return false
	}
	for _, v := range checksums {
		// We only need the first one.
		return v
	}
	return false
}

func (c *Client) Checksum100kExists(media *sortengine.Media) bool {
	checksums, err := c.CheckForChecksum100ks([]sortengine.Media{*media})
	if err != nil {
		fmt.Printf("Error checking for checksums: %s\n", err.Error())
		return false
	}
	for _, v := range checksums {
		// We only need the first one.
		return v
	}
	return false
}

//func (c *Client) SendFile(filename string) error {
func (c *Client) SendFile(media *sortengine.Media) error {
	// Open the file
	file, err := os.Open(media.Filename)
	if err != nil {
		fmt.Printf("Error opening file: %s\n", err.Error())
		return err
	}
	defer file.Close()

	// Check if checksum100k already exists on host
	if c.Checksum100kExists(media) && c.ChecksumExists(media) {
		fmt.Printf("Checksum already exists on server.  Skipping file %s.\n", media.Filename)
		return nil
	}

	// Use io.Pipe() for streaming uploads instead of buffering in memory
	// This allows large files to be uploaded without consuming excessive RAM
	// The pipe connects the multipart writer to the HTTP request body
	// Data flows through the pipe as it's written, not all at once
	pipeReader, pipeWriter := io.Pipe()
	writer := multipart.NewWriter(pipeWriter)

	// Error channel to capture errors from the goroutine
	errChan := make(chan error, 1)

	// Goroutine to write multipart data to the pipe
	// This runs concurrently with the HTTP request, streaming data
	go func() {
		defer pipeWriter.Close()
		defer writer.Close()

		// Write media metadata field
		mediaPart, err := writer.CreateFormField("media")
		if err != nil {
			errChan <- fmt.Errorf("error creating form field: %v", err)
			return
		}

		mediaJson, err := json.Marshal(media)
		if err != nil {
			errChan <- fmt.Errorf("error marshalling media: %v", err)
			return
		}
		_, err = mediaPart.Write(mediaJson)
		if err != nil {
			errChan <- fmt.Errorf("error writing media field: %v", err)
			return
		}

		// Create form file field
		part, err := writer.CreateFormFile("file", filepath.Base(media.Filename))
		if err != nil {
			errChan <- fmt.Errorf("error creating form file: %v", err)
			return
		}

		// Stream file data to multipart writer
		// This writes to the pipe, which is read by the HTTP request
		_, err = io.Copy(part, file)
		if err != nil {
			errChan <- fmt.Errorf("error copying file: %v", err)
			return
		}

		// Close writer to finalize multipart form
		err = writer.Close()
		if err != nil {
			errChan <- fmt.Errorf("error closing writer: %v", err)
			return
		}

		// Signal success
		errChan <- nil
	}()

	// Create the POST request with pipe reader as body
	// The HTTP client will read from the pipe as data becomes available
	url := fmt.Sprintf("http://%s/file", c.config.Client.Host)
	request, err := http.NewRequest("POST", url, pipeReader)
	if err != nil {
		pipeReader.Close()
		return fmt.Errorf("error creating request: %v", err)
	}

	request.Header.Set("Content-Type", writer.FormDataContentType())

	// Send request - data streams through the pipe
	// The goroutine writes to pipeWriter, HTTP client reads from pipeReader
	response, err := c.httpClient.Do(request)
	if err != nil {
		pipeReader.Close()
		// Wait for goroutine to finish
		<-errChan
		return fmt.Errorf("error sending request: %v", err)
	}
	defer response.Body.Close()

	// Check for errors from the goroutine
	select {
	case err := <-errChan:
		if err != nil {
			return err
		}
	default:
		// No error, continue
	}

	// Read response body completely to allow connection reuse
	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		return fmt.Errorf("error reading response: %v", err)
	}

	var responseMap map[string]string
	err = json.Unmarshal(responseBody, &responseMap)
	if err != nil {
		return fmt.Errorf("error decoding response: %v", err)
	}

	fmt.Printf("Uploaded: %s\n", media.Filename)
	
	return nil
}

func TestUpload() {
	homedir, err := os.UserHomeDir()
	if err != nil {
		fmt.Printf("Error getting home directory: %s\n", err.Error())
		os.Exit(1)
	}

	filename := filepath.Join(homedir, "pics/2015/20150802_222506.jpg")
	client.SendFile(sortengine.NewMediaFile(filename))
}

func TestChecksum() {
	homedir, err := os.UserHomeDir()
	if err != nil {
		fmt.Printf("Error getting home directory: %s\n", err.Error())
		os.Exit(1)
	}

	filename := filepath.Join(homedir, "pics/2015/20150802_222506.jpg")
	media := sortengine.NewMediaFile(filename)
	client.CheckForChecksums([]sortengine.Media{*media})
}

func checksum(filename string) (string, error) {
	f, err := os.Open(filename)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := md5.New()

	// Get the file's checksum
	_, err = io.Copy(h, f)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

// Goroutines Explained:
// Goroutines are lightweight threads managed by the Go runtime. They allow concurrent execution
// of functions without the overhead of traditional OS threads. Key benefits:
// - Very cheap to create (only a few KB of stack space)
// - Managed by Go runtime scheduler (M:N threading model)
// - Communicate via channels (safe concurrent data sharing)
// - Much faster than OS threads for I/O-bound operations
//
// In this implementation, we use goroutines to process multiple files concurrently,
// dramatically improving performance when dealing with many files.

// FileInfo holds information about a file to be processed
type FileInfo struct {
	Path string
	Info os.FileInfo
}

// ProcessStats tracks processing statistics across goroutines
// Uses atomic operations for thread-safe concurrent access
type ProcessStats struct {
	TotalFiles    int64
	Processed     int64
	Uploaded      int64
	Skipped       int64
	Errors        int64
}

// ProgressReporter handles progress reporting with time estimation
type ProgressReporter struct {
	startTime    time.Time
	lastUpdate   time.Time
	updateInterval time.Duration
	phase        string
	total        int64
	processed    *int64 // Pointer to atomic counter
	mu           sync.Mutex
}

// NewProgressReporter creates a new progress reporter
func NewProgressReporter(phase string, total int64, processed *int64) *ProgressReporter {
	return &ProgressReporter{
		startTime:     time.Now(),
		lastUpdate:    time.Now(),
		updateInterval: 500 * time.Millisecond, // Update every 500ms
		phase:         phase,
		total:         total,
		processed:     processed,
	}
}

// Update displays progress if enough time has passed since last update
func (pr *ProgressReporter) Update() {
	pr.mu.Lock()
	defer pr.mu.Unlock()
	
	now := time.Now()
	if now.Sub(pr.lastUpdate) < pr.updateInterval {
		return // Skip update if too soon
	}
	pr.lastUpdate = now
	
	processed := atomic.LoadInt64(pr.processed)
	pr.printProgress(processed)
}

// printProgress displays the progress bar and statistics
func (pr *ProgressReporter) printProgress(processed int64) {
	if pr.total == 0 {
		return
	}
	
	percentage := float64(processed) / float64(pr.total) * 100
	if percentage > 100 {
		percentage = 100
	}
	
	// Calculate elapsed time
	elapsed := time.Since(pr.startTime)
	
	// Calculate estimated time remaining
	var remaining time.Duration
	if processed > 0 {
		avgTimePerItem := elapsed / time.Duration(processed)
		remainingItems := pr.total - processed
		remaining = avgTimePerItem * time.Duration(remainingItems)
	}
	
	// Create progress bar (50 characters wide)
	barWidth := 50
	filled := int(float64(barWidth) * percentage / 100)
	if filled > barWidth {
		filled = barWidth
	}
	
	bar := make([]byte, barWidth)
	for i := 0; i < filled; i++ {
		bar[i] = '='
	}
	for i := filled; i < barWidth; i++ {
		bar[i] = ' '
	}
	
	// Format time remaining
	var remainingStr string
	if remaining > 0 {
		if remaining > time.Hour {
			remainingStr = fmt.Sprintf("%.1fh", remaining.Hours())
		} else if remaining > time.Minute {
			remainingStr = fmt.Sprintf("%.1fm", remaining.Minutes())
		} else {
			remainingStr = fmt.Sprintf("%.0fs", remaining.Seconds())
		}
	} else {
		remainingStr = "calculating..."
	}
	
	// Print progress line (overwrite previous line)
	fmt.Printf("\r%s [%s] %3.1f%% (%d/%d) | Elapsed: %s | Remaining: %s",
		pr.phase,
		string(bar),
		percentage,
		processed,
		pr.total,
		formatDuration(elapsed),
		remainingStr,
	)
}

// Finish completes the progress display
func (pr *ProgressReporter) Finish() {
	pr.mu.Lock()
	defer pr.mu.Unlock()
	
	processed := atomic.LoadInt64(pr.processed)
	pr.printProgress(processed)
	elapsed := time.Since(pr.startTime)
	fmt.Printf("\n%s completed in %s\n", pr.phase, formatDuration(elapsed))
}

// formatDuration formats a duration in a human-readable way
func formatDuration(d time.Duration) string {
	if d > time.Hour {
		return fmt.Sprintf("%.1fh", d.Hours())
	} else if d > time.Minute {
		return fmt.Sprintf("%.1fm", d.Minutes())
	} else {
		return fmt.Sprintf("%.0fs", d.Seconds())
	}
}

// FileWithChecksums holds a file path and its calculated checksums
type FileWithChecksums struct {
	Path        string
	Media       *sortengine.Media
	Checksum    string
	Checksum100k string
}

// BatchCheckResult holds the result of a batch checksum check
type BatchCheckResult struct {
	Checksum    string
	Checksum100k string
	Exists      bool
	Exists100k  bool
}

// processFile handles uploading a single file (checksums already verified)
func (c *Client) processFile(media *sortengine.Media, stats *ProcessStats) {
	defer atomic.AddInt64(&stats.Processed, 1)

	// Upload the file
	if err := c.SendFile(media); err != nil {
		// Error is logged but we don't print here to avoid cluttering progress bar
		atomic.AddInt64(&stats.Errors, 1)
		return
	}

	atomic.AddInt64(&stats.Uploaded, 1)
	// Progress is updated by the progress reporter, no need to print here
}

// parallelWalkDir walks a directory tree in parallel using a worker pool
// This is much faster than filepath.Walk for large directory trees with many subdirectories
// It uses goroutines to scan multiple directories concurrently
func (c *Client) parallelWalkDir(ctx context.Context, root string, filesChan chan<- FileInfo, numWorkers int) error {
	// Channel for directories to scan
	dirChan := make(chan string, numWorkers*2)
	
	// Track visited directories to avoid infinite loops (symlinks)
	visitedDirs := make(map[string]bool)
	var visitedMu sync.Mutex
	
	// Worker pool for scanning directories
	var scanWg sync.WaitGroup
	scanWg.Add(numWorkers)
	
	// Start directory scanning workers
	for i := 0; i < numWorkers; i++ {
		go func() {
			defer scanWg.Done()
			for dirPath := range dirChan {
				select {
				case <-ctx.Done():
					return
				default:
					c.scanDirectory(ctx, dirPath, filesChan, dirChan, &visitedMu, visitedDirs)
				}
			}
		}()
	}
	
	// Start with root directory
	dirChan <- root
	
	// Close dirChan when all directories are processed
	go func() {
		scanWg.Wait()
		close(dirChan)
	}()
	
	// Wait for all scanning to complete
	scanWg.Wait()
	
	return nil
}

// scanDirectory scans a single directory and processes files/subdirectories
// Implements error handling with retries for slow I/O
func (c *Client) scanDirectory(ctx context.Context, dirPath string, filesChan chan<- FileInfo, dirChan chan<- string, visitedMu *sync.Mutex, visitedDirs map[string]bool) {
	// Check if we've already visited this directory (avoid symlink loops)
	visitedMu.Lock()
	absPath, err := filepath.Abs(dirPath)
	if err != nil {
		visitedMu.Unlock()
		return
	}
	if visitedDirs[absPath] {
		visitedMu.Unlock()
		return
	}
	visitedDirs[absPath] = true
	visitedMu.Unlock()
	
	// Retry logic for slow I/O operations
	maxRetries := 3
	retryDelay := 100 * time.Millisecond
	
	var entries []os.FileInfo
	
	// Retry reading directory with exponential backoff
	for attempt := 0; attempt < maxRetries; attempt++ {
		select {
		case <-ctx.Done():
			return
		default:
		}
		
		var dir *os.File
		var err error
		dir, err = os.Open(dirPath)
		if err != nil {
			if attempt < maxRetries-1 {
				time.Sleep(retryDelay * time.Duration(attempt+1))
				continue
			}
			// Last attempt failed, log and return
			fmt.Printf("Error opening directory %s (after %d retries): %s\n", dirPath, maxRetries, err.Error())
			return
		}
		
		entries, err = dir.Readdir(-1)
		dir.Close()
		
		if err != nil {
			if attempt < maxRetries-1 {
				time.Sleep(retryDelay * time.Duration(attempt+1))
				continue
			}
			// Last attempt failed, log and return
			fmt.Printf("Error reading directory %s (after %d retries): %s\n", dirPath, maxRetries, err.Error())
			return
		}
		
		// Success, break out of retry loop
		break
	}
	
	// Process entries
	for _, entry := range entries {
		select {
		case <-ctx.Done():
			return
		default:
		}
		
		fullPath := filepath.Join(dirPath, entry.Name())
		
		if entry.IsDir() {
			// Add subdirectory to scan queue
			select {
			case dirChan <- fullPath:
			case <-ctx.Done():
				return
			}
		} else {
			// Send file to processing channel
			select {
			case filesChan <- FileInfo{Path: fullPath, Info: entry}:
			case <-ctx.Done():
				return
			}
		}
	}
}

// BatchCheckChecksums checks multiple checksums in a single HTTP request
// This dramatically reduces the number of HTTP requests needed
func (c *Client) BatchCheckChecksums(checksums []string, endpoint string) (map[string]bool, error) {
	if len(checksums) == 0 {
		return make(map[string]bool), nil
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	type ChecksumList struct {
		Checksums []string `json:"checksums"`
	}

	checksumList := ChecksumList{Checksums: checksums}
	dataBytes, err := json.Marshal(checksumList)
	if err != nil {
		return nil, fmt.Errorf("error marshalling checksums: %v", err)
	}

	dataPart, err := writer.CreateFormField("checksums")
	if err != nil {
		return nil, fmt.Errorf("error creating form field: %v", err)
	}

	dataPart.Write(dataBytes)
	writer.Close()

	request, err := http.NewRequest("POST", fmt.Sprintf("http://%s%s", c.config.Client.Host, endpoint), &body)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %v", err)
	}

	request.Header.Set("Content-Type", writer.FormDataContentType())

	response, err := c.httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("error sending request: %v", err)
	}
	defer response.Body.Close()

	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response: %v", err)
	}

	var responseData map[string]map[string]bool
	err = json.Unmarshal(responseBody, &responseData)
	if err != nil {
		return nil, fmt.Errorf("error unmarshalling response: %v", err)
	}

	return responseData["results"], nil
}

// ProcessDirectory processes files in a directory using a two-phase approach:
// Phase 1: Collect all files and calculate checksums in parallel
// Phase 2: Batch check all checksums, then upload only files that don't exist
// This dramatically reduces HTTP requests from 2 per file to ~2 per 100 files
func (c *Client) ProcessDirectory(dir string, numWorkers int) error {
	fmt.Printf("Scanning directory: %s\n", dir)
	
	// Statistics tracking
	stats := &ProcessStats{}
	
	// Context for cancellation support
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	
	// Phase 1: Collect all files and calculate checksums in parallel
	fmt.Printf("Phase 1: Collecting files and calculating checksums...\n")
	
	filesChan := make(chan FileInfo, numWorkers*2)
	resultsChan := make(chan FileWithChecksums, numWorkers*2)
	
	// Progress tracking for Phase 1
	phase1Processed := int64(0)
	phase1Reporter := NewProgressReporter("Phase 1", 0, &phase1Processed) // Total unknown initially
	
	var collectWg sync.WaitGroup
	
	// Start workers to calculate checksums in parallel
	for i := 0; i < numWorkers; i++ {
		collectWg.Add(1)
		go func() {
			defer collectWg.Done()
			for fileInfo := range filesChan {
				select {
				case <-ctx.Done():
					return
				default:
					// Calculate checksums for this file
					media := sortengine.NewMediaFile(fileInfo.Path)
					if media == nil {
						atomic.AddInt64(&stats.Errors, 1)
						continue
					}
					
					if err := media.SetChecksum(); err != nil {
						fmt.Printf("Error calculating checksum for %s: %s\n", fileInfo.Path, err.Error())
						atomic.AddInt64(&stats.Errors, 1)
						continue
					}
					
					// Calculate checksum100k
					checksum100k, err := checksum100k(fileInfo.Path)
					if err != nil {
						fmt.Printf("Error calculating checksum100k for %s: %s\n", fileInfo.Path, err.Error())
						atomic.AddInt64(&stats.Errors, 1)
						continue
					}
					
					atomic.AddInt64(&stats.TotalFiles, 1)
					atomic.AddInt64(&phase1Processed, 1)
					phase1Reporter.Update()
					resultsChan <- FileWithChecksums{
						Path:        fileInfo.Path,
						Media:       media,
						Checksum:    media.Checksum,
						Checksum100k: checksum100k,
					}
				}
			}
		}()
	}
	
	// Parallel directory walker - scans directories concurrently
	// This is much faster than filepath.Walk for large directory trees
	var walkWg sync.WaitGroup
	walkWg.Add(1)
	go func() {
		defer walkWg.Done()
		defer close(filesChan)
		
		// Use parallel directory walker instead of synchronous filepath.Walk
		if err := c.parallelWalkDir(ctx, dir, filesChan, numWorkers); err != nil {
			fmt.Printf("Error walking directory: %s\n", err.Error())
		}
	}()
	
	// Wait for directory walk to complete
	walkWg.Wait()
	
	// Wait for all checksum workers to finish
	collectWg.Wait()
	close(resultsChan)
	
	// Collect all results
	var allFiles []FileWithChecksums
	for file := range resultsChan {
		allFiles = append(allFiles, file)
	}
	
	totalFiles := len(allFiles)
	atomic.StoreInt64(&stats.TotalFiles, int64(totalFiles))
	
	// Update Phase 1 total and finish
	phase1Reporter.total = int64(totalFiles)
	phase1Reporter.Finish()
	
	if totalFiles == 0 {
		fmt.Printf("No files to process.\n")
		return nil
	}
	
	// Phase 2: Batch check all checksums
	fmt.Printf("\nPhase 2: Batch checking checksums (this reduces HTTP requests dramatically)...\n")
	
	// Collect all checksums
	checksums := make([]string, 0, totalFiles)
	checksums100k := make([]string, 0, totalFiles)
	checksumToFile := make(map[string]*FileWithChecksums)
	checksum100kToFile := make(map[string]*FileWithChecksums)
	
	for i := range allFiles {
		file := &allFiles[i]
		checksums = append(checksums, file.Checksum)
		checksums100k = append(checksums100k, file.Checksum100k)
		checksumToFile[file.Checksum] = file
		checksum100kToFile[file.Checksum100k] = file
	}
	
	// Batch check in groups (e.g., 100 at a time to avoid huge requests)
	batchSize := 100
	existsMap := make(map[string]bool)
	exists100kMap := make(map[string]bool)
	
	// Progress tracking for Phase 2 (2 batches: full checksums + 100k checksums)
	totalBatches := (len(checksums) + batchSize - 1) / batchSize + (len(checksums100k) + batchSize - 1) / batchSize
	phase2Processed := int64(0)
	phase2Reporter := NewProgressReporter("Phase 2", int64(totalBatches), &phase2Processed)
	
	// Batch check full checksums
	for i := 0; i < len(checksums); i += batchSize {
		end := i + batchSize
		if end > len(checksums) {
			end = len(checksums)
		}
		batch := checksums[i:end]
		batchResults, err := c.BatchCheckChecksums(batch, "/checksums")
		if err != nil {
			fmt.Printf("\nError batch checking checksums: %s\n", err.Error())
			// Fall back to individual checks if batch fails
			for _, cs := range batch {
				existsMap[cs] = false
			}
		} else {
			for cs, exists := range batchResults {
				existsMap[cs] = exists
			}
		}
		atomic.AddInt64(&phase2Processed, 1)
		phase2Reporter.Update()
	}
	
	// Batch check 100k checksums
	for i := 0; i < len(checksums100k); i += batchSize {
		end := i + batchSize
		if end > len(checksums100k) {
			end = len(checksums100k)
		}
		batch := checksums100k[i:end]
		batchResults, err := c.BatchCheckChecksums(batch, "/checksum100k")
		if err != nil {
			fmt.Printf("\nError batch checking checksums100k: %s\n", err.Error())
			// Fall back to individual checks if batch fails
			for _, cs := range batch {
				exists100kMap[cs] = false
			}
		} else {
			for cs, exists := range batchResults {
				exists100kMap[cs] = exists
			}
		}
		atomic.AddInt64(&phase2Processed, 1)
		phase2Reporter.Update()
	}
	
	phase2Reporter.Finish()
	
	// Phase 3: Upload only files that don't exist
	fmt.Printf("\nPhase 3: Uploading files that don't exist...\n")
	
	// First, determine which files need to be uploaded
	// A file is a duplicate only if BOTH checksums exist
	var filesToUpload []*sortengine.Media
	for i := range allFiles {
		file := &allFiles[i]
		exists := existsMap[file.Checksum]
		exists100k := exists100kMap[file.Checksum100k]
		
		if exists && exists100k {
			// File already exists, skip it
			atomic.AddInt64(&stats.Skipped, 1)
		} else {
			// File doesn't exist, add to upload list
			filesToUpload = append(filesToUpload, file.Media)
		}
	}
	
	uploadTotal := int64(len(filesToUpload))
	if uploadTotal == 0 {
		fmt.Printf("No files to upload (all are duplicates).\n")
	} else {
		// Progress tracking for Phase 3
		phase3Reporter := NewProgressReporter("Phase 3", uploadTotal, &stats.Processed)
		
		uploadChan := make(chan *sortengine.Media, numWorkers*2)
		var uploadWg sync.WaitGroup
		
		// Start progress updater goroutine
		stopProgress := make(chan bool)
		go func() {
			ticker := time.NewTicker(500 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					phase3Reporter.Update()
				case <-stopProgress:
					return
				}
			}
		}()
		
		// Start upload workers
		for i := 0; i < numWorkers; i++ {
			uploadWg.Add(1)
			go func() {
				defer uploadWg.Done()
				for media := range uploadChan {
					select {
					case <-ctx.Done():
						return
					default:
						c.processFile(media, stats)
					}
				}
			}()
		}
		
		// Feed files to upload workers
		for _, media := range filesToUpload {
			select {
			case uploadChan <- media:
			case <-ctx.Done():
				close(uploadChan)
				uploadWg.Wait()
				stopProgress <- true
				return ctx.Err()
			}
		}
		
		close(uploadChan)
		uploadWg.Wait()
		stopProgress <- true
		phase3Reporter.Finish()
	}
	
	// Print final statistics
	fmt.Printf("\n=== Processing Complete ===\n")
	fmt.Printf("Total files:    %d\n", atomic.LoadInt64(&stats.TotalFiles))
	fmt.Printf("Uploaded:       %d\n", atomic.LoadInt64(&stats.Uploaded))
	fmt.Printf("Skipped:        %d\n", atomic.LoadInt64(&stats.Skipped))
	fmt.Printf("Errors:         %d\n", atomic.LoadInt64(&stats.Errors))
	
	return nil
}

// WalkDir is the legacy sequential implementation (kept for backward compatibility)
// New code should use ProcessDirectory instead
func WalkDir(dir string) (error) {
	fmt.Printf("Scanning files, please wait...\n")
	
	// Use parallel processing with 10 workers by default
	// This is the legacy function - new code should use ProcessDirectory directly
	client.ProcessDirectory(dir, 10)
	
	return nil
}

func checksum100k(filename string) (string, error) {
	f, err := os.Open(filename)
	if err != nil {
		return "", err
	}
	defer f.Close()

	// Set the checksum function
	h := md5.New()

	// Get the file's checksum
	var BUFSIZE int64 = 102400
	finfo, err := os.Stat(filename)
	if err != nil {
		return "", err
	}
	if finfo.Size() < BUFSIZE {
		BUFSIZE = finfo.Size()
	}
	_, err = io.CopyN(h, f, BUFSIZE)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

func printVersion() {
	fmt.Printf("GoSort Client Version: %s\n", Version)
}

func CheckVersion() {
	// Get version from server and compare with client version.
	serverVersion, err := client.GetVersion()
	if err != nil {
		fmt.Printf("Error getting version: %s\n", err.Error())
		os.Exit(1)
	}
	var compareString string
	if serverVersion == Version {
		compareString = "=="
	} else {
		compareString = "!="
	}
	fmt.Printf("Comparing client version with server version.\n")
	fmt.Printf(" Client: %s %s Server: %s\n", Version, compareString, serverVersion)

	if serverVersion != Version {
		os.Exit(1)
	}
}

func main() {
	printVersion()

	// Parse command-line flags
	flags := &sortengine.ConfigFlags{}
	var configPath string
	numWorkers := flag.Int("workers", 10, "Number of parallel workers for file processing")
	flag.StringVar(&configPath, "config", "", "Path to config file (default: ~/.gosort.yml)")
	flag.StringVar(&flags.Host, "host", "", "Server host address (overrides config)")
	flag.BoolVar(&flags.InitConfig, "init", false, "Create default config file and exit")
	flag.Parse()

	// Handle -init flag
	if flags.InitConfig {
		initPath := configPath
		if initPath == "" {
			var err error
			initPath, err = sortengine.GetDefaultConfigPath()
			if err != nil {
				fmt.Printf("Error getting default config path: %s\n", err.Error())
				os.Exit(1)
			}
		}
		if err := sortengine.CreateDefaultConfig(initPath); err != nil {
			fmt.Printf("Error creating config file: %s\n", err.Error())
			os.Exit(1)
		}
		os.Exit(0)
	}

	// Check for directory argument
	args := flag.Args()
	if len(args) < 1 {
		fmt.Println("Usage: client [flags] <directory>")
		fmt.Println("\nFlags:")
		flag.PrintDefaults()
		os.Exit(1)
	}

	// Initialize client with config
	client = NewClient(configPath, flags)

	//TestChecksum()
	// TestUpload()
	// os.Exit(0)

	CheckVersion()

	dir := args[0]
	
	// Use parallel processing with configurable number of workers
	// Goroutines allow concurrent file processing, dramatically improving performance
	if err := client.ProcessDirectory(dir, *numWorkers); err != nil {
		fmt.Printf("Error processing directory: %s\n", err.Error())
		os.Exit(1)
	}
}
