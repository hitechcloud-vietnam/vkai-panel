package backup

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func readDirNames(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names, nil
}

func TestLocalDestinationRoundTrip(t *testing.T) {
	root := t.TempDir()
	dest, err := NewLocalDestination(root)
	if err != nil {
		t.Fatalf("NewLocalDestination: %v", err)
	}

	ctx := context.Background()
	payload := bytes.Repeat([]byte("local archive "), 1000)
	key := "tenant-a/website/shop/20250101T000000Z-shop.vkab"

	info, err := dest.Put(ctx, key, bytes.NewReader(payload), int64(len(payload)))
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if info.Size != int64(len(payload)) {
		t.Fatalf("Put reported %d bytes", info.Size)
	}

	rc, err := dest.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	got, _ := io.ReadAll(rc)
	_ = rc.Close()
	if !bytes.Equal(got, payload) {
		t.Fatal("the object read back is not the object written")
	}

	objects, err := dest.List(ctx, "tenant-a/website/")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(objects) != 1 || objects[0].Key != key {
		t.Fatalf("List returned %+v", objects)
	}

	if err := dest.Delete(ctx, key); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if err := dest.Delete(ctx, key); err != nil {
		t.Fatalf("deleting twice must be quiet, got %v", err)
	}
	if _, err := dest.Stat(ctx, key); err == nil {
		t.Fatal("the object still exists after Delete")
	}
}

func TestLocalDestinationIsAtomic(t *testing.T) {
	root := t.TempDir()
	dest, err := NewLocalDestination(root)
	if err != nil {
		t.Fatalf("NewLocalDestination: %v", err)
	}

	// A reader that fails half way is the shape of a disk filling up or a
	// source going away. Nothing must be published under the object's name.
	failing := io.MultiReader(strings.NewReader("half an archive"), &failingReader{})
	if _, err := dest.Put(context.Background(), "tenant/files/x/y", failing, -1); err == nil {
		t.Fatal("Put succeeded with a failing reader")
	}

	if _, err := os.Stat(filepath.Join(root, "tenant/files/x/y")); !os.IsNotExist(err) {
		t.Fatal("a half-written archive was published under the object key")
	}
	// And no staging file is left behind for retention to trip over.
	objects, err := dest.List(context.Background(), "")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(objects) != 0 {
		t.Fatalf("List sees %+v after a failed Put", objects)
	}
}

type failingReader struct{}

func (r *failingReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }

func TestLocalDestinationRefusesKeysThatEscape(t *testing.T) {
	root := t.TempDir()
	dest, err := NewLocalDestination(root)
	if err != nil {
		t.Fatalf("NewLocalDestination: %v", err)
	}

	for _, key := range []string{
		"../escaped",
		"/etc/cron.d/pwn",
		"a/../../escaped",
		`a\b`,
		"a//b",
		"./a",
		"",
	} {
		if _, err := dest.Put(context.Background(), key, strings.NewReader("x"), 1); err == nil {
			t.Fatalf("key %q was accepted", key)
		}
		if _, err := dest.Get(context.Background(), key); err == nil {
			t.Fatalf("key %q was readable", key)
		}
	}
}

func TestLocalDestinationNeedsAnAbsoluteRoot(t *testing.T) {
	if _, err := NewLocalDestination("relative/path"); err == nil {
		t.Fatal("a relative root was accepted")
	}
	if _, err := NewLocalDestination(""); err == nil {
		t.Fatal("an empty root was accepted")
	}
}

func mustTime(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatalf("parse %q: %v", value, err)
	}
	return parsed
}

func TestObjectKeySortsChronologically(t *testing.T) {
	older, err := ObjectKey("tenant", KindWebsite, "shop", mustTime(t, "2025-01-01T00:00:00Z"), "shop.vkab")
	if err != nil {
		t.Fatalf("ObjectKey: %v", err)
	}
	newer, err := ObjectKey("tenant", KindWebsite, "shop", mustTime(t, "2025-06-01T00:00:00Z"), "shop.vkab")
	if err != nil {
		t.Fatalf("ObjectKey: %v", err)
	}
	if !(older < newer) {
		t.Fatalf("keys do not sort chronologically: %q then %q", older, newer)
	}
	if _, err := ObjectKey("tenant/evil", KindWebsite, "shop", mustTime(t, "2025-01-01T00:00:00Z"), "x"); err == nil {
		t.Fatal("a separator in a key part was accepted")
	}
}
