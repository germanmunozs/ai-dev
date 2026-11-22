package gitmanager

import (
	"regexp"
	"sort"
)

// SecretScreeningLimitation is declared wherever screening is reported
// (F008-FR-093).
const SecretScreeningLimitation = "secret screening detects known high-risk patterns only and does not guarantee detection"

// secretPatterns are the known high-risk credential formats (F008-FR-091).
// Each is a linear-time regular expression; a finding records only its
// category, never the matched bytes.
var secretPatterns = []struct {
	category string
	pattern  *regexp.Regexp
}{
	{"private_key", regexp.MustCompile(`-----BEGIN (?:[A-Z0-9]+ )*PRIVATE KEY(?: BLOCK)?-----`)},
	{"aws_access_key_id", regexp.MustCompile(`\b(?:AKIA|ASIA|AGPA|AIDA|AROA|ANPA|ANVA|AIPA)[0-9A-Z]{16}\b`)},
	{"aws_secret_access_key", regexp.MustCompile(`(?i)aws_?secret_?access_?key["']?\s*[:=]\s*["']?[A-Za-z0-9/+]{40}`)},
	{"github_token", regexp.MustCompile(`\b(?:gh[pousr]_[A-Za-z0-9]{36}|github_pat_[A-Za-z0-9_]{82})\b`)},
	{"gitlab_token", regexp.MustCompile(`\bglpat-[A-Za-z0-9_-]{20}\b`)},
	{"slack_token", regexp.MustCompile(`\bxox[abprs]-[A-Za-z0-9-]{10,}`)},
	{"google_api_key", regexp.MustCompile(`\bAIza[0-9A-Za-z_-]{35}`)},
	{"stripe_secret_key", regexp.MustCompile(`\b[sr]k_live_[0-9A-Za-z]{24,}`)},
	{"anthropic_api_key", regexp.MustCompile(`\bsk-ant-[A-Za-z0-9_-]{20,}`)},
	{"openai_api_key", regexp.MustCompile(`\bsk-(?:proj-)?[A-Za-z0-9_-]{40,}`)},
	{"npm_token", regexp.MustCompile(`\bnpm_[A-Za-z0-9]{36}\b`)},
	{"url_credentials", regexp.MustCompile(`[A-Za-z][A-Za-z0-9+.-]*://[^\s/:@]+:[^\s/@]+@[^\s/]+`)},
}

// screenSecrets returns the sorted categories of every high-risk finding
// in data (F008-FR-085). The input is already bounded by the authorized
// file limit.
func screenSecrets(data []byte) []string {
	var categories []string
	for _, p := range secretPatterns {
		if p.pattern.Match(data) {
			categories = append(categories, p.category)
		}
	}
	sort.Strings(categories)
	return categories
}

// secretFindings screens one file and reports only its path and the
// finding categories.
func secretFindings(path []byte, data []byte) []SecurityFinding {
	var findings []SecurityFinding
	for _, category := range screenSecrets(data) {
		findings = append(findings, SecurityFinding{Path: NewObservedPath(path), Category: category})
	}
	return findings
}
