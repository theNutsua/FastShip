package detect

import (
	"os"
	"path/filepath"
	"testing"
)

// writeRepo builds a throwaway directory from a map of filename to
// contents. t.TempDir cleans it up automatically when the test ends.
func writeRepo(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()

	for name, content := range files {
		path := filepath.Join(dir, name)

		// Support nested paths like "src/index.js".
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestDetectRuntimeNode(t *testing.T) {
	repo := writeRepo(t, map[string]string{
		"package.json": `{"engines":{"node":">=18.0.0"}}`,
	})

	got, err := DetectRuntime(repo)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "node@18" {
		t.Errorf("got %q, want node@18", got)
	}
}

func TestDetectRuntimeGo(t *testing.T) {
	repo := writeRepo(t, map[string]string{
		"go.mod": "module example.com/app\n\ngo 1.22\n",
	})

	got, err := DetectRuntime(repo)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "go@1.22" {
		t.Errorf("got %q, want go@1.22", got)
	}
}

// A project with no version pinned should fall back to the default.
func TestDetectRuntimeNoVersion(t *testing.T) {
	repo := writeRepo(t, map[string]string{
		"package.json": `{"name":"app"}`,
	})

	got, _ := DetectRuntime(repo)
	if got != "node@"+defaultVersions["node"] {
		t.Errorf("got %q, want the default node version", got)
	}
}

// Ambiguity must error rather than pick one. This is the rule from the
// package doc: guessing silently is worse than failing loudly.
func TestDetectRuntimeAmbiguous(t *testing.T) {
	repo := writeRepo(t, map[string]string{
		"package.json":     `{"name":"frontend"}`,
		"requirements.txt": "flask\n",
	})

	if _, err := DetectRuntime(repo); err == nil {
		t.Error("expected an error for a repo with two runtimes")
	}
}

func TestDetectRuntimeNothing(t *testing.T) {
	repo := writeRepo(t, map[string]string{
		"README.md": "hello",
	})

	if _, err := DetectRuntime(repo); err == nil {
		t.Error("expected an error when no manifest is present")
	}
}

func TestDetectStartNode(t *testing.T) {
	repo := writeRepo(t, map[string]string{
		"package.json": `{"scripts":{"start":"node dist/server.js"}}`,
	})

	got, err := DetectStart(repo, "node@20")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// npm start rather than the raw script, so pre/post hooks still fire.
	if got != "npm start" {
		t.Errorf("got %q, want npm start", got)
	}
}

// A Procfile is the most explicit signal available, so it must beat
// package.json.
func TestDetectStartProcfileWins(t *testing.T) {
	repo := writeRepo(t, map[string]string{
		"package.json": `{"scripts":{"start":"node index.js"}}`,
		"Procfile":     "web: node custom.js\n",
	})

	got, _ := DetectStart(repo, "node@20")
	if got != "node custom.js" {
		t.Errorf("got %q, want the Procfile command", got)
	}
}

func TestDetectPortFromSource(t *testing.T) {
	repo := writeRepo(t, map[string]string{
		"package.json": `{"name":"app"}`,
		"index.js":     `require("http").createServer().listen(4000)`,
	})

	if got := DetectPort(repo, "node@20"); got != 4000 {
		t.Errorf("got %d, want 4000", got)
	}
}

// The regression test for the smtp_port problem. Without the ^ anchor,
// this returns 587.
func TestDetectPortIgnoresConnectPorts(t *testing.T) {
	repo := writeRepo(t, map[string]string{
		"requirements.txt": "flask\n",
		"app.py": "smtp_port = 587\n" +
			"redis_port = 6379\n" +
			"port = 8000\n",
	})

	if got := DetectPort(repo, "python@3.12"); got != 8000 {
		t.Errorf("got %d, want 8000 — matched a connect port instead", got)
	}
}

func TestDetectPortFallsBackToDefault(t *testing.T) {
	repo := writeRepo(t, map[string]string{
		"package.json": `{"name":"app"}`,
		"index.js":     `console.log("no server here")`,
	})

	if got := DetectPort(repo, "node@20"); got != 3000 {
		t.Errorf("got %d, want the node default of 3000", got)
	}
}

// node_modules must never be scanned. Without the skip, the dependency's
// listen call would win over the app's own.
func TestDetectPortSkipsNodeModules(t *testing.T) {
	repo := writeRepo(t, map[string]string{
		"package.json":                `{"name":"app"}`,
		"index.js":                    `app.listen(4000)`,
		"node_modules/express/lib.js": `server.listen(9999)`,
	})

	if got := DetectPort(repo, "node@20"); got == 9999 {
		t.Error("scanned node_modules — it must be skipped")
	}
}
