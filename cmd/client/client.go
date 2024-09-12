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
	// Create buffer to hold multipart form data
	
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	// Create a map of checksums to filenames
	fileMap := make(map[string]string)

	type ChecksumList struct {
		Checksums []string `json:"checksums"`
	}

	checksumList := ChecksumList{Checksums: make([]string, 0)}

	for _, filename := range filenames {
		md5sum, err := checksum(filename)
		if err != nil {
			fmt.Printf("Error calculating checksum: %s\n", err.Error())
			return make(map[string]bool, 0), err
		}
		fileMap[md5sum] = filename
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

	// Send it
	client := &http.Client{}
	response, err := client.Do(request)
	if err != nil {
		fmt.Printf("Error sending request: %s\n", err.Error())
		return make(map[string]bool, 0), err
	}
	defer response.Body.Close()

	responseBody, _ := io.ReadAll(response.Body)
	var responseData map[string]map[string]bool
	//var responseData map[string]bool
	err = json.Unmarshal(responseBody, &responseData)
	if err != nil {
		fmt.Printf("Error unmarshalling response: %s\n", err.Error())
		return make(map[string]bool, 0), err
	}
	
	fmt.Printf("Response: %v\n", responseData["results"])
	return responseData["results"], nil
}

func (c *Client) ChecksumExists(filename string) bool {
	checksums, err := c.CheckForChecksums([]string{filename})
	if err != nil {
		fmt.Printf("Error checking for checksums: %s\n", err.Error())
		return false
	}
	for _, v := range checksums {
		return v
	}
	return false
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
	if c.ChecksumExists(filename) {
		fmt.Printf("Checksum already exists on server.  Skipping file.\n")
		return nil
	}

	// Create buffer.  This will hold our multipart form data
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	media := sortengine.NewMediaFile(filename)
	mediaPart, err := writer.CreateFormField("media")
	if err != nil {
		fmt.Printf("Error creating form field: %s\n", err.Error())
		return err
	}

	mediaJson, err := json.Marshal(media)
	if err != nil {
		fmt.Printf("Error marshalling media: %s\n", err.Error())
		return err
	}
	mediaPart.Write(mediaJson)

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
	request, err := http.NewRequest("POST", url, &body)
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

	homedir, err := os.UserHomeDir()
	if err != nil {
		fmt.Printf("Error getting home directory: %s\n", err.Error())
		os.Exit(1)
	}

	filename := filepath.Join(homedir, "pics/2015/20150802_222506.jpg")
	client.SendFile(filename)
}

func TestChecksum() {
	client := NewClient()
	homedir, err := os.UserHomeDir()
	if err != nil {
		fmt.Printf("Error getting home directory: %s\n", err.Error())
		os.Exit(1)
	}

	filename := filepath.Join(homedir, "pics/2015/20150802_222506.jpg")
	client.CheckForChecksums([]string{filename})
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

	//TestChecksum()
	TestUpload()
	os.Exit(0)

	dir := os.Args[1]
	WalkDir(dir)
}
