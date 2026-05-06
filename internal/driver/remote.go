// Copyright 2022 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

package driver

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/honeydipper/honeydipper/v4/pkg/dipper"
)

// RemotePath is the root path where remote drivers are cached.
// It uses $HONEYDIPPER_DRIVERS_CACHE when set, otherwise /opt/honeydipper/drivers/cache.
var RemotePath string

var (
	errRemoteInvalidFileName    = errors.New("invalid fileName for remote driver")
	errRemoteInvalidDriverName  = errors.New("invalid driver name for remote driver")
	errRemoteURLMissing         = errors.New("url is missing for remote driver")
	errRemoteSHA256Missing      = errors.New("sha256 is missing for remote driver")
	errRemoteSHA256Invalid      = errors.New("invalid sha256 for remote driver")
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
	errRemoteSignatureRequired  = errors.New("signature is required for remote driver")
	errRemotePackageCheck       = errors.New("failed checking required package presence for remote driver")
	errRemotePackageInstall     = errors.New("failed installing required packages for remote driver")
	errRemoteRequiredPackages   = errors.New("invalid requiredPackages for remote driver")
	errRemoteInvalidPackageName = errors.New("invalid package name for remote driver")
	errRemoteMissingPackageSet  = errors.New("requiredPackages does not define packages for detected package manager")
	errRemoteNoPackageManager   = errors.New("no supported package manager found for remote driver")
	errRemoteRootRequired       = errors.New("root privileges are required to install required packages for remote driver")
	errRemotePublicKeyMissing   = errors.New("publicKey is missing for remote driver signature verification")
	errRemotePublicKeyInvalid   = errors.New("publicKey is invalid for remote driver signature verification")
	errRemoteSignatureMissing   = errors.New("signature is missing for remote driver")
	errRemoteSignatureInvalid   = errors.New("signature is invalid for remote driver")
	errRemoteSignatureVerify    = errors.New("failed verifying remote driver signature")
	errRemoteRegistryRequest    = errors.New("failed creating remote registry request")
	errRemoteRegistryFetch      = errors.New("failed fetching remote registry manifest")
	errRemoteRegistryDecode     = errors.New("failed decoding remote registry manifest")
	errRemoteRegistryVersion    = errors.New("failed resolving remote driver version from registry")
	errRemoteRegistryArtifact   = errors.New("failed resolving remote driver artifact from registry")
)

type remoteSignaturePolicy struct {
	required  bool
	publicKey []byte
	signature []byte
}

type remoteSource struct {
	rawURL      string
	expectedSHA string
	fileName    string
	sigPolicy   *remoteSignaturePolicy
	sourceType  string
}

type remoteRegistryManifest struct {
	Driver    string                           `json:"driver"`
	Latest    string                           `json:"latest"`
	PublicKey string                           `json:"publicKey"`
	Channels  map[string]string                `json:"channels"`
	Versions  map[string]remoteRegistryVersion `json:"versions"`
}

type remoteRegistryVersion struct {
	PublicKey string                   `json:"publicKey"`
	Artifacts []remoteRegistryArtifact `json:"artifacts"`
}

