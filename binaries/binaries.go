package binaries

import (
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/prisma/prisma-client-go/binaries/platform"
	"github.com/prisma/prisma-client-go/logger"
)

// PrismaVersion is a hardcoded version of the Prisma CLI.
// const PrismaVersion = "3.13.0"
var PrismaVersion = "3.13.0"

// EngineVersion is a hardcoded version of the Prisma Engine.
// The versions can be found under https://github.com/prisma/prisma-engines/commits/master
// const EngineVersion = "694eea289a8462c80264df36757e4fdc129b1b32"
var QueryEngineVersion = "58b76d24c10d06ee3aba2c8f1e5cbe75db073d3c"  // 用于指定 query-engine 的版本
var SchemaEngineVersion = "58b76d24c10d06ee3aba2c8f1e5cbe75db073d3c" // 用于指定 schema-engine 的版本

// PrismaURL points to an S3 bucket URL where the CLI binaries are stored.
var PrismaURL = "https://prisma-photongo.s3-eu-west-1.amazonaws.com/%s-%s-%s-x64.gz"

// EngineURL points to an S3 bucket URL where the Prisma engines are stored.
var queryEngineURL = "https://binaries.prisma.sh/all_commits/%s/%s/%s.gz" // queryEngine 下载链接; https://github.com/steebchen/prisma-client-go/issues/1107
var scehmaEngineURL = "https://prisma-bin.fireboom.io/%s/%s/%s.gz"

const (
	QueryEngineName  = "query-engine"
	SchemaEngineName = "schema-engine"
)

type Engine struct {
	Name string
	Env  string
}

var Engines = []Engine{{
	QueryEngineName,
	"PRISMA_QUERY_ENGINE_BINARY",
}, {
	SchemaEngineName,
	"PRISMA_INTROSPECTION_ENGINE_BINARY",
}}

// init overrides URLs if env variables are specific for debugging purposes and to
// be able to provide a fallback if the links above should go down
func init() {
	if prismaURL, ok := os.LookupEnv("PRISMA_CLI_URL"); ok {
		PrismaURL = prismaURL
	}
	if engineURL, ok := os.LookupEnv("PRISMA_ENGINE_URL"); ok {
		queryEngineURL = engineURL
	}
}

func getDownLoadUrl(engineName, binaryName string) (res string) {
	if strings.Contains(binaryName, "debian-openssl-") { // TODO 先临时处理下，后续迁移 schema-engine 后统一处理
		switch engineName {
		case SchemaEngineName:
			return fmt.Sprintf(scehmaEngineURL, SchemaEngineVersion, "linux-static-x64", engineName)
		case QueryEngineName:
			return fmt.Sprintf(queryEngineURL, QueryEngineVersion, "linux-musl", engineName)
		default:
			logger.Info.Printf("can not get download url with engineName = %s", engineName)
			return ""
		}
	}

	switch engineName {
	case SchemaEngineName:
		return fmt.Sprintf(scehmaEngineURL, SchemaEngineVersion, binaryName, engineName)
	case QueryEngineName:
		return fmt.Sprintf(queryEngineURL, QueryEngineVersion, binaryName, engineName)
	default:
		logger.Info.Printf("can not get download url with engineName = %s", engineName)
		return ""
	}
}

// PrismaCLIName returns the local file path of where the CLI lives
func PrismaCLIName() string {
	variation := platform.Name()
	return fmt.Sprintf("prisma-cli-%s-x64", variation)
}

var baseDirName = filepath.Join("prisma", "binaries")

// GlobalTempDir returns the path of where the engines live
// internally, this is the global temp dir
func GlobalTempDir(version string) string {
	temp := os.TempDir()
	logger.Debug.Printf("temp dir: %s", temp)

	return filepath.ToSlash(filepath.Join(temp, baseDirName, "engines", version))
}

func GlobalUnpackDir(version string) string {
	return filepath.ToSlash(filepath.Join(GlobalTempDir(version), "unpacked", "v2"))
}

// GlobalCacheDir returns the path of where the CLI lives
// internally, this is the global temp dir
func GlobalCacheDir() string {
	cache, err := os.UserCacheDir()
	if err != nil {
		panic(fmt.Errorf("could not read user cache dir: %w", err))
	}

	logger.Debug.Printf("global cache dir: %s", cache)

	return filepath.ToSlash(filepath.Join(cache, baseDirName, "cli", PrismaVersion))
}

