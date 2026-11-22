package guide

import "unicode"

// TemplateSafe reports whether templates may be rendered for featureID: none
// of its runes is in Unicode category Cc (control) or Cf (format, including
// bidirectional overrides and zero-width characters), and none is U+2028 or
// U+2029 (F007-FR-006). Any other identifier gets its report with no
// templates.
func TemplateSafe(featureID string) bool {
	for _, r := range featureID {
		if unicode.Is(unicode.Cc, r) || unicode.Is(unicode.Cf, r) || r == '\u2028' || r == '\u2029' {
			return false
		}
	}
	return true
}