type remoteRegistryArtifact struct {
	OS        string `json:"os"`
	Arch      string `json:"arch"`
	URL       string `json:"url"`
	SHA256    string `json:"sha256"`
	FileName  string `json:"fileName"`
	PublicKey string `json:"publicKey"`
	Signature string `json:"signature"`
}

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
	source, err := resolveRemoteSource(d.meta.Name, d.meta.HandlerData)
	if err != nil {
		logRemoteAcquireDecision(d.meta.Name, source, false, "resolve_source_failed")
		panic(fmt.Errorf("%w: %w", ErrDriverError, err))
	}
	logRemoteAcquireDecision(d.meta.Name, source, true, "source_resolved")

	cacheDir := filepath.Join(remotePath(), "sha256", source.expectedSHA)
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		logRemoteAcquireDecision(d.meta.Name, source, false, "cache_dir_create_failed")
		panic(fmt.Errorf("%w: %w: %w", ErrDriverError, errRemoteCacheDirCreate, err))
	}

	executablePath := filepath.Join(cacheDir, source.fileName)
	if hasValidCachedBinary(executablePath, source.expectedSHA) {
		logRemoteAcquireDecision(d.meta.Name, source, true, "cache_hit")
		if err := verifyRemoteSignature(source.sigPolicy, source.expectedSHA); err != nil {
			logRemoteAcquireDecision(d.meta.Name, source, false, "cache_signature_verify_failed")
			panic(fmt.Errorf("%w: %w", ErrDriverError, err))
		}

		d.meta.Executable = executablePath
		d.installRequiredPackages()

		return
	}

	release := acquireDigestLock(cacheDir)
	defer release()

	if hasValidCachedBinary(executablePath, source.expectedSHA) {
		logRemoteAcquireDecision(d.meta.Name, source, true, "cache_hit_after_lock")
		if err := verifyRemoteSignature(source.sigPolicy, source.expectedSHA); err != nil {
			logRemoteAcquireDecision(d.meta.Name, source, false, "cache_signature_verify_failed")
			panic(fmt.Errorf("%w: %w", ErrDriverError, err))
		}

		d.meta.Executable = executablePath
		d.installRequiredPackages()

		return
	}

	logRemoteAcquireDecision(d.meta.Name, source, true, "cache_miss_download")
	if err := downloadAndVerify(source.rawURL, executablePath, source.expectedSHA, source.sigPolicy); err != nil {
		logRemoteAcquireDecision(d.meta.Name, source, false, "download_or_verify_failed")
		panic(fmt.Errorf("%w: %w %s: %w", ErrDriverError, errRemoteAcquireDownload, d.meta.Name, err))
	}
	logRemoteAcquireDecision(d.meta.Name, source, true, "download_verified")

	d.meta.Executable = executablePath
	d.installRequiredPackages()
}

func (d *RemoteDriver) installRequiredPackages() {
	raw, ok := d.meta.HandlerData["requiredPackages"]
	if !ok {
		return
	}

	manager, binary, installArgs, err := resolvePackageInstaller(exec.LookPath)
	if err != nil {
		panic(fmt.Errorf("%w: %w: %s", ErrDriverError, errRemoteNoPackageManager, d.meta.Name))
	}

	pkgs, err := resolveRequiredPackagesForManager(raw, manager)
	if err != nil {
		panic(fmt.Errorf("%w: %w: %w", ErrDriverError, errRemoteRequiredPackages, err))
	}
	if len(pkgs) == 0 {
		return
	}

	ctx := context.Background()
	missingPkgs, err := filterMissingPackages(ctx, manager, pkgs, runCommand)
	if err != nil {
		panic(fmt.Errorf("%w: %w %s: %w", ErrDriverError, errRemotePackageCheck, d.meta.Name, err))
	}
	if len(missingPkgs) == 0 {
		dipper.Logger.Infof("[remote-driver] required packages already present for %s with %s: %v", d.meta.Name, manager, pkgs)

		return
	}

	cmdName, cmdArgs, err := resolveInstallInvocation(binary, installArgs, missingPkgs, exec.LookPath, os.Geteuid)
	if err != nil {
		panic(fmt.Errorf("%w: %w: %s", ErrDriverError, err, d.meta.Name))
	}

	cmd := exec.CommandContext(ctx, cmdName, cmdArgs...)

	dipper.Logger.Infof("[remote-driver] installing required packages for %s with %s: %v", d.meta.Name, manager, missingPkgs)
	if out, err := cmd.CombinedOutput(); err != nil {
		panic(fmt.Errorf("%w: %w %s: %s", ErrDriverError, errRemotePackageInstall, d.meta.Name, string(out)))
	}
}

func runCommand(ctx context.Context, name string, args ...string) error {
	return exec.CommandContext(ctx, name, args...).Run() //nolint:wrapcheck
}

func resolvePackageInstaller(lookPath func(file string) (string, error)) (string, string, []string, error) {
	if _, err := lookPath("apk"); err == nil {
		return "apk", "apk", []string{"add", "--no-cache"}, nil
	}
	if _, err := lookPath("apt-get"); err == nil {
		return "apt", "apt-get", []string{"install", "-y"}, nil
	}
	if _, err := lookPath("apt"); err == nil {
		return "apt", "apt", []string{"install", "-y"}, nil
	}
	if _, err := lookPath("dnf"); err == nil {
		return "dnf", "dnf", []string{"install", "-y"}, nil
	}
	if _, err := lookPath("brew"); err == nil {
		return "brew", "brew", []string{"install"}, nil
	}

	return "", "", nil, errRemoteNoPackageManager
}

