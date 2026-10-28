// SPDX-License-Identifier: AGPL-3.0-only

package encoders

import (
	"strconv"

	"github.com/PeiceOfToast/shrinkyourshit/internal/flow"
)

// qualityValues holds the quality numbers for one step of the easy-mode
// slider, one field per table column in ARCHITECTURE.md.
type qualityValues struct {
	X265CRF   int // libx265 CRF (preset medium)
	NVENCCQ   int // hevc_nvenc CQ (preset p5)
	QSVGQ     int // hevc_qsv global_quality
	SVTAV1CRF int // libsvtav1 CRF (preset 6)
	X264CRF   int // libx264 CRF
}

// qualityTable is the quality step table from ARCHITECTURE.md, indexed by
// the 1-based quality step (index 0 is unused). Step 1 saves the most
// space, step 5 the least. Tune it in this one place.
//
// Columns the table in ARCHITECTURE.md does not define are derived here:
//   - NVENC h264 and av1 reuse the NVENC HEVC CQ column.
//   - QSV h264 and av1 reuse the QSV HEVC global_quality column.
//   - VAAPI (the qsv backend's fallback variant) reuses global_quality for
//     hevc and av1, and -qp for h264.
var qualityTable = [flow.QualityMax + 1]qualityValues{
	{},
	{X265CRF: 28, NVENCCQ: 32, QSVGQ: 30, SVTAV1CRF: 38, X264CRF: 26},
	{X265CRF: 26, NVENCCQ: 30, QSVGQ: 28, SVTAV1CRF: 35, X264CRF: 24},
	{X265CRF: 24, NVENCCQ: 28, QSVGQ: 26, SVTAV1CRF: 32, X264CRF: 22},
	{X265CRF: 22, NVENCCQ: 26, QSVGQ: 24, SVTAV1CRF: 30, X264CRF: 20},
	{X265CRF: 20, NVENCCQ: 24, QSVGQ: 22, SVTAV1CRF: 27, X264CRF: 18},
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
