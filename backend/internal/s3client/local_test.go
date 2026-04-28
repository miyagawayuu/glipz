package s3client

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestLocalClientPutHeadGetRange(t *testing.T) {
	c, err := NewLocal(t.TempDir(), "https://media.example.com")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := c.PutObject(ctx, "uploads/u/file.txt", "text/plain", strings.NewReader("hello world"), int64(len("hello world"))); err != nil {
		t.Fatal(err)
	}
	if got := c.PublicURL("uploads/u/file.txt"); got != "https://media.example.com/uploads/u/file.txt" {
		t.Fatalf("PublicURL = %q", got)
	}
	meta, err := c.HeadObject(ctx, "uploads/u/file.txt")
	if err != nil {
		t.Fatal(err)
	}
	if meta.ContentType != "text/plain" || meta.ContentLength != int64(len("hello world")) {
		t.Fatalf("HeadObject meta = %+v", meta)
	}
	obj, err := c.GetObject(ctx, "uploads/u/file.txt", "bytes=6-10")
	if err != nil {
		t.Fatal(err)
	}
	b, err := io.ReadAll(obj.Body)
	closeErr := obj.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if closeErr != nil {
		t.Fatal(closeErr)
	}
	if string(b) != "world" {
		t.Fatalf("range body = %q", b)
	}
	if obj.ContentRange != "bytes 6-10/11" || obj.ContentLength != 5 {
		t.Fatalf("range meta = %+v", obj.ObjectMeta)
	}
	if err := c.DeleteObject(ctx, "uploads/u/file.txt"); err != nil {
		t.Fatalf("DeleteObject: %v", err)
	}
	if _, err := c.HeadObject(ctx, "uploads/u/file.txt"); !IsNotFound(err) {
		t.Fatalf("HeadObject after delete err = %v, want not found", err)
	}
}

func TestLocalClientRejectsTraversal(t *testing.T) {
	c, err := NewLocal(t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	err = c.PutObject(context.Background(), "../escape.txt", "text/plain", strings.NewReader("x"), 1)
	if err == nil {
		t.Fatal("expected traversal key to fail")
	}
}

func TestLocalClientNotFoundAndUnsupportedPresign(t *testing.T) {
	c, err := NewLocal(t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.HeadObject(context.Background(), "missing.txt"); !IsNotFound(err) {
		t.Fatalf("HeadObject missing err = %v", err)
	}
	if _, err := c.PresignPut(context.Background(), "x", "text/plain", 0); !errors.Is(err, ErrPresignUnsupported) {
		t.Fatalf("PresignPut err = %v", err)
	}
}