func resolveRequiredPackagesForManager(raw interface{}, manager string) ([]string, error) {
	if rawList, ok := raw.([]interface{}); ok {
		return parsePackageList(rawList)
	}

	byManager, ok := raw.(map[string]interface{})
	if !ok {
		return nil, errRemoteRequiredPackages
	}

	keys := []string{manager}
	if manager == "apt" {
		keys = append(keys, "apt-get")
	}
	for _, key := range keys {
		rawList, exists := byManager[key]
		if !exists {
			continue
		}

		pkgs, err := parsePackageList(rawList)
		if err != nil {
			return nil, err
		}

		return pkgs, nil
	}

	return nil, errRemoteMissingPackageSet
}

func parsePackageList(rawList interface{}) ([]string, error) {
	if list, ok := rawList.([]string); ok {
		pkgs := make([]string, 0, len(list))
		for _, name := range list {
			if !isValidPackageName(name) {
				return nil, fmt.Errorf("%w: %v", errRemoteInvalidPackageName, name)
			}
			pkgs = append(pkgs, name)
		}

		return pkgs, nil
	}

	list, ok := rawList.([]interface{})
	if !ok {
		return nil, errRemoteRequiredPackages
	}

	pkgs := make([]string, 0, len(list))
	for _, p := range list {
		name, ok := p.(string)
		if !ok || !isValidPackageName(name) {
			return nil, fmt.Errorf("%w: %v", errRemoteInvalidPackageName, p)
		}
		pkgs = append(pkgs, name)
	}

	return pkgs, nil
}

func filterMissingPackages(
	ctx context.Context,
	manager string,
	pkgs []string,
	runner func(context.Context, string, ...string) error,
) ([]string, error) {
	missing := make([]string, 0, len(pkgs))
	for _, pkg := range pkgs {
		checkCmd, checkArgs, err := packageCheckCommand(manager, pkg)
		if err != nil {
			return nil, err
		}

		if err := runner(ctx, checkCmd, checkArgs...); err != nil {
			missing = append(missing, pkg)
		}
	}

	return missing, nil
}

func packageCheckCommand(manager string, pkg string) (string, []string, error) {
	switch manager {
	case "apk":
		return "apk", []string{"info", "-e", pkg}, nil
	case "apt":
		return "dpkg", []string{"-s", pkg}, nil
	case "dnf":
		return "dnf", []string{"list", "installed", pkg}, nil
	case "brew":
		return "brew", []string{"list", "--formula", pkg}, nil
	default:
		return "", nil, errRemoteNoPackageManager
	}
}

func resolveInstallInvocation(
	binary string,
	installArgs []string,
	pkgs []string,
	lookPath func(file string) (string, error),
	geteuid func() int,
) (string, []string, error) {
	args := append(append([]string{}, installArgs...), pkgs...)
	if geteuid() == 0 {
		return binary, args, nil
	}

	if _, err := lookPath("sudo"); err == nil {
		return "sudo", append([]string{"-n", binary}, args...), nil
	}

	return "", nil, errRemoteRootRequired
}

func isValidPackageName(name string) bool {
	if len(name) == 0 {
		return false
	}
	for i, c := range name {
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
			// always valid
		case c == '-' || c == '_' || c == '.' || c == '+':
			if i == 0 {
				return false // must start with alphanumeric
			}
		default:
			return false
		}
	}

	return true
}

func resolveRemoteSource(driverName string, handlerData map[string]interface{}) (*remoteSource, error) {
	rawURL, _ := handlerData["url"].(string)
	if rawURL != "" {
		return resolveDirectRemoteSource(driverName, handlerData, rawURL)
	}

	registryURL, _ := handlerData["registryURL"].(string)
	if registryURL == "" {
		return nil, fmt.Errorf("%w: %s", errRemoteURLMissing, driverName)
	}

	return resolveRegistryRemoteSource(driverName, handlerData, registryURL)
}

