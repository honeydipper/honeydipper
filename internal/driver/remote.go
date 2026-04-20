// Copyright 2022 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

package driver

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// RemotePath is the root path where remote drivers are cached.
// It uses $HONEYDIPPER_DRIVERS_CACHE when set, otherwise /opt/honeydipper/drivers/cache.
var RemotePath string

var (
	errRemoteInvalidFileName    = errors.New("invalid fileName for remote driver")
	errRemoteInvalidDriverName  = errors.New("invalid driver name for remote driver")
	errRemoteHTTPStatus         = errors.New("unexpected http status while downloading remote driver")
	errRemoteSHA256Mismatch     = errors.New("sha256 mismatch for remote driver")
	errRemoteCacheLockCreate    = errors.New("failed creating cache lock directory")
	errRemoteCacheLockTimeout   = errors.New("timeout waiting for cache lock")
	errRemoteCacheDirCreate     = errors.New("failed creating remote driver cache directory")
	errRemoteAcquireDownload    = errors.New("failed acquiring remote driver")
	errRemoteDownloadRequest    = errors.New("failed creating remote driver download request")
	errRemoteDownloadHTTP       = errors.New("failed downloading remote driver")
	errRemoteDownloadOpenTemp   = errors.New("failed creating temp file for remote driver")
	errRemoteDownloadWriteTemp  = errors.New("failed writing temp file for remote driver")
	errRemoteDownloadCloseTemp  = errors.New("failed closing temp file for remote driver")
	errRemoteDownloadRenameTemp = errors.New("failed finalizing remote driver binary")
	errRemoteChecksumOpen       = errors.New("failed opening file for checksum")
	errRemoteChecksumRead       = errors.New("failed reading file for checksum")
)

func remotePath() string {
	if RemotePath == "" {
		driverPath, ok := os.LookupEnv("HONEYDIPPER_DRIVERS_CACHE")
		if ok {
			RemotePath = driverPath
		} else {
			RemotePath = "/opt/honeydipper/drivers/cache"
		}
	}

	return RemotePath
}

// RemoteDriver fetches and verifies a driver artifact, then executes it like a builtin driver.
type RemoteDriver struct {
	*BuiltinDriver
}

// NewRemoteDriver creates a handler for a remotely acquired driver.
func NewRemoteDriver(m *Meta) *RemoteDriver {
	return &RemoteDriver{BuiltinDriver: &BuiltinDriver{meta: m}}
}

// Acquire downloads and verifies the remote binary if it is not already available in cache.
func (d *RemoteDriver) Acquire() {
	rawURL, ok := d.meta.HandlerData["url"].(string)
	if !ok || rawURL == "" {
		panic(fmt.Errorf("%w: url is missing for remote driver: %s", ErrDriverError, d.meta.Name))
	}

	expectedSHA, ok := d.meta.HandlerData["sha256"].(string)
	if !ok || expectedSHA == "" {
		panic(fmt.Errorf("%w: sha256 is missing for remote driver: %s", ErrDriverError, d.meta.Name))
	}
	expectedSHA = strings.ToLower(expectedSHA)
	if _, err := hex.DecodeString(expectedSHA); err != nil || len(expectedSHA) != sha256.Size*2 {
		panic(fmt.Errorf("%w: invalid sha256 for remote driver: %s", ErrDriverError, d.meta.Name))
	}

	fileName, err := remoteFileName(d.meta.Name, rawURL, d.meta.HandlerData)
	if err != nil {
		panic(fmt.Errorf("%w: %w", ErrDriverError, err))
	}

	cacheDir := filepath.Join(remotePath(), "sha256", expectedSHA)
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		panic(fmt.Errorf("%w: %w: %w", ErrDriverError, errRemoteCacheDirCreate, err))
	}

	executablePath := filepath.Join(cacheDir, fileName)
	if hasValidCachedBinary(executablePath, expectedSHA) {
		d.meta.Executable = executablePath

		return
	}

	release := acquireDigestLock(cacheDir)
	defer release()

	if hasValidCachedBinary(executablePath, expectedSHA) {
		d.meta.Executable = executablePath

		return
	}

	if err := downloadAndVerify(rawURL, executablePath, expectedSHA); err != nil {
		panic(fmt.Errorf("%w: %w %s: %w", ErrDriverError, errRemoteAcquireDownload, d.meta.Name, err))
	}

	d.meta.Executable = executablePath
}