func FetchEngine(toDir string, engineName string, binaryPlatformName string) error {
	logger.Debug.Printf("checking %s...", engineName)

	to := GetEnginePath(toDir, engineName, binaryPlatformName)
	binaryPlatformRemoteName := binaryPlatformName
	if binaryPlatformRemoteName == "linux" {
		binaryPlatformRemoteName = "linux-musl"
	}
	url := platform.CheckForExtension(binaryPlatformName, getDownLoadUrl(engineName, binaryPlatformRemoteName))

	logger.Debug.Printf("download url %s", url)

	if _, err := os.Stat(to); !os.IsNotExist(err) {
		logger.Debug.Printf("%s is cached", to)
		return nil
	}

	logger.Debug.Printf("%s is missing, downloading...", engineName)

	if err := download(url, to); err != nil {
		return fmt.Errorf("could not download %s to %s: %w", url, to, err)
	}

	logger.Debug.Printf("%s done", engineName)

	return nil
}

// FetchNative fetches the Prisma binaries needed for the generator to a given directory
func FetchNative(toDir string) error {
	if toDir == "" {
		return fmt.Errorf("toDir must be provided")
	}

	if !filepath.IsAbs(toDir) {
		return fmt.Errorf("toDir must be absolute")
	}

	// if err := DownloadCLI(toDir); err != nil {
	// 	return fmt.Errorf("could not download engines: %w", err)
	// }

	for _, e := range Engines {
		if _, err := DownloadEngine(e.Name, toDir); err != nil {
			return fmt.Errorf("could not download engines: %w", err)
		}
	}

	return nil
}

func DownloadCLI(toDir string) error {
	cli := PrismaCLIName()
	to := platform.CheckForExtension(platform.Name(), filepath.ToSlash(filepath.Join(toDir, cli)))
	url := platform.CheckForExtension(platform.Name(), fmt.Sprintf(PrismaURL, "prisma-cli", PrismaVersion, platform.Name()))

	logger.Debug.Printf("ensuring CLI %s from %s to %s", cli, to, url)

	if _, err := os.Stat(to); os.IsNotExist(err) {
		logger.Info.Printf("prisma cli doesn't exist, fetching... (this might take a few minutes)")

		if err := download(url, to); err != nil {
			return fmt.Errorf("could not download %s to %s: %w", url, to, err)
		}

		logger.Info.Printf("prisma cli fetched successfully.")
	} else {
		logger.Debug.Printf("prisma cli is cached")
	}

	return nil
}

func GetEnginePath(dir, engine, binaryName string) string { //本地存储地址
	switch engine {
	case SchemaEngineName:
		return platform.CheckForExtension(binaryName, filepath.ToSlash(filepath.Join(dir, SchemaEngineVersion, fmt.Sprintf("prisma-%s-%s", engine, binaryName))))
	case QueryEngineName:
		return platform.CheckForExtension(binaryName, filepath.ToSlash(filepath.Join(dir, QueryEngineVersion, fmt.Sprintf("prisma-%s-%s", engine, binaryName))))
	default:
		logger.Info.Printf("can not get local path with engineName = %s", engine)
		return ""
	}
}

func GetEnginePathWithVersion(dir, engine, binaryName, scehemaEngineVersion string) string { //本地存储地址
	switch engine {
	case SchemaEngineName:
		return platform.CheckForExtension(binaryName, filepath.ToSlash(filepath.Join(dir, scehemaEngineVersion, fmt.Sprintf("prisma-%s-%s", engine, binaryName))))
	case QueryEngineName:
		return platform.CheckForExtension(binaryName, filepath.ToSlash(filepath.Join(dir, QueryEngineVersion, fmt.Sprintf("prisma-%s-%s", engine, binaryName))))
	default:
		logger.Info.Printf("can not get local path with engineName = %s", engine)
		return ""
	}
}

func DownloadEngine(name string, toDir string) (file string, err error) {
	binaryName := platform.BinaryPlatformName()

	logger.Debug.Printf("checking %s...", name)

	to := GetEnginePath(toDir, name, binaryName)

	url := platform.CheckForExtension(binaryName, getDownLoadUrl(name, binaryName))

	logger.Debug.Printf("download url %s", url)

	if _, err := os.Stat(to); !os.IsNotExist(err) {
		logger.Debug.Printf("%s is cached", to)
		return to, nil
	}

	logger.Debug.Printf("%s is missing, downloading...", name)

	startDownload := time.Now()
	if err := download(url, to); err != nil {
		return "", fmt.Errorf("could not download %s to %s: %w", url, to, err)
	}

	logger.Debug.Printf("download() took %s", time.Since(startDownload))

	logger.Debug.Printf("%s done", name)

	return to, nil
}