func resolveDirectRemoteSource(driverName string, handlerData map[string]interface{}, rawURL string) (*remoteSource, error) {
	expectedSHA, ok := handlerData["sha256"].(string)
	if !ok || expectedSHA == "" {
		return nil, fmt.Errorf("%w: %s", errRemoteSHA256Missing, driverName)
	}
	expectedSHA = strings.ToLower(expectedSHA)
	if _, err := hex.DecodeString(expectedSHA); err != nil || len(expectedSHA) != sha256.Size*2 {
		return nil, fmt.Errorf("%w: %s", errRemoteSHA256Invalid, driverName)
	}

	fileName, err := remoteFileName(driverName, rawURL, handlerData)
	if err != nil {
		return nil, err
	}

	sigPolicy, err := parseRemoteSignaturePolicy(handlerData)
	if err != nil {
		return nil, err
	}

	return &remoteSource{
		rawURL:      rawURL,
		expectedSHA: expectedSHA,
		fileName:    fileName,
		sigPolicy:   sigPolicy,
		sourceType:  "direct",
	}, nil
}

func resolveRegistryRemoteSource(driverName string, handlerData map[string]interface{}, registryURL string) (*remoteSource, error) {
	manifest, err := fetchRemoteRegistryManifest(registryURL, driverName)
	if err != nil {
		return nil, err
	}

	version, err := resolveRemoteRegistryVersion(handlerData, manifest)
	if err != nil {
		return nil, err
	}

	artifact, err := resolveRemoteRegistryArtifact(version, manifest)
	if err != nil {
		return nil, err
	}

	resolved := map[string]interface{}{
		"url":    artifact.URL,
		"sha256": artifact.SHA256,
	}
	if artifact.FileName != "" {
		resolved["fileName"] = artifact.FileName
	}
	if requireSignature, ok := handlerData["requireSignature"].(bool); ok {
		resolved["requireSignature"] = requireSignature
	}
	if artifact.PublicKey != "" {
		resolved["publicKey"] = artifact.PublicKey
		if manifestVersion, ok := manifest.Versions[version]; ok && manifestVersion.PublicKey != "" {
			resolved["publicKey"] = artifact.PublicKey
		}
	}
	if resolved["publicKey"] == nil {
		if manifestVersion, ok := manifest.Versions[version]; ok && manifestVersion.PublicKey != "" {
			resolved["publicKey"] = manifestVersion.PublicKey
		} else if manifest.PublicKey != "" {
			resolved["publicKey"] = manifest.PublicKey
		}
	}
	if artifact.Signature != "" {
		resolved["signature"] = artifact.Signature
	}

	source, err := resolveDirectRemoteSource(driverName, resolved, artifact.URL)
	if err != nil {
		return nil, err
	}
	source.sourceType = "registry"

	return source, nil
}

func logRemoteAcquireDecision(driverName string, source *remoteSource, allowed bool, reason string) {
	if dipper.Logger == nil {
		return
	}

	sourceType := "unknown"
	sha := "unknown"
	if source != nil {
		if source.sourceType != "" {
			sourceType = source.sourceType
		}
		if source.expectedSHA != "" {
			sha = source.expectedSHA
		}
	}

	if allowed {
		dipper.Logger.Infof(
			"[remote-driver] acquire_decision driver=%s source=%s allowed=true reason=%s sha256=%s",
			driverName,
			sourceType,
			reason,
			sha,
		)

		return
	}

	dipper.Logger.Warningf(
		"[remote-driver] acquire_decision driver=%s source=%s allowed=false reason=%s sha256=%s",
		driverName,
		sourceType,
		reason,
		sha,
	)
}

func fetchRemoteRegistryManifest(registryURL string, driverName string) (*remoteRegistryManifest, error) {
	manifestURL, err := url.JoinPath(registryURL, driverName+".json")
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errRemoteRegistryRequest, err)
	}

	client := &http.Client{Timeout: 120 * time.Second}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, manifestURL, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errRemoteRegistryRequest, err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errRemoteRegistryFetch, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: %w: %d", errRemoteRegistryFetch, errRemoteHTTPStatus, resp.StatusCode)
	}

	var manifest remoteRegistryManifest
	if err := json.NewDecoder(resp.Body).Decode(&manifest); err != nil {
		return nil, fmt.Errorf("%w: %w", errRemoteRegistryDecode, err)
	}

	return &manifest, nil
}

