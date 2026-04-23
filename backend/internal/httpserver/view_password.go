package httpserver

import (
	"strings"

	"glipz.io/backend/internal/repo"
)

const maskedCaptionPlaceholder = " [閲覧パスワード保護] "

type viewPasswordTextRangeJSON struct {
	Start int `json:"start"`
	End   int `json:"end"`
}

func jsonRangesToRepo(ranges []viewPasswordTextRangeJSON) []repo.ViewPasswordTextRange {
	if len(ranges) == 0 {
		return nil
	}
	out := make([]repo.ViewPasswordTextRange, 0, len(ranges))
	for _, rg := range ranges {
		out = append(out, repo.ViewPasswordTextRange{Start: rg.Start, End: rg.End})
	}
	return out
}

func repoRangesToJSON(ranges []repo.ViewPasswordTextRange) []viewPasswordTextRangeJSON {
	if len(ranges) == 0 {
		return []viewPasswordTextRangeJSON{}
	}
	out := make([]viewPasswordTextRangeJSON, 0, len(ranges))
	for _, rg := range ranges {
		out = append(out, viewPasswordTextRangeJSON{Start: rg.Start, End: rg.End})
	}
	return out
}

func scopeProtectsText(scope int) bool {
	return scope == repo.ViewPasswordScopeAll || scope&repo.ViewPasswordScopeText != 0
}

func scopeProtectsMedia(scope int) bool {
	return scope == repo.ViewPasswordScopeAll || scope&repo.ViewPasswordScopeMedia != 0
}

func maskCaptionText(caption string, ranges []repo.ViewPasswordTextRange) string {
	if len(ranges) == 0 {
		return ""
	}
	runes := []rune(caption)
	var b strings.Builder
	pos := 0
	for _, rg := range ranges {
		if rg.Start > len(runes) || rg.End > len(runes) || rg.Start >= rg.End {
			continue
		}
		if pos < rg.Start {
			b.WriteString(string(runes[pos:rg.Start]))
		}
		b.WriteString(maskedCaptionPlaceholder)
		pos = rg.End
	}
	if pos < len(runes) {
		b.WriteString(string(runes[pos:]))
	}
	return b.String()
}