func remoteFileName(driverName string, rawURL string, handlerData map[string]interface{}) (string, error) {
	if fileName, ok := handlerData["fileName"].(string); ok && fileName != "" {
		if strings.ContainsRune(fileName, os.PathSeparator) || fileName == "." || fileName == ".." {
			return "", fmt.Errorf("%w: %s", errRemoteInvalidFileName, driverName)
		}

		return fileName, nil
	}

	parsedURL, err := url.Parse(rawURL)
	if err == nil {
		base := filepath.Base(parsedURL.Path)
		if base != "" && base != "." && base != "/" {
			return base, nil
		}
	}

	if strings.ContainsRune(driverName, os.PathSeparator) || driverName == "." || driverName == ".." {
		return "", fmt.Errorf("%w: %s", errRemoteInvalidDriverName, driverName)
	}

	return driverName, nil
}

func hasValidCachedBinary(binaryPath string, expectedSHA string) bool {
	info, err := os.Stat(binaryPath)
	if err != nil || !info.Mode().IsRegular() {
		return false
	}

	sha, err := checksumFile(binaryPath)
	if err != nil {
		return false
	}

	return sha == expectedSHA
}

func acquireDigestLock(cacheDir string) func() {
	lockPath := filepath.Join(cacheDir, ".acquire-lock")
	deadline := time.Now().Add(30 * time.Second)
	for {
		err := os.Mkdir(lockPath, 0o700)
		if err == nil {
			return func() {
				_ = os.Remove(lockPath)
			}
		}
		if !os.IsExist(err) {
			panic(fmt.Errorf("%w: %w: %w", ErrDriverError, errRemoteCacheLockCreate, err))
		}
		if time.Now().After(deadline) {
			panic(fmt.Errorf("%w: %w", ErrDriverError, errRemoteCacheLockTimeout))
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func downloadAndVerify(rawURL string, executablePath string, expectedSHA string) error {
	client := &http.Client{Timeout: 120 * time.Second}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, rawURL, nil)
	if err != nil {
		return fmt.Errorf("%w: %w", errRemoteDownloadRequest, err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %w", errRemoteDownloadHTTP, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%w: %d", errRemoteHTTPStatus, resp.StatusCode)
	}

	tmpPath := executablePath + ".tmp"
	tmpFile, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o700)
	if err != nil {
		return fmt.Errorf("%w: %w", errRemoteDownloadOpenTemp, err)
	}

	hash := sha256.New()
	if _, err := io.Copy(io.MultiWriter(tmpFile, hash), resp.Body); err != nil {
		_ = tmpFile.Close()
		_ = os.Remove(tmpPath)

		return fmt.Errorf("%w: %w", errRemoteDownloadWriteTemp, err)
	}
	if err := tmpFile.Close(); err != nil {
		_ = os.Remove(tmpPath)

		return fmt.Errorf("%w: %w", errRemoteDownloadCloseTemp, err)
	}

	actualSHA := hex.EncodeToString(hash.Sum(nil))
	if actualSHA != expectedSHA {
		_ = os.Remove(tmpPath)

		return errRemoteSHA256Mismatch
	}

	if err := os.Rename(tmpPath, executablePath); err != nil {
		_ = os.Remove(tmpPath)
		if hasValidCachedBinary(executablePath, expectedSHA) {
			return nil
		}

		return fmt.Errorf("%w: %w", errRemoteDownloadRenameTemp, err)
	}

	return nil
}

func checksumFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("%w: %w", errRemoteChecksumOpen, err)
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("%w: %w", errRemoteChecksumRead, err)
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}