func resolveRemoteRegistryVersion(handlerData map[string]interface{}, manifest *remoteRegistryManifest) (string, error) {
	version, _ := handlerData["version"].(string)
	if version != "" {
		if _, ok := manifest.Versions[version]; ok {
			return version, nil
		}

		return "", fmt.Errorf("%w: version %s not found", errRemoteRegistryVersion, version)
	}

	channel := "stable"
	if configuredChannel, ok := handlerData["channel"].(string); ok && configuredChannel != "" {
		channel = configuredChannel
	}
	if resolvedVersion, ok := manifest.Channels[channel]; ok {
		if _, ok := manifest.Versions[resolvedVersion]; ok {
			return resolvedVersion, nil
		}
	}
	if manifest.Latest != "" {
		if _, ok := manifest.Versions[manifest.Latest]; ok {
			return manifest.Latest, nil
		}
	}

	return "", fmt.Errorf("%w: no version resolved for channel %s", errRemoteRegistryVersion, channel)
}

func resolveRemoteRegistryArtifact(version string, manifest *remoteRegistryManifest) (*remoteRegistryArtifact, error) {
	resolvedVersion := manifest.Versions[version]
	for _, artifact := range resolvedVersion.Artifacts {
		if artifact.OS == runtime.GOOS && artifact.Arch == runtime.GOARCH {
			if artifact.URL == "" || artifact.SHA256 == "" {
				return nil, fmt.Errorf("%w: missing url or sha256 for %s/%s", errRemoteRegistryArtifact, artifact.OS, artifact.Arch)
			}

			return &artifact, nil
		}
	}

	return nil, fmt.Errorf("%w: no artifact for %s/%s in version %s", errRemoteRegistryArtifact, runtime.GOOS, runtime.GOARCH, version)
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

func parseRemoteSignaturePolicy(handlerData map[string]interface{}) (*remoteSignaturePolicy, error) {
	requireSignature := false
	if v, ok := os.LookupEnv("HONEYDIPPER_REMOTE_REQUIRE_SIGNATURE"); ok {
		parsed, err := strconv.ParseBool(v)
		if err == nil {
			requireSignature = parsed
		}
	}
	if v, ok := handlerData["requireSignature"].(bool); ok {
		requireSignature = v
	}

	publicKeyB64, _ := handlerData["publicKey"].(string)
	signatureB64, _ := handlerData["signature"].(string)
	if publicKeyB64 == "" && signatureB64 == "" && !requireSignature {
		return &remoteSignaturePolicy{}, nil
	}
	if publicKeyB64 == "" {
		if requireSignature {
			return nil, errRemotePublicKeyMissing
		}

		return &remoteSignaturePolicy{}, nil
	}
	if signatureB64 == "" {
		return nil, errRemoteSignatureMissing
	}

	publicKey, err := base64.StdEncoding.DecodeString(publicKeyB64)
	if err != nil || len(publicKey) != ed25519.PublicKeySize {
		return nil, errRemotePublicKeyInvalid
	}

	signature, err := base64.StdEncoding.DecodeString(signatureB64)
	if err != nil || len(signature) != ed25519.SignatureSize {
		return nil, errRemoteSignatureInvalid
	}

	return &remoteSignaturePolicy{
		required:  requireSignature,
		publicKey: publicKey,
		signature: signature,
	}, nil
}

func verifyRemoteSignature(policy *remoteSignaturePolicy, expectedSHA string) error {
	if policy == nil {
		return nil
	}
	if len(policy.publicKey) == 0 && len(policy.signature) == 0 {
		if policy.required {
			return errRemoteSignatureRequired
		}

		return nil
	}
	if len(policy.publicKey) == 0 {
		return errRemotePublicKeyMissing
	}
	if len(policy.signature) == 0 {
		return errRemoteSignatureMissing
	}

	digest, err := hex.DecodeString(expectedSHA)
	if err != nil {
		return errRemoteSignatureVerify
	}
	if !ed25519.Verify(policy.publicKey, digest, policy.signature) {
		return errRemoteSignatureVerify
	}

	return nil
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

func downloadAndVerify(rawURL string, executablePath string, expectedSHA string, sigPolicy *remoteSignaturePolicy) error {
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
	if err := verifyRemoteSignature(sigPolicy, actualSHA); err != nil {
		_ = os.Remove(tmpPath)

		return fmt.Errorf("%w: %w", errRemoteSignatureVerify, err)
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