func download(url string, to string) error {
	if err := os.MkdirAll(path.Dir(to), os.ModePerm); err != nil {
		return fmt.Errorf("could not run MkdirAll on path %s: %w", to, err)
	}

	// copy to temp file first
	dest := to + ".tmp"

	resp, err := http.Get(url) //nolint:gosec
	if err != nil {
		return fmt.Errorf("could not get %s: %w", url, err)
	}
	//goland:noinspection GoUnhandledErrorResult
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		out, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("received code %d from %s: %+v", resp.StatusCode, url, string(out))
	}

	out, err := os.Create(dest)
	if err != nil {
		return fmt.Errorf("could not create %s: %w", dest, err)
	}
	//goland:noinspection GoUnhandledErrorResult
	defer out.Close()

	if err := os.Chmod(dest, os.ModePerm); err != nil {
		return fmt.Errorf("could not chmod +x %s: %w", url, err)
	}

	g, err := gzip.NewReader(resp.Body)
	if err != nil {
		return fmt.Errorf("could not create gzip reader: %w", err)
	}
	//goland:noinspection GoUnhandledErrorResult
	defer g.Close()

	if _, err := io.Copy(out, g); err != nil { //nolint:gosec
		return fmt.Errorf("could not copy %s: %w", url, err)
	}

	// temp file is ready, now copy to the original destination
	if err := copyFile(dest, to); err != nil {
		return fmt.Errorf("copy temp file: %w", err)
	}

	return nil
}

func copyFile(from string, to string) error {
	input, err := os.ReadFile(from)
	if err != nil {
		return fmt.Errorf("readfile: %w", err)
	}

	if err := os.WriteFile(to, input, os.ModePerm); err != nil {
		return fmt.Errorf("writefile: %w", err)
	}

	return nil
}

// ----------------------- 自定义 fireboom 下载逻辑 -------------
var engineNames = []string{QueryEngineName, SchemaEngineName}

const downloadURL = "https://prisma-bin.fireboom.io/%s/%s.gz"

type FetchNativeRes struct {
	SchemaEnginePath string
	QueryEnginePath  string
}

func (f *FetchNativeRes) setPath(engineName, enginePath string) {
	switch engineName {
	case QueryEngineName:
		f.QueryEnginePath = enginePath
	case SchemaEngineName:
		f.SchemaEnginePath = enginePath
	}
}

// FetchNativeWithVersion 用于 fireboom 下载指定版本的 Prisma Binaries
func FetchNativeWithVersion(toDir, version string) (FetchNativeRes, error) {
	res := FetchNativeRes{}
	if toDir == "" {
		return res, fmt.Errorf("toDir must be provided")
	}
	if !filepath.IsAbs(toDir) {
		return res, fmt.Errorf("toDir must be absolute")
	}

	for _, engineName := range engineNames {
		enginePath, err := downloadEngine(toDir, version, engineName)
		if err != nil {
			return res, fmt.Errorf("could not download engines: %w", err)
		}

		res.setPath(engineName, enginePath)
	}

	return res, nil
}

func downloadEngine(toDir, version, engineName string) (file string, err error) {
	binaryName := binaryName(engineName)
	to := getEnginePath(toDir, version, binaryName)
	url := downLoadUrl(version, binaryName)
	logger.Debug.Printf("downloading %s to %s", url, to)

	if _, err := os.Stat(to); !os.IsNotExist(err) {
		logger.Debug.Printf("%s is cached", to)
		return to, nil
	}

	logger.Debug.Printf("%s is missing, downloading...", binaryName)
	startDownload := time.Now()
	if err := download(url, to); err != nil {
		return "", fmt.Errorf("could not download %s to %s: %w", url, to, err)
	}

	logger.Debug.Printf("download() took %s", time.Since(startDownload))

	logger.Debug.Printf("%s done", binaryName)

	return to, nil
}

func getEnginePath(dir, version, binaryName string) string { //本地存储地址
	return filepath.ToSlash(filepath.Join(dir, version, binaryName))
}

// binaryName 二进制文件名
func binaryName(engineName string) string {

	platform, arch := platform.Name(), platform.Arch()
	res := fmt.Sprintf("%s-%s-%s", platform, arch, engineName)
	if platform == "windows" {
		return fmt.Sprintf("%s.exe", res)
	}

	return res
}

func downLoadUrl(version, binaryName string) (res string) {
	return fmt.Sprintf(downloadURL, version, binaryName)
}
