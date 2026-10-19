package cmd

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = w
	fn()
	w.Close()
	os.Stderr = old
	out, _ := io.ReadAll(r)
	r.Close()
	return string(out)
}

func TestWarnCredentialMisconfig(t *testing.T) {
	tests := []struct {
		name        string
		username    string
		password    string
		passwordEnv string
		envValue    string
		setEnv      bool
		wantSubstr  string
	}{
		{name: "password without username", password: "pw", wantSubstr: "without --username"},
		{name: "password-env unset", passwordEnv: "IMGP_TEST_UNSET", wantSubstr: "is unset or empty"},
		{name: "password-env empty", passwordEnv: "IMGP_TEST_EMPTY", setEnv: true, wantSubstr: "is unset or empty"},
		{name: "username without password", username: "user", wantSubstr: "no CLI credentials will be sent"},
		{name: "complete credentials", username: "user", password: "pw", wantSubstr: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer saveGlobals()()
			username, password, passwordEnv = tt.username, tt.password, tt.passwordEnv
			if tt.setEnv {
				os.Setenv(tt.passwordEnv, "")
				defer os.Unsetenv(tt.passwordEnv)
			}
			out := captureStderr(t, warnCredentialMisconfig)
			if tt.wantSubstr == "" {
				if out != "" {
					t.Errorf("expected no WARN, got %q", out)
				}
				return
			}
			if !strings.Contains(out, tt.wantSubstr) {
				t.Errorf("stderr = %q, want substring %q", out, tt.wantSubstr)
			}
		})
	}
}

func TestCacheInfoCmd_NotExist(t *testing.T) {
	baseDir := t.TempDir()
	defer saveGlobals()()
	cacheDir = filepath.Join(baseDir, "nonexistent")

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	cacheInfoCmd.RunE(nil, nil)

	w.Close()
	os.Stdout = old
	out, _ := io.ReadAll(r)
	r.Close()

	if !strings.Contains(string(out), "not yet created") {
		t.Errorf("expected 'not yet created', got: %s", string(out))
	}
}

func TestCacheClearCmd_Empty(t *testing.T) {
	baseDir := t.TempDir()
	defer saveGlobals()()
	cacheDir = baseDir

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	cacheClearCmd.RunE(nil, nil)

	w.Close()
	os.Stdout = old
	out, _ := io.ReadAll(r)
	r.Close()

	if !strings.Contains(string(out), "Cleared 0") {
		t.Errorf("expected 'Cleared 0', got: %s", string(out))
	}
}

func TestCacheClearCmd_WithFiles(t *testing.T) {
	baseDir := t.TempDir()
	defer saveGlobals()()
	cacheDir = baseDir
	os.WriteFile(filepath.Join(baseDir, "abc.gz"), make([]byte, 100), 0644)
	os.WriteFile(filepath.Join(baseDir, "def.gz"), make([]byte, 200), 0644)

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	cacheClearCmd.RunE(nil, nil)

	w.Close()
	os.Stdout = old
	out, _ := io.ReadAll(r)
	r.Close()

	s := string(out)
	if !strings.Contains(s, "Cleared 2") {
		t.Errorf("expected 'Cleared 2', got: %s", s)
	}
	if !strings.Contains(s, "300 B") {
		t.Errorf("expected '300 B', got: %s", s)
	}
}
