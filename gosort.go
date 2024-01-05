package main

import (
	"os"
	"crypto/sha256"
	"fmt"
	//"log"
	"io"
)

func sha256sum(filename string) (string, error) {
	f, err := os.Open(filename)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	_, err = io.Copy(h, f)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

func fileExists(filename string) bool {
	if _, err := os.Stat(filename); err == nil {
		return true
	} else {
		return false
	}
}

func main() {
	imageFile := "omelette.jpg"
	image := NewMediaFile(imageFile)
	
	//image.GetMetadata()
	image.Print()
	os.Exit(0)
	sort := NewSort("./gosort.db")
	sort.Scan()
}
