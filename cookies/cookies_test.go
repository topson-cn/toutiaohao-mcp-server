package cookies

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCookieSaveAndLoad(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "cookies.json")
	store := NewFileCookieStore(path)

	data := []byte(`[{"name":"sid","value":"abc123"}]`)
	if err := store.SaveCookies(data); err != nil {
		t.Fatalf("SaveCookies failed: %v", err)
	}

	loaded, err := store.LoadCookies()
	if err != nil {
		t.Fatalf("LoadCookies failed: %v", err)
	}
	if string(loaded) != string(data) {
		t.Errorf("LoadCookies = %s, want %s", loaded, data)
	}
}

func TestCookieLoadFileNotExist(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "nonexistent.json")
	store := NewFileCookieStore(path)

	data, err := store.LoadCookies()
	if err != nil {
		t.Fatalf("LoadCookies should not error for missing file: %v", err)
	}
	if data != nil {
		t.Errorf("LoadCookies = %s, want nil", data)
	}
}

func TestCookieDeleteFile(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "cookies.json")
	store := NewFileCookieStore(path)

	// Create the file first
	if err := os.WriteFile(path, []byte("test"), 0644); err != nil {
		t.Fatalf("setup: WriteFile failed: %v", err)
	}

	if err := store.DeleteCookies(); err != nil {
		t.Fatalf("DeleteCookies failed: %v", err)
	}

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("Cookie file should be deleted, but still exists")
	}
}

func TestCookieSaveUsesOwnerOnlyPermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private", "cookies.json")
	store := NewFileCookieStore(path)
	if err := store.SaveCookies([]byte(`[{"name":"sid","value":"secret"}]`)); err != nil {
		t.Fatalf("SaveCookies failed: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat failed: %v", err)
	}
	if got := info.Mode().Perm(); got != 0600 {
		t.Fatalf("cookie mode = %#o, want 0600", got)
	}

	dirInfo, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatalf("directory Stat failed: %v", err)
	}
	if got := dirInfo.Mode().Perm(); got != 0700 {
		t.Fatalf("cookie directory mode = %#o, want 0700", got)
	}
}

func TestCookieSaveTightensExistingFilePermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cookies.json")
	if err := os.WriteFile(path, []byte("old"), 0644); err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	store := NewFileCookieStore(path)
	if err := store.SaveCookies([]byte("new")); err != nil {
		t.Fatalf("SaveCookies failed: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0600 {
		t.Fatalf("cookie mode = %#o, want 0600", got)
	}
}
