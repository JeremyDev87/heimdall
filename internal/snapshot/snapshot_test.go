package snapshot

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDigestMatchesPythonOracleAndIgnoresCaches(t *testing.T) {
	root, _ := filepath.Abs(filepath.Join("..", ".."))
	target := filepath.Join(root, "fixtures", "pass", "target")
	got, err := TreeDigest(target)
	if err != nil {
		t.Fatal(err)
	}
	if got != "b1687698b415349f801df263ff5c2922567ffbc507a03da9a71473d308b87e87" {
		t.Fatalf("digest=%s", got)
	}
	cache := filepath.Join(target, "__pycache__")
	if err := os.Mkdir(cache, 0o700); err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(cache)
	if err := os.WriteFile(filepath.Join(cache, "ignored.pyc"), []byte("ignored"), 0o600); err != nil {
		t.Fatal(err)
	}
	again, err := TreeDigest(target)
	if err != nil {
		t.Fatal(err)
	}
	if again != got {
		t.Fatalf("ignored cache changed digest: %s", again)
	}
}
func TestSymlinkFailsClosed(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "file"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("file", filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	_, err := TreeDigest(root)
	if Code(err) != "symlink_unsupported" {
		t.Fatalf("error=%v", err)
	}
}
func BenchmarkTreeDigest(b *testing.B) {
	root, _ := filepath.Abs(filepath.Join("..", ".."))
	target := filepath.Join(root, "fixtures", "pass", "target")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := TreeDigest(target); err != nil {
			b.Fatal(err)
		}
	}
}
