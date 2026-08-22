package main_test

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

var binary string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "pingu-bin")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	binary = filepath.Join(dir, "pingu")
	build := exec.Command("go", "build", "-o", binary, ".")
	build.Dir = "."
	if out, err := build.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "build failed: %v\n%s", err, out)
		os.Exit(1)
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// run executes the built binary and returns stdout, stderr, and the exit code.
func run(t *testing.T, env []string, args ...string) (string, string, int) {
	t.Helper()
	cmd := exec.Command(binary, args...)
	cmd.Env = append(os.Environ(), env...)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	code := 0
	if exit, ok := err.(*exec.ExitError); ok {
		code = exit.ExitCode()
	} else if err != nil {
		t.Fatalf("run pingu: %v", err)
	}
	return stdout.String(), stderr.String(), code
}

// fakeOpenAI serves a streaming text completion.
func fakeOpenAI(t *testing.T, reply string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Drain the request body so the provider can serialize it.
		io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintf(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":%q}}]}\n\n", reply)
		fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n")
		fmt.Fprint(w, "data: {\"choices\":[],\"usage\":{\"prompt_tokens\":3,\"completion_tokens\":2}}\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
}

func testEnv(baseURL string) []string {
	return []string{
		"OPENAI_API_KEY=secret-key-value-do-not-log",
		"OPENAI_BASE_URL=" + baseURL,
		"LOG_LEVEL=debug",
	}
}

func TestInitCreatesAgent(t *testing.T) {
	dir := t.TempDir()
	agentDir := filepath.Join(dir, "my-agent")
	stdout, _, code := run(t, nil, "init", agentDir)
	if code != 0 {
		t.Fatalf("exit = %d, stdout = %q", code, stdout)
	}
	for _, f := range []string{"instructions.md", ".gitignore"} {
		if _, err := os.Stat(filepath.Join(agentDir, f)); err != nil {
			t.Errorf("missing %s: %v", f, err)
		}
	}
	gitignore, _ := os.ReadFile(filepath.Join(agentDir, ".gitignore"))
	if !strings.Contains(string(gitignore), ".pingu/") {
		t.Errorf("gitignore = %q", gitignore)
	}
}

func TestInitRefusesNonEmpty(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "occupied"), []byte("x"), 0o644)
	_, _, code := run(t, nil, "init", dir)
	if code != 2 {
		t.Errorf("exit = %d, want 2", code)
	}
	// The existing file must be untouched.
	data, _ := os.ReadFile(filepath.Join(dir, "occupied"))
	if string(data) != "x" {
		t.Error("existing file was modified")
	}
}

func TestRunMessageEndToEnd(t *testing.T) {
	srv := fakeOpenAI(t, "Hello from the fake provider")
	defer srv.Close()

	dir := t.TempDir()
	agentDir := filepath.Join(dir, "agent")
	if _, _, code := run(t, nil, "init", agentDir); code != 0 {
		t.Fatalf("init failed")
	}

	stdout, stderr, code := run(t, testEnv(srv.URL), "run", agentDir, "--message", "hi there")
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	if !strings.Contains(stdout, "Hello from the fake provider") {
		t.Errorf("stdout = %q", stdout)
	}
	// No secrets in logs, even at debug level.
	if strings.Contains(stderr, "secret-key-value-do-not-log") {
		t.Errorf("API key leaked into logs: %q", stderr)
	}
}

func TestRunMissingInstructions(t *testing.T) {
	dir := t.TempDir()
	_, stderr, code := run(t, testEnv("http://unused"), "run", dir, "--message", "hi")
	if code != 2 {
		t.Errorf("exit = %d, want 2", code)
	}
	if !strings.Contains(stderr, "instructions.md") {
		t.Errorf("stderr = %q", stderr)
	}
}

func TestRunMissingAPIKey(t *testing.T) {
	dir := t.TempDir()
	agentDir := filepath.Join(dir, "agent")
	run(t, nil, "init", agentDir)
	_, stderr, code := run(t, []string{"OPENAI_API_KEY=", "OPENAI_BASE_URL="}, "run", agentDir, "--message", "hi")
	if code != 2 {
		t.Errorf("exit = %d, want 2", code)
	}
	if !strings.Contains(stderr, "OPENAI_API_KEY") {
		t.Errorf("stderr = %q", stderr)
	}
}

func TestRunProviderFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, `{"error":{"message":"nope"}}`)
	}))
	defer srv.Close()

	dir := t.TempDir()
	agentDir := filepath.Join(dir, "agent")
	run(t, nil, "init", agentDir)
	_, stderr, code := run(t, testEnv(srv.URL), "run", agentDir, "--message", "hi")
	if code != 1 {
		t.Errorf("exit = %d, want 1", code)
	}
	if !strings.Contains(stderr, "http_500") {
		t.Errorf("stderr = %q", stderr)
	}
}

func TestHelp(t *testing.T) {
	stdout, _, code := run(t, nil, "--help")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	for _, want := range []string{"Usage:", "init", "run"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("help missing %q", want)
		}
	}
}

func TestBadModelRef(t *testing.T) {
	srv := fakeOpenAI(t, "unused")
	defer srv.Close()
	dir := t.TempDir()
	agentDir := filepath.Join(dir, "agent")
	run(t, nil, "init", agentDir)
	_, stderr, code := run(t, testEnv(srv.URL), "run", agentDir, "--message", "hi", "--model", "no-slash")
	if code != 2 {
		t.Errorf("exit = %d, want 2", code)
	}
	if !strings.Contains(stderr, "model") {
		t.Errorf("stderr = %q", stderr)
	}
}
