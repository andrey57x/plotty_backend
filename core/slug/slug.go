package slug

import (
	"strings"

	gosimple "github.com/gosimple/slug"
)

func FromTitle(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "story"
	}
	out := gosimple.Make(s)
	if out == "" {
		return "story"
	}
	return out
}
