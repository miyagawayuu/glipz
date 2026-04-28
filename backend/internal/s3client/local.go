package s3client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type LocalClient struct {
	root          string
	publicBaseURL string
}

type localObjectMeta struct {
	ContentType  string `json:"content_type"`
	SizeBytes    int64  `json:"size_bytes"`
	CacheControl string `json:"cache_control,omitempty"`
}

func NewLocal(root, publicBaseURL string) (*LocalClient, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, fmt.Errorf("local storage path is required")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return nil, err
	}
	return &LocalClient{
		root:          abs,
		publicBaseURL: strings.TrimSuffix(strings.TrimSpace(publicBaseURL), "/"),
	}, nil
}

func (c *LocalClient) PutObject(ctx context.Context, objectKey, contentType string, body io.Reader, size int64) error {
	if size <= 0 {
		return fmt.Errorf("local PutObject: invalid content length")
	}
	target, err := c.objectPath(objectKey)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(target), ".glipz-upload-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer func() {
		_ = os.Remove(tmpPath)
	}()
	written, copyErr := copyWithContext(ctx, tmp, body)
	closeErr := tmp.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	if written != size {
		return fmt.Errorf("local PutObject: wrote %d bytes, expected %d", written, size)
	}
	if err := os.Rename(tmpPath, target); err != nil {
		return err
	}
	return c.writeMeta(objectKey, localObjectMeta{
		ContentType:  normalizeContentType(contentType, objectKey),
		SizeBytes:    size,
		CacheControl: "public, max-age=31536000, immutable",
	})
}

func (c *LocalClient) PresignPut(context.Context, string, string, time.Duration) (string, error) {
	return "", ErrPresignUnsupported
}

func (c *LocalClient) HeadObject(ctx context.Context, objectKey string) (ObjectMeta, error) {
	select {
	case <-ctx.Done():
		return ObjectMeta{}, ctx.Err()
	default:
	}
	target, err := c.objectPath(objectKey)
	if err != nil {
		return ObjectMeta{}, err
	}
	st, err := os.Stat(target)
	if err != nil {
		if os.IsNotExist(err) {
			return ObjectMeta{}, ErrNotFound
		}
		return ObjectMeta{}, err
	}
	if st.IsDir() {
		return ObjectMeta{}, ErrNotFound
	}
	meta := c.readMeta(objectKey, st)
	return ObjectMeta{
		ContentType:   meta.ContentType,
		ContentLength: st.Size(),
		LastModified:  st.ModTime(),
		CacheControl:  meta.CacheControl,
		AcceptRanges:  "bytes",
	}, nil
}

func (c *LocalClient) GetObject(ctx context.Context, objectKey, byteRange string) (*ObjectReader, error) {
	target, err := c.objectPath(objectKey)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(target)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	st, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	if st.IsDir() {
		_ = f.Close()
		return nil, ErrNotFound
	}
	meta := c.readMeta(objectKey, st)
	start, end, partial, err := parseSingleRange(strings.TrimSpace(byteRange), st.Size())
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	if _, err := f.Seek(start, io.SeekStart); err != nil {
		_ = f.Close()
		return nil, err
	}
	length := end - start + 1
	var body io.ReadCloser = f
	if partial {
		body = struct {
			io.Reader
			io.Closer
		}{
			Reader: io.LimitReader(f, length),
			Closer: f,
		}
	}
	out := &ObjectReader{
		ObjectMeta: ObjectMeta{
			ContentType:   meta.ContentType,
			ContentLength: length,
			LastModified:  st.ModTime(),
			CacheControl:  meta.CacheControl,
			AcceptRanges:  "bytes",
		},
		Body: body,
	}
	if partial {
		out.ContentRange = fmt.Sprintf("bytes %d-%d/%d", start, end, st.Size())
	}
	select {
	case <-ctx.Done():
		_ = body.Close()
		return nil, ctx.Err()
	default:
		return out, nil
	}
}

