package detect

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// Conventional ports per ecosystem, used when scanning finds nothing.
var defaultPorts = map[string]int{
	"node":   3000,
	"python": 8000,
	"go":     8080,
	"ruby":   3000,
	"rust":   8080,
	"java":   8080,
}

// listenPatterns match the common ways each language binds a port.
//
// These are regexes over source text, not real parsing. They catch the
// common cases and miss clever ones — which is acceptable because the
// primary strategy is injecting PORT as an environment variable and
// letting the app read it. This scan is a fallback.
var listenPatterns = []*regexp.Regexp{
	regexp.MustCompile(`\.listen\(\s*(\d{2,5})`),          // node: app.listen(3000)
	regexp.MustCompile(`(?m)^\s*port\s*[=:]\s*(\d{2,5})`), // python: port=8000
	regexp.MustCompile(`ListenAndServe\(\s*"[^"]*:(\d+)`), // go: ":8080"
	regexp.MustCompile(`bind\(\s*"[^"]*:(\d+)`),           // rust: "0.0.0.0:8080"
}

// Directories never worth scanning. node_modules alone would mean reading
// thousands of files, and dependencies contain listen calls that are not
// the app's own.
var skipDirs = map[string]bool{
	"node_modules": true,
	"vendor":       true,
	".git":         true,
	"dist":         true,
	"build":        true,
	"target":       true,
	"__pycache__":  true,
	".venv":        true,
}

// Source extensions worth scanning, by language.
var sourceExts = map[string][]string{
	"node":   {".js", ".ts", ".mjs"},
	"python": {".py"},
	"go":     {".go"},
	"ruby":   {".rb"},
	"rust":   {".rs"},
}

// DetectPort finds the port an app listens on.
//
// Unlike runtime and start detection this never fails — a port of 0 is
// meaningful, indicating an internal service that needs no external route.
func DetectPort(repoPath, runtime string) int {
	lang := languageOf(runtime)

	if port := scanForPort(repoPath, lang); port > 0 {
		return port
	}

	// Nothing found. Use the ecosystem convention — and because FastShip
	// injects PORT as an environment variable at runtime, most apps that
	// read process.env.PORT will pick this up and agree.
	return defaultPorts[lang]
}

// scanForPort walks source files looking for a listen call.
//
// Depth is capped at 2 to keep this fast. An app that binds its port five
// directories deep is unusual enough to warrant setting port: explicitly.
func scanForPort(repoPath, lang string) int {
	exts := sourceExts[lang]
	if len(exts) == 0 {
		return 0
	}

	var found int

	filepath.Walk(repoPath, func(path string, info os.FileInfo, err error) error {
		if err != nil || found > 0 {
			return nil
		}

		if info.IsDir() {
			if skipDirs[info.Name()] {
				return filepath.SkipDir
			}
			if depth(repoPath, path) > 2 {
				return filepath.SkipDir
			}
			return nil
		}

		if !hasExt(path, exts) {
			return nil
		}

		if port := scanFile(path); port > 0 {
			found = port
		}
		return nil
	})

	return found
}

// scanFile reads one file and returns the first plausible port it finds.
func scanFile(path string) int {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}

	for _, re := range listenPatterns {
		m := re.FindSubmatch(data)
		if len(m) < 2 {
			continue
		}

		port, err := strconv.Atoi(string(m[1]))
		if err != nil {
			continue
		}

		// Ports below 1024 need root to bind, so an app declaring one is
		// almost certainly a false match on some unrelated number.
		if port >= 1024 && port <= 65535 {
			return port
		}
	}

	return 0
}

func depth(root, path string) int {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return 99
	}
	return strings.Count(rel, string(filepath.Separator))
}

func hasExt(path string, exts []string) bool {
	ext := filepath.Ext(path)
	for _, e := range exts {
		if ext == e {
			return true
		}
	}
	return false
}

// readFileString is a small helper used by start.go.
func readFileString(path string) (string, error) {
	data, err := os.ReadFile(path)
	return string(data), err
}
