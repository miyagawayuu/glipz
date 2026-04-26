package httpserver

import (
	"io"
	"net/http"
	"strconv"
	"strings"
)

func responseContentLengthExceeds(h http.Header, maxBytes int64) bool {
	if maxBytes <= 0 {
		return false
	}
	raw := strings.TrimSpace(h.Get("Content-Length"))
	if raw == "" {
		return false
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	return err == nil && n > maxBytes
}

func copyWithMaxBytes(w io.Writer, r io.Reader, maxBytes int64) (written int64, exceeded bool, err error) {
	if maxBytes <= 0 {
		n, err := io.Copy(w, r)
		return n, false, err
	}
	n, err := io.Copy(w, io.LimitReader(r, maxBytes))
	if err != nil {
		return n, false, err
	}
	if n < maxBytes {
		return n, false, nil
	}
	var probe [1]byte
	m, err := r.Read(probe[:])
	if m > 0 {
		return n, true, nil
	}
	if err != nil && err != io.EOF {
		return n, false, err
	}
	return n, false, nil
}