func (c *LocalClient) DeleteObject(ctx context.Context, objectKey string) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	target, err := c.objectPath(objectKey)
	if err != nil {
		return err
	}
	if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
		return err
	}
	if meta, err := c.metaPath(objectKey); err == nil {
		if err := os.Remove(meta); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func (c *LocalClient) PublicURL(objectKey string) string {
	key := strings.TrimLeft(strings.TrimSpace(objectKey), "/")
	if c.publicBaseURL != "" && key != "" {
		return c.publicBaseURL + "/" + key
	}
	return "/api/v1/media/object/" + key
}

func (c *LocalClient) objectPath(objectKey string) (string, error) {
	key := strings.ReplaceAll(strings.TrimSpace(objectKey), "\\", "/")
	key = strings.TrimLeft(key, "/")
	for _, part := range strings.Split(key, "/") {
		if part == ".." {
			return "", fmt.Errorf("invalid object key")
		}
	}
	cleaned := path.Clean(key)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") || strings.HasPrefix(cleaned, "/") {
		return "", fmt.Errorf("invalid object key")
	}
	if strings.HasSuffix(cleaned, ".meta.json") {
		return "", fmt.Errorf("invalid object key")
	}
	full := filepath.Join(c.root, filepath.FromSlash(cleaned))
	if !strings.HasPrefix(full, c.root+string(os.PathSeparator)) && full != c.root {
		return "", fmt.Errorf("invalid object key")
	}
	return full, nil
}

func (c *LocalClient) metaPath(objectKey string) (string, error) {
	p, err := c.objectPath(objectKey)
	if err != nil {
		return "", err
	}
	return p + ".meta.json", nil
}

func (c *LocalClient) writeMeta(objectKey string, meta localObjectMeta) error {
	p, err := c.metaPath(objectKey)
	if err != nil {
		return err
	}
	b, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	return os.WriteFile(p, b, 0o644)
}

func (c *LocalClient) readMeta(objectKey string, st os.FileInfo) localObjectMeta {
	meta := localObjectMeta{
		ContentType:  normalizeContentType("", objectKey),
		SizeBytes:    st.Size(),
		CacheControl: "public, max-age=31536000, immutable",
	}
	p, err := c.metaPath(objectKey)
	if err != nil {
		return meta
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return meta
	}
	var stored localObjectMeta
	if err := json.Unmarshal(b, &stored); err != nil {
		return meta
	}
	if strings.TrimSpace(stored.ContentType) != "" {
		meta.ContentType = strings.TrimSpace(stored.ContentType)
	}
	if strings.TrimSpace(stored.CacheControl) != "" {
		meta.CacheControl = strings.TrimSpace(stored.CacheControl)
	}
	return meta
}

func normalizeContentType(contentType, objectKey string) string {
	if ct := strings.TrimSpace(contentType); ct != "" {
		return ct
	}
	if ext := path.Ext(objectKey); ext != "" {
		if ct := mime.TypeByExtension(ext); ct != "" {
			return ct
		}
	}
	return "application/octet-stream"
}

func parseSingleRange(raw string, size int64) (start, end int64, partial bool, err error) {
	if raw == "" {
		return 0, size - 1, false, nil
	}
	if size <= 0 || !strings.HasPrefix(raw, "bytes=") || strings.Contains(raw, ",") {
		return 0, 0, false, ErrInvalidRange
	}
	spec := strings.TrimPrefix(raw, "bytes=")
	parts := strings.SplitN(spec, "-", 2)
	if len(parts) != 2 {
		return 0, 0, false, ErrInvalidRange
	}
	if parts[0] == "" {
		suffix, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil || suffix <= 0 {
			return 0, 0, false, ErrInvalidRange
		}
		if suffix > size {
			suffix = size
		}
		return size - suffix, size - 1, true, nil
	}
	start, err = strconv.ParseInt(parts[0], 10, 64)
	if err != nil || start < 0 || start >= size {
		return 0, 0, false, ErrInvalidRange
	}
	if parts[1] == "" {
		return start, size - 1, true, nil
	}
	end, err = strconv.ParseInt(parts[1], 10, 64)
	if err != nil || end < start {
		return 0, 0, false, ErrInvalidRange
	}
	if end >= size {
		end = size - 1
	}
	return start, end, true, nil
}

func copyWithContext(ctx context.Context, dst io.Writer, src io.Reader) (int64, error) {
	buf := make([]byte, 32*1024)
	var written int64
	for {
		select {
		case <-ctx.Done():
			return written, ctx.Err()
		default:
		}
		nr, er := src.Read(buf)
		if nr > 0 {
			nw, ew := dst.Write(buf[:nr])
			if nw > 0 {
				written += int64(nw)
			}
			if ew != nil {
				return written, ew
			}
			if nr != nw {
				return written, io.ErrShortWrite
			}
		}
		if er != nil {
			if er == io.EOF {
				return written, nil
			}
			return written, er
		}
	}
}

var _ Store = (*LocalClient)(nil)
