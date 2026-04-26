package httpserver

import "testing"

func TestMediaContentTypeSafety(t *testing.T) {
	tests := []struct {
		name            string
		contentType     string
		active          bool
		inlineSafe      bool
		allowedUpload   bool
		allowedPresign  bool
		allowedDMAttach bool
		download        bool
	}{
		{
			name:            "svg image is active",
			contentType:     "image/svg+xml; charset=utf-8",
			active:          true,
			inlineSafe:      false,
			allowedUpload:   false,
			allowedPresign:  false,
			allowedDMAttach: false,
			download:        true,
		},
		{
			name:            "html is active",
			contentType:     "text/html",
			active:          true,
			inlineSafe:      false,
			allowedUpload:   false,
			allowedPresign:  false,
			allowedDMAttach: false,
			download:        true,
		},
		{
			name:            "xhtml is active",
			contentType:     "application/xhtml+xml",
			active:          true,
			inlineSafe:      false,
			allowedUpload:   false,
			allowedPresign:  false,
			allowedDMAttach: false,
			download:        true,
		},
		{
			name:            "javascript is active",
			contentType:     "application/javascript",
			active:          true,
			inlineSafe:      false,
			allowedUpload:   false,
			allowedPresign:  false,
			allowedDMAttach: false,
			download:        true,
		},
		{
			name:            "png is inline safe",
			contentType:     "image/png",
			active:          false,
			inlineSafe:      true,
			allowedUpload:   true,
			allowedPresign:  true,
			allowedDMAttach: true,
			download:        false,
		},
		{
			name:            "png parameters are normalized",
			contentType:     "image/png; charset=binary",
			active:          false,
			inlineSafe:      true,
			allowedUpload:   true,
			allowedPresign:  true,
			allowedDMAttach: true,
			download:        false,
		},
		{
			name:            "mp4 is inline safe",
			contentType:     "video/mp4",
			active:          false,
			inlineSafe:      true,
			allowedUpload:   true,
			allowedPresign:  true,
			allowedDMAttach: true,
			download:        false,
		},
		{
			name:            "mp3 is inline safe",
			contentType:     "audio/mpeg",
			active:          false,
			inlineSafe:      true,
			allowedUpload:   true,
			allowedPresign:  true,
			allowedDMAttach: true,
			download:        false,
		},
		{
			name:            "bmp is not an accepted public media format",
			contentType:     "image/bmp",
			active:          false,
			inlineSafe:      false,
			allowedUpload:   false,
			allowedPresign:  false,
			allowedDMAttach: true,
			download:        true,
		},
		{
			name:            "octet stream is DM attachment only",
			contentType:     "application/octet-stream",
			active:          false,
			inlineSafe:      false,
			allowedUpload:   false,
			allowedPresign:  false,
			allowedDMAttach: true,
			download:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isActiveMediaContentType(tt.contentType); got != tt.active {
				t.Fatalf("isActiveMediaContentType(%q) = %v, want %v", tt.contentType, got, tt.active)
			}
			if got := isInlineSafeMediaContentType(tt.contentType); got != tt.inlineSafe {
				t.Fatalf("isInlineSafeMediaContentType(%q) = %v, want %v", tt.contentType, got, tt.inlineSafe)
			}
			if got := isAllowedUploadMediaContentType(tt.contentType); got != tt.allowedUpload {
				t.Fatalf("isAllowedUploadMediaContentType(%q) = %v, want %v", tt.contentType, got, tt.allowedUpload)
			}
			if got := isAllowedPresignedMediaContentType(tt.contentType); got != tt.allowedPresign {
				t.Fatalf("isAllowedPresignedMediaContentType(%q) = %v, want %v", tt.contentType, got, tt.allowedPresign)
			}
			if got := isAllowedDMAttachmentContentType(tt.contentType); got != tt.allowedDMAttach {
				t.Fatalf("isAllowedDMAttachmentContentType(%q) = %v, want %v", tt.contentType, got, tt.allowedDMAttach)
			}
			if got := shouldDownloadMediaContentType(tt.contentType); got != tt.download {
				t.Fatalf("shouldDownloadMediaContentType(%q) = %v, want %v", tt.contentType, got, tt.download)
			}
		})
	}
}
