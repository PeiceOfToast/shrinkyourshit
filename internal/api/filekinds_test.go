// SPDX-License-Identifier: AGPL-3.0-only

package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/PeiceOfToast/shrinkyourshit/internal/flow"
	"github.com/PeiceOfToast/shrinkyourshit/internal/server"
	"github.com/stretchr/testify/require"
)

// postLibrary posts a create-library body and returns the response.
func postLibrary(t *testing.T, h *apiHarness, body map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	return h.do(http.MethodPost, "/api/v1/libraries", body)
}

// TestLibraryFileKindsDefault (task 34): a create without file_kinds
// defaults to ["video"], and the kinds round-trip through GET.
func TestLibraryFileKindsDefault(t *testing.T) {
	h := newAPIHarness(t)
	rec := postLibrary(t, h, map[string]any{"path": h.root})
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	var dto server.LibraryDTO
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &dto))
	require.Equal(t, []string{"video"}, dto.FileKinds, "a default library reports [\"video\"]")

	got := h.do(http.MethodGet, "/api/v1/libraries/"+dto.ID, nil)
	require.Equal(t, http.StatusOK, got.Code, got.Body.String())
	var roundTrip server.LibraryDTO
	require.NoError(t, json.Unmarshal(got.Body.Bytes(), &roundTrip))
	require.Equal(t, []string{"video"}, roundTrip.FileKinds)
}

// TestLibraryFileKindsCreateAndUpdate (task 34): create and update accept
// file_kinds and the stored row carries them.
func TestLibraryFileKindsCreateAndUpdate(t *testing.T) {
	h := newAPIHarness(t)
	rec := postLibrary(t, h, map[string]any{"path": h.root, "file_kinds": []string{"image", "video"}})
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	var dto server.LibraryDTO
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &dto))
	require.Equal(t, []string{"image", "video"}, dto.FileKinds)

	row, err := h.st.GetLibrary(context.Background(), dto.ID)
	require.NoError(t, err)
	require.Equal(t, []string{"image", "video"}, row.FileKinds)

	target, err := json.Marshal(flow.Default())
	require.NoError(t, err)
	up := h.do(http.MethodPut, "/api/v1/libraries/"+dto.ID, map[string]any{
		"name": dto.Name, "path": h.root, "target": json.RawMessage(target), "file_kinds": []string{"audio"},
	})
	require.Equal(t, http.StatusOK, up.Code, up.Body.String())
	var updated server.LibraryDTO
	require.NoError(t, json.Unmarshal(up.Body.Bytes(), &updated))
	require.Equal(t, []string{"audio"}, updated.FileKinds)
}

// TestLibraryFileKindsValidation (task 34): an invalid kind, an empty list
// and a duplicate are each rejected with an error naming the field and the
// bad value.
func TestLibraryFileKindsValidation(t *testing.T) {
	h := newAPIHarness(t)

	cases := []struct {
		name  string
		kinds []string
		want  string // the bad value the message must name
	}{
		{name: "invalid kind", kinds: []string{"document"}, want: "document"},
		{name: "empty list", kinds: []string{}, want: "[]"},
		{name: "duplicate", kinds: []string{"video", "video"}, want: "video"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := postLibrary(t, h, map[string]any{"path": h.root, "file_kinds": tc.kinds})
			require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
			require.Contains(t, rec.Body.String(), "file_kinds", "the error names the field")
			require.Contains(t, rec.Body.String(), tc.want, "the error names the bad value")
		})
	}
}
