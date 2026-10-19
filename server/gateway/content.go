package gateway

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ManifestFile describes one content file in the bundle.
type ManifestFile struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Size   int    `json:"size"`
}

// Content is the loaded content manifest. It is read-only after LoadContent.
type Content struct {
	version   string
	files     []ManifestFile
	bundleURL string
}

// Version returns the content version (hash over all file hashes).
func (c *Content) Version() string { return c.version }

// LoadContent hashes *.yaml and schema/*.json under dir. Line endings are normalised so
// the version does not depend on the checkout platform. Files are visited in lexical order.
func LoadContent(dir, bundleURL string) (*Content, error) {
	var files []ManifestFile
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		ext := strings.ToLower(filepath.Ext(p))
		if d.IsDir() || (ext != ".yaml" && ext != ".json") {
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		b = bytes.ReplaceAll(b, []byte("\r\n"), []byte("\n"))
		rel, _ := filepath.Rel(dir, p)
		sum := sha256.Sum256(b)
		files = append(files, ManifestFile{Path: filepath.ToSlash(rel), SHA256: hex.EncodeToString(sum[:]), Size: len(b)})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("content: %w", err)
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	h := sha256.New()
	for _, f := range files {
		fmt.Fprintf(h, "%s:%s\n", f.Path, f.SHA256)
	}
	return &Content{version: hex.EncodeToString(h.Sum(nil))[:16], files: files, bundleURL: bundleURL}, nil
}
