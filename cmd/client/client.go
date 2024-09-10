package main

// This application is the client for the GoSort application.
// It will send images and videos to the GoSort API for sorting.

import (
	"bytes"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	//"net/url"
	"os"
	"path/filepath"

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
}

func NewClient() *Client {
	client := &Client{}
	var err error
	client.config, err = sortengine.LoadConfig()
	if err != nil {
		fmt.Printf("Error loading config: %s\n", err.Error())
		os.Exit(1)
	}
	return client
}

func (c *Client) CheckForChecksums(filenames []string) (map[string]bool, error) {
	fileMap := make(map[string]string)
	checksumList := make([]string, 0)
	//var err error
	for _, filename := range filenames {
		md5sum, err := checksum(filename)
		if err != nil {
			fmt.Printf("Error calculating checksum: %s\n", err.Error())
			return make(map[string]bool, 0), err
		}
		fileMap[filename] = md5sum
		checksumList = append(checksumList, md5sum)
	}

	url := fmt.Sprintf("http://%s/checksums", c.config.Client.Host)
	jsonBytes, err := json.Marshal(checksumList)
	request, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonBytes))
	if err != nil {
		fmt.Printf("Error creating request: %s\n", err.Error())
		return make(map[string]bool), err
	}

	request.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	response, err := client.Do(request)
	if err != nil {
		fmt.Printf("Error sending request: %s\n", err.Error())
		return make(map[string]bool), err
	}
	defer response.Body.Close()

	fmt.Printf("Response: %v\n", response.Body)
	return make(map[string]bool), nil
}

func (c *Client) SendFile(filename string) error {
	fmt.Println("Sending file...")
	
	// Open the file
	file, err := os.Open(filename)
	if err != nil {
		fmt.Printf("Error opening file: %s\n", err.Error())
		return err
	}
	defer file.Close()

	// Check if checksum already exists on host
	result, err := c.CheckForChecksums([]string{filename})
	fmt.Printf("Result: %v\n", result)

	// Create buffer.  This will hold our multipart form data
	var requestBody bytes.Buffer
	writer := multipart.NewWriter(&requestBody)

	// Create a new form-data field
	part, err := writer.CreateFormFile("file", filepath.Base(filename))
	if err != nil {
		fmt.Printf("Error creating form file: %s\n", err.Error())
		return err
	}

	// Copy the file to the form-data field
	_, err = io.Copy(part, file)
	if err != nil {
		fmt.Printf("Error copying file to form file: %s\n", err.Error())
		return err
	}

	// Close the multipart writer to set the terminating boundary

	err = writer.Close()
	if err != nil {
		fmt.Printf("Error closing writer: %s\n", err.Error())
		return err
	}

	// Create the POST request
	url := fmt.Sprintf("http://%s/file", c.config.Client.Host)
	request, err := http.NewRequest("POST", url, &requestBody)
	if err != nil {
		// Error occurred.
		fmt.Printf("Unable to POST: %v\n", err)
		return err
	}

	request.Header.Set("Content-Type", writer.FormDataContentType())

	// SEND IT!
	httpClient := &http.Client{}
	response, err := httpClient.Do(request)
	if err != nil {
		fmt.Printf("136 Error sending request: %s\n", err.Error())
		return err
	}
	defer response.Body.Close()

	fmt.Printf("Response: %v\n", response)

	return nil
}

func TestUpload() {
	client := NewClient()

	filename := "/home/scheel/pics/2015/20150802_222506.jpg"
	client.SendFile(filename)
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

func WalkFunc(path string, info os.FileInfo, err error) error {
	if err != nil {
		return err
	}

	if info.IsDir() {
		return nil
	}

	// Process stuff
	img := sortengine.NewMediaFile(info.Name())
	fmt.Printf("Processing %s\n", img.Filename)

	return nil
}

func WalkDir(dir string) (error) {
	filepath.Walk(dir, WalkFunc)
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
	_, err = io.CopyN(h, f, BUFSIZE)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

func printVersion() {
	fmt.Printf("GoSort Client Version: %s\n", Version)
}

func main() {
	printVersion()
	if len(os.Args) < 2 {
		fmt.Println("Usage: send <directory>")
		os.Exit(1)
	}

	TestUpload()
	os.Exit(0)

	dir := os.Args[1]
	WalkDir(dir)
}
