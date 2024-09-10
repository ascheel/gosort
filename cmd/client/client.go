package main

// This application is the client for the GoSort application.
// It will send images and videos to the GoSort API for sorting.

import (
	"crypto/md5"
	"fmt"
	"io"
	"os"
	"path/filepath"

	m "github.com/ascheel/gosort/internal/sortengine"
	//"github.com/veandco/go-sdl2/img"
)

var (
	Version string
)

type Send struct {
	Directory string
	Host string
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
	img := m.NewMediaFile(info.Name())
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

	dir := os.Args[1]
	WalkDir(dir)
}
