package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"internal/media"
)

var (
	Version string
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

func GetMetadata(filename string) (map[string]string) {
	return media.NewMediaFile(filename).Metadata
}

func PrintMetadata(metadata map[string]string) error {
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
	metadata := GetMetadata(filename)
	PrintMetadata(metadata)
}

