package utils

import (
	"encoding/hex"
	"errors"
	"io"
	"math/rand"
	"os"
	"path/filepath"
)

func RandPath(prefix, suffix string) string {
	randBytes := make([]byte, 16)
	rand.Read(randBytes)

	return filepath.Join(os.TempDir(), prefix+hex.EncodeToString(randBytes)+suffix)
}

func IsDirEmpty(name string) (bool, error) {
	f, err := os.Open(name)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return true, nil
		}

		return false, err
	}
	defer f.Close()

	// read in ONLY one file
	_, err = f.Readdir(1)

	// and if the file is EOF... well, the dir is empty.
	if err == io.EOF {
		return true, nil
	}
	return false, err
}

func PathExists(filename string) bool {
	_, err := os.Lstat(filename)
	return err == nil
}
