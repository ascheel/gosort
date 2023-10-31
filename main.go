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
	var s Sort = Sort {
		dbFilename: "./gosort.db",
	}
	s.Init()
	var m Media = Media {
		filename_original: "/home/art/imagesort-py/images/samsung_phone/2023-08-25 20.17.19.jpg",
	}
	m.Init()
}
