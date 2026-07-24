package detect

import (
	"fmt"
	"path/filepath"
	"strings"
)

// conventionalEntrypoints lists the files each ecosystem uses by default,
// in the order they should be tried.
var conventionalEntrypoints = map[string][]string{
	"node":   {"index.js", "server.js", "app.js", "main.js", "src/index.js"},
	"python": {"main.py", "app.py", "wsgi.py"},
	"ruby":   {"config.ru", "app.rb"},
}

// DetectStart works out how to launch the app.
//
// Each ecosystem has a conventional place that declares this — the start
// script in package.json, a Procfile, or a well-known entry file. This
// checks them in order of how explicit they are.
func DetectStart(repoPath, runtime string) (string, error) {
	lang := languageOf(runtime)

	// A Procfile is the most explicit signal in any ecosystem, so it wins
	// regardless of language.
	if cmd := readProcfile(repoPath); cmd != "" {
		return cmd, nil
	}

	switch lang {
	case "node":
		return nodeStart(repoPath)
	case "go":
		// Go compiles to a single binary. The build engine produces it at
		// a known path, so the start command is fixed.
		return "./app", nil
	case "python":
		return conventionalStart(repoPath, lang, "python %s")
	case "ruby":
		return conventionalStart(repoPath, lang, "ruby %s")
	case "rust":
		return "./app", nil
	}

	return "", fmt.Errorf(
		"could not determine how to start a %s app\n\n"+
			"add one to ship.yaml, for example:\n"+
			"  start: ./run.sh", lang)
}

// nodeStart prefers the start script in package.json, then main, then
// conventional entry files.
func nodeStart(repoPath string) (string, error) {
	pkg, err := readPackageJSON(repoPath)
	if err == nil {
		if s := pkg.Scripts["start"]; s != "" {
			// Run through npm rather than using the raw script, so any
			// pre/post hooks the project defines still fire.
			return "npm start", nil
		}
		if pkg.Main != "" {
			return "node " + pkg.Main, nil
		}
	}

	return conventionalStart(repoPath, "node", "node %s")
}

// conventionalStart looks for a language's well-known entry files.
func conventionalStart(repoPath, lang, format string) (string, error) {
	for _, entry := range conventionalEntrypoints[lang] {
		if fileExists(filepath.Join(repoPath, entry)) {
			return fmt.Sprintf(format, entry), nil
		}
	}

	return "", fmt.Errorf(
		"could not find an entry point for this %s app\n\n"+
			"add one to ship.yaml, for example:\n"+
			"  start: %s", lang, fmt.Sprintf(format, "main file"))
}

// readProcfile returns the web process command from a Procfile.
//
// Format is "web: npm start" — one process type per line. FastShip only
// looks at the web entry since that is the one that serves traffic.
func readProcfile(repoPath string) string {
	data, err := readFileString(filepath.Join(repoPath, "Procfile"))
	if err != nil {
		return ""
	}

	for _, line := range strings.Split(data, "\n") {
		if strings.HasPrefix(line, "web:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "web:"))
		}
	}
	return ""
}

// languageOf strips the version from a runtime string.
// "node@20" becomes "node".
func languageOf(runtime string) string {
	if i := strings.Index(runtime, "@"); i >= 0 {
		return runtime[:i]
	}
	return runtime
}
