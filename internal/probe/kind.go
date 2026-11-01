// SPDX-License-Identifier: AGPL-3.0-only

package probe

import (
	"path/filepath"
	"strings"
)

// Kind is what a file is, decided by extension, not by probing. The store
// and the queue carry a Kind; only internal/probe, internal/encoders and
// internal/flow know what a Kind means for encoding.
type Kind string

const (
	KindVideo    Kind = "video"
	KindAudio    Kind = "audio"
	KindSubtitle Kind = "subtitle"
	KindImage    Kind = "image"
	KindOther    Kind = "other"
)

// kindExtensions maps a file extension (lowercase, with the dot) to the
// Kind it belongs to. Only internal/probe, internal/encoders and
// internal/flow know what a Kind means for encoding (AGENTS.md rule 9).
var kindExtensions = map[string]Kind{
	".mkv": KindVideo, ".mp4": KindVideo, ".avi": KindVideo,
	".mov": KindVideo, ".m4v": KindVideo, ".ts": KindVideo,
	".wmv": KindVideo, ".flv": KindVideo, ".webm": KindVideo,

	".mp3": KindAudio, ".flac": KindAudio, ".aac": KindAudio,
	".m4a": KindAudio, ".ogg": KindAudio, ".opus": KindAudio,
	".wav": KindAudio,

	".srt": KindSubtitle, ".ass": KindSubtitle, ".ssa": KindSubtitle,
	".sub": KindSubtitle, ".vtt": KindSubtitle, ".idx": KindSubtitle,
	".sup": KindSubtitle, ".pgs": KindSubtitle, ".smi": KindSubtitle,

	".jpg": KindImage, ".jpeg": KindImage, ".png": KindImage,
	".gif": KindImage, ".webp": KindImage, ".avif": KindImage,
	".heic": KindImage, ".heif": KindImage, ".tiff": KindImage,
	".bmp": KindImage,
}

// KindForPath returns the Kind for a path, decided by its extension. An
// unknown extension is KindOther, never an error.
func KindForPath(p string) Kind {
	if kind, ok := kindExtensions[strings.ToLower(filepath.Ext(p))]; ok {
		return kind
	}
	return KindOther
}

// IsMedia reports whether a kind is probed and encoded (video or audio).
// Subtitles, images and other files never reach an encoder.
func IsMedia(k Kind) bool {
	return k == KindVideo || k == KindAudio
}

// KindValues lists the valid kinds.
func KindValues() []Kind {
	return []Kind{KindVideo, KindAudio, KindSubtitle, KindImage, KindOther}
}
