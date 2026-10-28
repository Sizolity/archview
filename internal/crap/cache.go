package crap

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"

	"github.com/Sizolity/archview/internal/cache"
)

// A CRAP run costs a `go test -cover` of the whole module, so the report is
// cached on disk, keyed by everything it depends on: the tree fingerprint, the
// go-crap binary itself and the extra flags.
//
// The cache lives in the user's cache directory, never in the inspected module:
// archview reads projects and does not write into them. Point -cache-dir
// somewhere else (or at the module) only on purpose.
const cacheVersion = 1

type cacheEntry struct {
	Version     int             `json:"version"`
	Fingerprint string          `json:"fingerprint"`
	Args        []string        `json:"args"`
	Tool        string          `json:"tool"`
	ToolSize    int64           `json:"tool_size"`
	ToolMtime   int64           `json:"tool_mtime"`
	Report      json.RawMessage `json:"report"`
}

// CollectCached is Collect with a disk cache in front of it. A missing tool, an
// unreadable cache or a fingerprint miss all fall back to a real run; a failure
// to write the cache is ignored, it only costs time next time.
func CollectCached(ctx context.Context, dir, command string, args []string, cacheDir string) *Bundle {
	if cacheDir == "" {
		return Collect(ctx, dir, command, args)
	}
	fp, err := cache.FingerprintOf(dir)
	if err != nil {
		return Collect(ctx, dir, command, args)
	}
	path := cachePath(dir, cacheDir)
	entry := cacheEntry{
		Version:     cacheVersion,
		Fingerprint: fp,
		Args:        args,
	}
	entry.Tool, entry.ToolSize, entry.ToolMtime = toolIdentity(command)

	if raw := readEntry(path, entry); raw != nil {
		if b, err := bundleFromReport(raw); err == nil {
			b.Cached = true
			return b
		}
	}

	b, raw := collect(ctx, dir, command, args)
	if raw != nil && len(b.Warnings) == 0 {
		entry.Report = raw
		writeEntry(path, entry)
	}
	return b
}

// cachePath is one file per module root inside the cache directory.
func cachePath(dir, cacheDir string) string {
	abs, err := filepath.Abs(dirOrDot(dir))
	if err != nil {
		abs = dirOrDot(dir)
	}
	sum := sha256.Sum256([]byte(abs))
	return filepath.Join(cacheDir, "crap-"+hex.EncodeToString(sum[:8])+".json")
}

func dirOrDot(dir string) string {
	if dir == "" {
		return "."
	}
	return dir
}

// toolIdentity identifies the go-crap binary without running it: an upgrade
// replaces the file, which changes size and mtime.
func toolIdentity(command string) (string, int64, int64) {
	if command == "" {
		command = "go-crap"
	}
	path, err := exec.LookPath(command)
	if err != nil {
		return command, 0, 0
	}
	st, err := os.Stat(path)
	if err != nil {
		return path, 0, 0
	}
	return path, st.Size(), st.ModTime().UnixNano()
}

func readEntry(path string, want cacheEntry) json.RawMessage {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var got cacheEntry
	if err := json.Unmarshal(data, &got); err != nil {
		return nil
	}
	if got.Version != want.Version || got.Fingerprint != want.Fingerprint ||
		!slices.Equal(got.Args, want.Args) || got.Tool != want.Tool ||
		got.ToolSize != want.ToolSize || got.ToolMtime != want.ToolMtime {
		return nil
	}
	return got.Report
}

func writeEntry(path string, e cacheEntry) {
	data, err := json.Marshal(e)
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "crap-*.tmp")
	if err != nil {
		return
	}
	name := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(name)
		return
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(name)
		return
	}
	_ = os.Rename(name, path)
}
