//go:build integration

package livetest

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/bloberror"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/container"
)

// Container returns a container name unique to the test, under the
// Azurite account the BLOBFS_STORAGE_* variables name, and deletes the
// container when the test ends, whether or not anything created it. The
// test itself creates the container, through the store's start, so the
// test drives the same path the binary does. The deletion goes through
// the Azure SDK directly, because the storage library never deletes a
// container; this is the one place outside the storage adapter's own
// module that names the SDK, and it is test support.
func Container(t testing.TB) string {
	t.Helper()
	endpoint, account, key := os.Getenv("BLOBFS_STORAGE_ENDPOINT"), os.Getenv("BLOBFS_STORAGE_ACCOUNT"), os.Getenv("BLOBFS_STORAGE_KEY")
	if endpoint == "" || account == "" || key == "" {
		t.Fatal("BLOBFS_STORAGE_ENDPOINT, BLOBFS_STORAGE_ACCOUNT, and BLOBFS_STORAGE_KEY are not all set; run under mise with the compose stack up")
	}
	name := "blobfs-test-" + suffix(t)
	cred, err := container.NewSharedKeyCredential(account, key)
	if err != nil {
		t.Fatalf("shared key credential: %v", err)
	}
	client, err := container.NewClientWithSharedKeyCredential(endpoint+"/"+name, cred, nil)
	if err != nil {
		t.Fatalf("container client for %s: %v", name, err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if _, err := client.Delete(ctx, nil); err != nil && !bloberror.HasCode(err, bloberror.ContainerNotFound) {
			t.Errorf("delete container %s: %v", name, err)
		}
	})
	return name
}
