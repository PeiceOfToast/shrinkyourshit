// SPDX-License-Identifier: AGPL-3.0-only

package probe

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
)

// fingerprintSample is how many bytes are hashed from the head and tail of
// a file. Files at or below this size are hashed whole.
const fingerprintSample = 1 << 20 // 1 MiB

// Fingerprint identifies a file by size, mtime (unix seconds) and a SHA-256
// over its first and last 1 MiB. It is media-agnostic and cheap for huge
// files, which matters because it runs against library files on every scan.
// Format: "size:mtime:hexhash".
func Fingerprint(path string) (string, error) {
	st, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("probe: fingerprint %s: %w", path, err)
	}

	h := sha256.New()
	if st.Size() <= fingerprintSample {
		if err := hashFilePart(h, path, 0, st.Size()); err != nil {
			return "", err
		}
	} else {
		if err := hashFilePart(h, path, 0, fingerprintSample); err != nil {
			return "", err
		}
		if err := hashFilePart(h, path, st.Size()-fingerprintSample, fingerprintSample); err != nil {
			return "", err
		}
	}
	return fmt.Sprintf("%d:%d:%s", st.Size(), st.ModTime().Unix(), hex.EncodeToString(h.Sum(nil))), nil
}

// hashFilePart hashes length bytes starting at offset.
func hashFilePart(h io.Writer, path string, offset, length int64) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("probe: fingerprint %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()
	if offset > 0 {
		if _, err := f.Seek(offset, io.SeekStart); err != nil {
			return fmt.Errorf("probe: fingerprint %s: %w", path, err)
		}
	}
	if _, err := io.CopyN(h, f, length); err != nil {
		return fmt.Errorf("probe: fingerprint %s: %w", path, err)
	}
	return nil
}
