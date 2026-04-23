package repo

import (
	"reflect"
	"testing"
)

func TestExtractHashtags(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		text string
		want []string
	}{
		{
			name: "ascii and duplicates",
			text: "hello #GoLang #golang #search_test",
			want: []string{"golang", "search_test"},
		},
		{
			name: "unicode tags",
			text: "今日は #連合 と #ハッシュタグ を試す",
			want: []string{"連合", "ハッシュタグ"},
		},
		{
			name: "ignore url fragments",
			text: "https://example.com/#fragment と #realtag",
			want: []string{"realtag"},
		},
		{
			name: "ignore invalid starter",
			text: "abc#hidden ##double # visible",
			want: []string{},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := ExtractHashtags(tt.text)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("ExtractHashtags(%q) = %#v, want %#v", tt.text, got, tt.want)
			}
		})
	}
}

func TestNormalizeHashtagQuery(t *testing.T) {
	t.Parallel()

	if got, want := NormalizeHashtagQuery("  #GoLang  "), "golang"; got != want {
		t.Fatalf("NormalizeHashtagQuery mismatch: got %q want %q", got, want)
	}
	if got, want := NormalizeHashtagQuery(" #連合 "), "連合"; got != want {
		t.Fatalf("NormalizeHashtagQuery mismatch: got %q want %q", got, want)
	}
}
