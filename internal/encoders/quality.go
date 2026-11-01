// SPDX-License-Identifier: AGPL-3.0-only

package encoders

import (
	"strconv"

	"github.com/PeiceOfToast/shrinkyourshit/internal/flow"
)

// qualityValues holds the quality numbers for one step of the easy-mode
// slider, one field per table column in ARCHITECTURE.md.
type qualityValues struct {
	X265CRF       int // libx265 CRF (preset medium)
	NVENCCQ       int // hevc_nvenc CQ (preset p5)
	QSVGQ         int // hevc_qsv global_quality
	SVTAV1CRF     int // libsvtav1 CRF (preset 6)
	X264CRF       int // libx264 CRF
	VideoToolboxQ int // videotoolbox -q:v constant quality
}

// qualityTable is the quality step table from ARCHITECTURE.md, indexed by
// the 1-based quality step (index 0 is unused). Step 1 saves the most
// space, step 5 the least. Tune it in this one place.
//
// Columns the table in ARCHITECTURE.md does not define are derived here:
//   - NVENC h264 and av1 reuse the NVENC HEVC CQ column.
//   - QSV: the QSV column is ICQ (task 35). The builder passes the value
//     as -global_quality, which ffmpeg maps to Intel's intelligent
//     constant-quality rate control (ICQ) whenever no bitrate cap is set:
//     the encoder holds a constant quality and the file size follows the
//     content. ICQQuality runs 1 (best quality, largest files) to 51
//     (worst, smallest); the slider's 1..5 maps onto 30..22, so step 1
//     saves the most space (ICQ 30, lowest quality ceiling) and step 5
//     keeps the most quality (ICQ 22, least saving). hevc_qsv and av1_qsv
//     take this ICQ mapping. h264_qsv keeps the same values as its
//     fallback path — ICQ support for h264 varies by Intel generation, and
//     switching h264 to a different mapping would change encodes the user
//     never asked to change, so the builder never does.
//   - VAAPI (the vaapi backend and the qsv backend's fallback variant)
//     reuses the QSV global_quality column: -global_quality for hevc and
//     av1, and -qp for h264 (h264_vaapi has no global_quality mode).
//   - VideoToolbox reuses no other column: its constant quality is its own
//     1..100 scale where higher means better, so the easy steps 1..5 map to
//     30..90, leaving headroom at both ends.
var qualityTable = [flow.QualityMax + 1]qualityValues{
	{},
	{X265CRF: 28, NVENCCQ: 32, QSVGQ: 30, SVTAV1CRF: 38, X264CRF: 26, VideoToolboxQ: 30},
	{X265CRF: 26, NVENCCQ: 30, QSVGQ: 28, SVTAV1CRF: 35, X264CRF: 24, VideoToolboxQ: 45},
	{X265CRF: 24, NVENCCQ: 28, QSVGQ: 26, SVTAV1CRF: 32, X264CRF: 22, VideoToolboxQ: 60},
	{X265CRF: 22, NVENCCQ: 26, QSVGQ: 24, SVTAV1CRF: 30, X264CRF: 20, VideoToolboxQ: 75},
	{X265CRF: 20, NVENCCQ: 24, QSVGQ: 22, SVTAV1CRF: 27, X264CRF: 18, VideoToolboxQ: 90},
}

// qualityFor returns the table row for a quality step, clamped into the
// valid 1..5 range so a bad Target can only degrade, never panic.
func qualityFor(step int) qualityValues {
	if step < 1 {
		step = 1
	}
	if step > flow.QualityMax {
		step = flow.QualityMax
	}
	return qualityTable[step]
}

// itoa is a tiny helper for building numeric arguments.
func itoa(n int) string {
	return strconv.Itoa(n)
}
