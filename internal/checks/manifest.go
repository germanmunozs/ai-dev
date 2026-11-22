package checks

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"sort"
	"strings"
)

type fileManifest struct {
	Files map[string]string `json:"files"`
}

// ValidateManifestFiles returns one issue per invalid manifest or required file.
func ValidateManifestFiles(root fs.FS, manifestPath string) (issues []string) {
	defer func() {
		if recovered := recover(); recovered != nil {
			issues = []string{fmt.Sprintf("unexpected filesystem failure while validating %s: %v", manifestPath, recovered)}
		}
	}()
	data, err := fs.ReadFile(root, manifestPath)
	if err != nil {
		return []string{fmt.Sprintf("manifest %s is missing or unreadable: %v", manifestPath, err)}
	}
	var manifest fileManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return []string{fmt.Sprintf("manifest %s is malformed: %v", manifestPath, err)}
	}
	if manifest.Files == nil {
		return []string{fmt.Sprintf("manifest %s has no files map", manifestPath)}
	}
	if len(manifest.Files) == 0 {
		return []string{fmt.Sprintf("manifest %s has an empty files map", manifestPath)}
	}
	paths := make([]string, 0, len(manifest.Files))
	for path := range manifest.Files {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	issues = make([]string, 0)
	for _, path := range paths {
		if !fs.ValidPath(path) || path == "." {
			issues = append(issues, fmt.Sprintf("manifest lists unsafe repository-relative path %q", path))
			continue
		}
		contents, err := fs.ReadFile(root, path)
		if err != nil {
			issues = append(issues, fmt.Sprintf("required file %s is missing or unreadable", path))
			continue
		}
		if strings.TrimSpace(string(contents)) == "" {
			issues = append(issues, fmt.Sprintf("required file %s is empty", path))
		}
	}
	return issues
}
