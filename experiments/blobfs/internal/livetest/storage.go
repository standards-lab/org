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
	name := "blobfs-test-" + suffix(t)
	client := containerClient(t, name)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if _, err := client.Delete(ctx, nil); err != nil && !bloberror.HasCode(err, bloberror.ContainerNotFound) {
			t.Errorf("delete container %s: %v", name, err)
		}
	})
	return name
}

// Blobs returns the names of the blobs the container holds, in the order
// the service lists them, and false when the container does not exist. A
// test that checks what a run stored, or that a container is gone, reads
// through it.
func Blobs(ctx context.Context, t testing.TB, name string) (blobs []string, exists bool) {
	t.Helper()
	pager := containerClient(t, name).NewListBlobsFlatPager(nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if bloberror.HasCode(err, bloberror.ContainerNotFound) {
			return nil, false
		}
		if err != nil {
			t.Fatalf("list blobs of %s: %v", name, err)
		}
		for _, item := range page.Segment.BlobItems {
			blobs = append(blobs, *item.Name)
		}
	}
	return blobs, true
}

// containerClient returns the SDK's client for the container named, under
// the account the BLOBFS_STORAGE_* variables name. Missing variables fail
// the test.
func containerClient(t testing.TB, name string) *container.Client {
	t.Helper()
	endpoint, account, key := os.Getenv("BLOBFS_STORAGE_ENDPOINT"), os.Getenv("BLOBFS_STORAGE_ACCOUNT"), os.Getenv("BLOBFS_STORAGE_KEY")
	if endpoint == "" || account == "" || key == "" {
		t.Fatal("BLOBFS_STORAGE_ENDPOINT, BLOBFS_STORAGE_ACCOUNT, and BLOBFS_STORAGE_KEY are not all set; run under mise with the compose stack up")
	}
	cred, err := container.NewSharedKeyCredential(account, key)
	if err != nil {
		t.Fatalf("shared key credential: %v", err)
	}
	client, err := container.NewClientWithSharedKeyCredential(endpoint+"/"+name, cred, nil)
	if err != nil {
		t.Fatalf("container client for %s: %v", name, err)
	}
	return client
}
