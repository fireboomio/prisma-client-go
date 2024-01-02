package binaries

import (
	"fmt"
	"log"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func tmpDir(t *testing.T) string {
	dir, err := os.MkdirTemp("/tmp", "prisma-client-go-test-fetchEngine-")
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestFetch(t *testing.T) {
	dir := tmpDir(t)
	//goland:noinspection GoUnhandledErrorResult
	defer os.RemoveAll(dir)

	if err := FetchNative(dir); err != nil {
		t.Fatalf("fetchEngine failed: %s", err)
	}
}

func TestFetch_localDir(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	if err := FetchNative(wd); err != nil {
		t.Fatalf("fetchEngine failed: %s", err)
	}
}

func TestFetch_withCache(t *testing.T) {
	dir := tmpDir(t)
	//goland:noinspection GoUnhandledErrorResult
	defer os.RemoveAll(dir)

	start := time.Now()
	if err := FetchNative(dir); err != nil {
		t.Fatalf("fetchEngine 1 failed: %s", err)
	}

	log.Printf("first fetchEngine took %s", time.Since(start))

	start = time.Now()
	if err := FetchNative(dir); err != nil {
		t.Fatalf("fetchEngine 2 failed: %s", err)
	}

	log.Printf("second fetchEngine took %s", time.Since(start))

	if time.Since(start) > 20*time.Millisecond {
		t.Fatalf("second fetchEngine took more than 10ms")
	}
}

func TestFetch_relativeDir(t *testing.T) {
	actual := FetchNative(".")
	expected := fmt.Errorf("toDir must be absolute")
	assert.Equal(t, expected, actual)
}

func TestDownloadURL(t *testing.T) {
	assert.Equal(t, "https://binaries.prisma.sh/all_commits/694eea289a8462c80264df36757e4fdc129b1b32/linux-musl/query-engine.gz", getDownLoadUrl("query-engine", "debian-openssl-3.2.x"))
	assert.Equal(t, "https://prisma-bin.fireboom.io/b3dba99ab5820f4e6b67e9220b80c89b71c2b5e6/linux-static-x64/schema-engine.gz", getDownLoadUrl("schema-engine", "debian-openssl-3.2.x"))

}
