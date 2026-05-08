package main

import (
	"crypto/sha256"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
)

func sha256Command(arguments []string) int {
	flags := flag.NewFlagSet("sha256", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	path := flags.String("file", "", "regular file to hash; omit to hash standard input")
	if err := flags.Parse(arguments); err != nil {
		return commandError(err)
	}
	if flags.NArg() != 0 {
		return commandError(errors.New("sha256 accepts only -file"))
	}
	var digest string
	var err error
	if *path == "" {
		digest, err = sha256Reader(os.Stdin)
	} else {
		digest, err = sha256StableFile(*path, nil)
	}
	if err != nil {
		return commandError(err)
	}
	fmt.Println(digest)
	return 0
}

func sha256StableFile(path string, afterFirst func()) (string, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return "", fmt.Errorf("inspect SHA-256 input %q: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("SHA-256 input %q is not a regular file", path)
	}
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open SHA-256 input %q: %w", path, err)
	}
	defer file.Close()
	first, err := sha256Reader(file)
	if err != nil {
		return "", fmt.Errorf("hash SHA-256 input %q: %w", path, err)
	}
	if afterFirst != nil {
		afterFirst()
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return "", fmt.Errorf("rewind SHA-256 input %q: %w", path, err)
	}
	second, err := sha256Reader(file)
	if err != nil {
		return "", fmt.Errorf("rehash SHA-256 input %q: %w", path, err)
	}
	after, err := file.Stat()
	if err != nil {
		return "", fmt.Errorf("reinspect SHA-256 input %q: %w", path, err)
	}
	if first != second || info.Size() != after.Size() || !info.ModTime().Equal(after.ModTime()) {
		return "", fmt.Errorf("SHA-256 input %q changed while hashing", path)
	}
	return first, nil
}

func sha256Reader(reader io.Reader) (string, error) {
	digest := sha256.New()
	if _, err := io.Copy(digest, reader); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", digest.Sum(nil)), nil
}
