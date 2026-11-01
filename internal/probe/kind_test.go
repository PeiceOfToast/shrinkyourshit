// SPDX-License-Identifier: AGPL-3.0-only

package probe

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestKindForPath(t *testing.T) {
	cases := []struct {
		path string
		want Kind
	}{
		{"movie.mkv", KindVideo},
		{"movie.mp4", KindVideo},
		{"movie.avi", KindVideo},
		{"show.MOV", KindVideo},
		{"song.mp3", KindAudio},
		{"song.flac", KindAudio},
		{"song.opus", KindAudio},
		{"subs.srt", KindSubtitle},
		{"subs.ass", KindSubtitle},
		{"subs.ssa", KindSubtitle},
		{"subs.sup", KindSubtitle},
		{"photo.jpg", KindImage},
		{"photo.JPG", KindImage},
		{"photo.jpeg", KindImage},
		{"photo.png", KindImage},
		{"photo.webp", KindImage},
		{"photo.avif", KindImage},
		{"photo.heic", KindImage},
		{"photo.tiff", KindImage},
		{"photo.bmp", KindImage},
		{"notes.txt", KindOther},
		{"archive.zip", KindOther},
		{"/abs/path/to/movie.mkv", KindVideo},
		{"/abs/path/to/notes.txt", KindOther},
		{"", KindOther},
	}
	for _, c := range cases {
		assert.Equal(t, c.want, KindForPath(c.path), "path %q", c.path)
	}
}

func TestIsMedia(t *testing.T) {
	assert.True(t, IsMedia(KindVideo))
	assert.True(t, IsMedia(KindAudio))
	assert.False(t, IsMedia(KindSubtitle))
	assert.False(t, IsMedia(KindImage))
	assert.False(t, IsMedia(KindOther))
}

func TestKindValues(t *testing.T) {
	assert.Equal(t,
		[]Kind{KindVideo, KindAudio, KindSubtitle, KindImage, KindOther},
		KindValues())
}
