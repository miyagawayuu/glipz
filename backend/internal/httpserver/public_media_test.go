package httpserver

import (
	"testing"

	"glipz.io/backend/internal/config"
)

func TestFederationRemoteMediaURL(t *testing.T) {
	s := &Server{cfg: config.Config{GlipzProtocolPublicOrigin: "https://local.example"}}

	got := s.federationRemoteMediaURL("https://remote.example/media/avatar.png#frag")
	want := "https://local.example/api/v1/media/remote?url=https%3A%2F%2Fremote.example%2Fmedia%2Favatar.png"
	if got != want {
		t.Fatalf("proxied remote media URL = %q, want %q", got, want)
	}

	own := "https://local.example/api/v1/media/object/users/u/avatar.png"
	if got := s.federationRemoteMediaURL(own); got != own {
		t.Fatalf("own media URL = %q, want unchanged %q", got, own)
	}

	localhost := "https://localhost/media/avatar.png"
	if got := s.federationRemoteMediaURL(localhost); got != localhost {
		t.Fatalf("unsafe localhost URL = %q, want unchanged %q", got, localhost)
	}
}

func TestLocalMediaDirectURLUsesConfiguredPublicBase(t *testing.T) {
	s := &Server{cfg: config.Config{GlipzProtocolMediaPublicBase: "https://media.example.com/"}}

	got := s.localMediaDirectURL("/posts/abc/image.png")
	want := "https://media.example.com/posts/abc/image.png"
	if got != want {
		t.Fatalf("direct media URL = %q, want %q", got, want)
	}
}
