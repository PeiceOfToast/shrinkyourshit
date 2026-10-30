// SPDX-License-Identifier: AGPL-3.0-only

package update_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func jsonEncoder(w http.ResponseWriter) func(any) error {
	return json.NewEncoder(w).Encode
}

// tarGzBytes builds a single-entry tar.gz, the layout the release script
// produces for the unix archives.
func tarGzBytes(t *testing.T, name string, content []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)
	require.NoError(t, tw.WriteHeader(&tar.Header{
		Name: name,
		Mode: 0o755,
		Size: int64(len(content)),
	}))
	_, err := tw.Write(content)
	require.NoError(t, err)
	require.NoError(t, tw.Close())
	require.NoError(t, gw.Close())
	return buf.Bytes()
}

func sha256Hex(t *testing.T, data []byte) string {
	t.Helper()
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
