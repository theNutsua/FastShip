package detect

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// manifest maps a marker file to the runtime it implies.
// Order matters. The slice is checked top to bottom and the first match
// wins, so more specific markers belong earlier.
var manifests = []struct {
	file    string
	runtime string
}{
	{"go.mod", "go"},
	{"package.json", "node"},
	{"Cargo.toml", "rust"},
	{"pyproject.toml", "python"},
	{"requirements.txt", "python"},
	{"Gemfile", "ruby"},
	{"pom.xml", "java"},
	{"build.gradle", "java"},
}

// Default versions used when a project does not pin one. These should
// track the current LTS or stable release for each ecosystem.
var defaultVersions = map[string]string{
	"go":     "1.22",
	"node":   "20",
	"python": "3.12",
	"ruby":   "3.3",
	"rust":   "1.75",
	"java":   "21",
}

// DetectRuntime scans a repository root and returns "language@version",
// e.g. "node@20".
// It returns an error rather than guessing when no manifest is found, or
// when two manifests for different languages are present — both cases
// need the engineer to be explicit.
func DetectRuntime(repoPath string) (string, error) {
	var found []string

	for _, m := range manifests {
		if fileExists(filepath.Join(repoPath, m.file)) {
			// Two markers for the same language (pyproject.toml and
			// requirements.txt) are not a conflict.
			if !contains(found, m.runtime) {
				found = append(found, m.runtime)
			}
		}
	}

	switch len(found) {
	case 0:
		return "", fmt.Errorf(
			"could not detect a runtime in %s\n\n"+
				"add one to fastship.yaml, for example:\n"+
				"  runtime: node@20", repoPath)

	case 1:
		// Exactly one language. Now find its version.
		lang := found[0]
		version := detectVersion(repoPath, lang)
		return lang + "@" + version, nil

	default:
		// Multiple languages — common in a repo with a Python API and a
		// JS frontend. FastShip will not choose for you.
		return "", fmt.Errorf(
			"found multiple runtimes in %s: %s\n\n"+
				"set one explicitly in fastship.yaml:\n"+
				"  runtime: %s@%s\n\n"+
				"or split them into separate apps with a path: for each",
			repoPath, strings.Join(found, ", "),
			found[0], defaultVersions[found[0]])
	}
}

// detectVersion finds the pinned version for a language, falling back to
// the default when nothing is pinned.
func detectVersion(repoPath, lang string) string {
	switch lang {
	case "node":
		if v := nodeVersion(repoPath); v != "" {
			return v
		}
	case "go":
		if v := goVersion(repoPath); v != "" {
			return v
		}
	case "python":
		if v := readVersionFile(repoPath, ".python-version"); v != "" {
			return v
		}
	case "ruby":
		if v := readVersionFile(repoPath, ".ruby-version"); v != "" {
			return v
		}
	}
	return defaultVersions[lang]
}

// nodeVersion checks .nvmrc first, then the engines field in package.json.
func nodeVersion(repoPath string) string {
	if v := readVersionFile(repoPath, ".nvmrc"); v != "" {
		return v
	}

	pkg, err := readPackageJSON(repoPath)
	if err != nil {
		return ""
	}

	// engines.node is usually a range like ">=18" or "^20.0.0". FastShip
	// needs a concrete major version, so pull the first number out.
	return majorVersion(pkg.Engines.Node)
}

// goVersion reads the "go 1.22" directive from go.mod.
func goVersion(repoPath string) string {
	data, err := os.ReadFile(filepath.Join(repoPath, "go.mod"))
	if err != nil {
		return ""
	}

	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "go ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "go "))
		}
	}
	return ""
}

// packageJSON is the subset of package.json FastShip cares about.
type packageJSON struct {
	Scripts map[string]string `json:"scripts"`
	Main    string            `json:"main"`
	Engines struct {
		Node string `json:"node"`
	} `json:"engines"`
}

func readPackageJSON(repoPath string) (*packageJSON, error) {
	data, err := os.ReadFile(filepath.Join(repoPath, "package.json"))
	if err != nil {
		return nil, err
	}

	var pkg packageJSON
	if err := json.Unmarshal(data, &pkg); err != nil {
		return nil, err
	}
	return &pkg, nil
}

// readVersionFile reads a single-line version file like .nvmrc, stripping
// any leading "v" so "v18.2.0" becomes "18.2.0".
func readVersionFile(repoPath, name string) string {
	data, err := os.ReadFile(filepath.Join(repoPath, name))
	if err != nil {
		return ""
	}
	return strings.TrimPrefix(strings.TrimSpace(string(data)), "v")
}

// majorVersion extracts the first number from a version range.
// ">=18.0.0" becomes "18", "^20" becomes "20".
var versionRe = regexp.MustCompile(`(\d+)`)

func majorVersion(s string) string {
	m := versionRe.FindStringSubmatch(s)
	if len(m) < 2 {
		return ""
	}
	return m[1]
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func contains(list []string, s string) bool {
	for _, item := range list {
		if item == s {
			return true
		}
	}
	return false
}
