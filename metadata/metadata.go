package main

import (
	"crypto/md5"
	"crypto/sha256"
	"fmt"
	"hash"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"reflect"

	"github.com/barasher/go-exiftool"
	"github.com/kolesa-team/goexiv"
)

func FileExtMatches(filename string, exts []string) bool {
	fileExt := filepath.Ext(filename)
	if len(fileExt) < 2 {
		fileExt = ""
	} else {
		fileExt = fileExt[1:]
	}

	for _, ext := range exts {
		if strings.EqualFold(ext, fileExt) {
			return true
		}
	}
	return false
}

func IsImage(filename string) bool {
	if _, ok := os.Stat(filename); os.IsNotExist(ok) {
		return false
	}

	exts := []string{"jpg", "jpeg", "gif", "tif", "tiff", "png"}
	return FileExtMatches(filename, exts)
}

func IsVideo(filename string) bool {
	if _, ok := os.Stat(filename); os.IsNotExist(ok) {
		return false
	}

	exts := []string{"mp4", "mkv", "m4v", "avi", "mpg"}
	return FileExtMatches(filename, exts)
}

func GetImageMetadata(filename string) (map[string]string, error) {
	img, err := goexiv.Open(filename)
	if err != nil {
		panic(err)
	}

	err = img.ReadMetadata()
	if err != nil {
		panic(err)
	}

	metadata := img.GetExifData().AllTags()
	metadata["Filename"] = filepath.Base(filename)
	fileInfo, err := os.Stat(filename)
	if err != nil {
		panic(err)
	}
	metadata["File.Size"] = strconv.FormatInt(fileInfo.Size(), 10)
	metadata["File.ModifiedDate"] = fileInfo.ModTime().Format("2006-01-02 15.04.05")
	metadata["File.MD5Sum"], err = checksum(filename, "md5")
	if err != nil {
		panic(err)
	}
	metadata["File.Sha256Sum"], err = checksum(filename, "sha256")
	if err != nil {
		panic(err)
	}
	return metadata, nil
}

func GetVideoMetadata(filename string) (map[string]string, error) {
	et, err := exiftool.NewExiftool()
	if err != nil {
		panic(err)
	}
	defer et.Close()

	metadata := make(map[string]string)

	fileInfos := et.ExtractMetadata(filename)
	for _, fileInfo := range fileInfos {
		if fileInfo.Err != nil {
			fmt.Printf("Error concerning %v: %v\n", fileInfo.File, fileInfo.Err)
			continue
		}
		for key, value := range fileInfo.Fields {
			// metadata[k] = v
			// fmt.Printf("[%v] %v\n", k, v)
			switch v := value.(type) {
			case string:
				metadata[key] = v
			case int:
				metadata[key] = strconv.Itoa(v)
			case float64:
				metadata[key] = strconv.FormatFloat(v, 'f', -1, 64)
			case bool:
				metadata[key] = strconv.FormatBool(v)
			default:
				metadata[key] = fmt.Sprintf("<Unsupported field of type %s>", reflect.TypeOf(v))
			}
		}
	}

	fileInfo, err := os.Stat(filename)
	if err != nil {
		panic(err)
	}
	metadata["File.Size"] = strconv.FormatInt(fileInfo.Size(), 10)
	metadata["File.ModifiedDate"] = fileInfo.ModTime().Format("2006-01-02 16.04.05")
	metadata["File.MD5Sum"], err = checksum(filename, "md5")
	if err != nil {
		panic(err)
	}
	metadata["File.Sha256Sum"], err = checksum(filename, "sha256")
	if err != nil {
		panic(err)
	}

	return metadata, nil
}

func GetMetadata(filename string) (map[string]string, error) {
	if IsImage(filename) {
		return GetImageMetadata(filename)
	} else if IsVideo(filename) {
		return GetVideoMetadata(filename)
	} else {
		panic("Unsupported file type.")
	}
}

func PrintMetadata(metadata map[string]string) error {
	// fileInfo, err := os.Stat(metadata["Filename"])
	// if err != nil {
	// 	return err
	// }
	// metadata["File.Size"] = strconv.FormatInt(fileInfo.Size(), 10)
	width := 0
	keys := make([]string, 0, len(metadata))
	for key := range metadata {
		keys = append(keys, key)
		if len(key) > width {
			width = len(key)
		}
	}
	sort.Strings(keys)
	for _, key := range keys {
		fmt.Printf("%-*s: %s\n", width, key, metadata[key])
	}
	return nil
}

func main() {
	filename := os.Args[1]
	fmt.Println(filename)
	metadata, err := GetMetadata(filename)
	if err != nil {
		panic(err)
	}
	PrintMetadata(metadata)
}

func checksum(filename string, checksumFormat string) (string, error) {
	f, err := os.Open(filename)
	if err != nil {
		return "", err
	}
	defer f.Close()

	// Set the checksum function
	ChecksumFunctions := map[string]func() hash.Hash {
		"sha256": sha256.New,
		"md5":    md5.New,
	}
	h := ChecksumFunctions[checksumFormat]()

	// Get the file's checksum
	_, err = io.Copy(h, f)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

