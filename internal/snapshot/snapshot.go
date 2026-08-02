package snapshot

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const MaxFiles = 2000
const MaxFileBytes = 8 * 1024 * 1024
const MaxTotalBytes = 32 * 1024 * 1024

var ignored = map[string]bool{".git": true, ".venv": true, ".pytest_cache": true, ".ruff_cache": true, "__pycache__": true}

type Error struct{ code string }

func (err *Error) Error() string { return err.code }
func fail(code string) error     { return &Error{code: code} }
func Code(err error) string {
	var target *Error
	if errors.As(err, &target) {
		return target.code
	}
	return ""
}

type entry struct {
	relative, path string
	info           fs.FileInfo
}

func TreeDigest(root string) (string, error) {
	entries, err := collect(root)
	if err != nil {
		return "", err
	}
	digest := sha256.New()
	var fileCount, totalBytes int64
	var length [8]byte
	for _, item := range entries {
		kind := byte('D')
		payload := []byte{}
		if item.info.Mode().IsRegular() {
			kind = 'F'
			fileCount++
			if fileCount > MaxFiles || item.info.Size() > MaxFileBytes {
				return "", fail("target_too_large")
			}
			totalBytes += item.info.Size()
			if totalBytes > MaxTotalBytes {
				return "", fail("target_too_large")
			}
			payload, err = os.ReadFile(item.path)
			if err != nil {
				return "", fail("target_unavailable")
			}
		} else if !item.info.IsDir() {
			return "", fail("special_file_unsupported")
		}
		digest.Write([]byte{kind})
		binary.BigEndian.PutUint64(length[:], uint64(len([]byte(filepath.ToSlash(item.relative)))))
		digest.Write(length[:])
		digest.Write([]byte(filepath.ToSlash(item.relative)))
		binary.BigEndian.PutUint64(length[:], uint64(len(payload)))
		digest.Write(length[:])
		digest.Write(payload)
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}

func CopyTarget(source, destination string) error {
	if _, err := TreeDigest(source); err != nil {
		return err
	}
	rootInfo, err := os.Stat(source)
	if err != nil || !rootInfo.IsDir() {
		return fail("target_unavailable")
	}
	if err := os.Mkdir(destination, rootInfo.Mode().Perm()); err != nil {
		return err
	}
	entries, err := collect(source)
	if err != nil {
		return err
	}
	for _, item := range entries {
		target := filepath.Join(destination, item.relative)
		if item.info.IsDir() {
			if err := os.Mkdir(target, item.info.Mode().Perm()); err != nil {
				return err
			}
			continue
		}
		data, err := os.ReadFile(item.path)
		if err != nil {
			return err
		}
		if err := os.WriteFile(target, data, item.info.Mode().Perm()); err != nil {
			return err
		}
	}
	return nil
}

func collect(root string) ([]entry, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, fail("target_unavailable")
	}
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return nil, fail("target_unavailable")
	}
	var entries []entry
	err = filepath.WalkDir(root, func(path string, directory fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == root {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		for _, part := range strings.Split(filepath.ToSlash(relative), "/") {
			if ignored[part] {
				if directory.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
		}
		if directory.Type()&os.ModeSymlink != 0 {
			return fail("symlink_unsupported")
		}
		info, err := directory.Info()
		if err != nil {
			return err
		}
		entries = append(entries, entry{relative: relative, path: path, info: info})
		return nil
	})
	if err != nil {
		var snapshotError *Error
		if errors.As(err, &snapshotError) {
			return nil, err
		}
		return nil, fail("target_unavailable")
	}
	sort.Slice(entries, func(i, j int) bool {
		return filepath.ToSlash(entries[i].relative) < filepath.ToSlash(entries[j].relative)
	})
	return entries, nil
}
