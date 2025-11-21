package main

// This application is the client for the GoSort application.
// It will send images and videos to the GoSort API for sorting.

import (
	"bytes"
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
	client := &http.Client{}
	response, err := client.Do(request)
	if err != nil {
		fmt.Printf("Error sending request: %s\n", err)
		return "", err
	}
	defer response.Body.Close()

	responseBody, _ := io.ReadAll(response.Body)

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

	// Create buffer.  This will hold our multipart form data
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	//media := sortengine.NewMediaFile(filename)
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
	part, err := writer.CreateFormFile("file", filepath.Base(media.Filename))
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
		fmt.Printf("Error sending request: %s\n", err.Error())
		return err
	}
	defer response.Body.Close()

	var responseMap map[string]string
	err = json.NewDecoder(response.Body).Decode(&responseMap)
	if err != nil {
		fmt.Printf("Error decoding response: %s\n", err.Error())
		return err
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

func WalkFunc(path string, info os.FileInfo, err error) error {
	if err != nil {
		return err
	}

	if info.IsDir() {
		return nil
	}

	// Process stuff
	img := sortengine.NewMediaFile(path)
	img.SetChecksum()
	//client.AddFile(img)
	client.SendFile(img)
	return nil
}

func UploadFiles() {
	count := 0
	for _, file := range client.FileList {
		count += 1
		fmt.Printf("(%04d) Uploading %s\n", count, file.Media.Filename)
		client.SendFile(&file.Media)
	}
}

func WalkDir(dir string) (error) {
	fmt.Printf("Scanning files, please wait...\n")
	filepath.Walk(dir, WalkFunc)
	UploadFiles()
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
	WalkDir(dir)
}
