package ingest

import (
	"path"
	"strings"

	"github.com/cpkeller25/cairn/internal/scorecard"
)

// factsFromPaths derives repository facts from a list of file paths, all
// relativve to the repository root.  It is pure: same input, smake output
func factsFromPaths(paths []string) scorecard.Facts {
	var f scorecard.Facts

	for _, p := range paths {
		lower := strings.ToLower(p)

		if isReadme(lower) {
			f.HasReadme = true
		}

		if isCIConfig(lower) {
			f.HasCI = true
		}

		if isDockerfile(lower) {
			f.HasDockerfile = true
		}

		if isLicenseFile(lower) {
			f.HasLicense = true
		}

		if isTestPath(lower) {
			f.HasTests = true
		}
	}

	return f
}

func isRootLevel(p string) bool { return !strings.Contains(p, "/") }

func isReadme(p string) bool {
	if !isRootLevel(p) {
		return false
	}
	return p == "readme" || strings.HasPrefix(p, "readme.")
}

func isLicenseFile(p string) bool {
	if !isRootLevel(p) {
		return false
	}
	switch {
	case p == "license", p == "licence", p == "copying", p == "unlicense":
		return true
	case strings.HasPrefix(p, "license."), strings.HasPrefix(p, "licence."):
		return true
	}
	return false
}

func isDockerfile(p string) bool {
	base := path.Base(p)
	return base == "dockerfile" || strings.HasPrefix(base, "dockerfile.")
}

func isCIConfig(p string) bool {
	if strings.HasPrefix(p, ".github/workflows/") &&
		(strings.HasSuffix(p, ".yml") || strings.HasSuffix(p, ".yaml")) {
		return true
	}
	switch p {
	case ".gitlab-ci.yml", ".travis.yml", "jenkinsfile",
		"azure-pipelines.yml", ".circleci/config.yml", "cloudbuild.yaml":
		return true
	}
	return false
}

// testDirs are directory names that conventionally hold tests.
var testDirs = map[string]bool{
	"test": true, "tests": true, "spec": true, "specs": true, "__tests__": true,
}

func isTestPath(p string) bool {
	base := path.Base(p)

	switch {
	case strings.HasSuffix(base, "_test.go"): // Go
		return true
	case strings.HasSuffix(base, "_test.py"), strings.HasPrefix(base, "test_") && strings.HasSuffix(base, ".py"):
		return true
	case strings.HasSuffix(base, "_spec.rb"), strings.HasSuffix(base, "_test.rb"): // Ruby
		return true
	case strings.HasSuffix(base, "test.java"), strings.HasSuffix(base, "tests.java"): // Java
		return true
	case strings.HasSuffix(base, ".test.js"), strings.HasSuffix(base, ".test.ts"),
		strings.HasSuffix(base, ".spec.js"), strings.HasSuffix(base, ".spec.ts"):
		return true
	}

	// Any path segment that is conventionally a test directory.
	for _, segment := range strings.Split(path.Dir(p), "/") {
		if testDirs[segment] {
			return true
		}
	}
	return false
}
