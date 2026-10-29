// SPDX-License-Identifier: AGPL-3.0-only

package worker

import (
	"os"
	"path/filepath"
	"strings"
)

// scratch owns the worker's temp space: one directory per job under the
// scratch root. Every encode happens here, never on the library (rule 10).
type scratch struct {
	root string
}

func newScratch(root string) *scratch {
	return &scratch{root: root}
}

// jobDir is the temp directory of one job.
func (s *scratch) jobDir(jobID string) string {
	return filepath.Join(s.root, "job-"+jobID)
}

// ensure creates the job directory.
func (s *scratch) ensure(jobID string) (string, error) {
	dir := s.jobDir(jobID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

// remove deletes a job directory (cleanup on success, failure and cancel).
func (s *scratch) remove(jobID string) {
	_ = os.RemoveAll(s.jobDir(jobID))
}

// cleanupOrphans removes everything under the scratch root. It runs on
// worker start: leftovers belong to jobs of a dead worker process (the
// server requeues those via the heartbeat timeout).
func (s *scratch) cleanupOrphans() {
	entries, err := os.ReadDir(s.root)
	if err != nil {
		return // root may not exist yet; nothing to clean
	}
	for _, e := range entries {
		_ = os.RemoveAll(filepath.Join(s.root, e.Name()))
	}
}

// extractFiles lists the subtitle extracts the runner produced for an
// output named base (for example output.eng.srt beside output.mkv).
func (s *scratch) extractFiles(dir, base string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasPrefix(name, base+".") || !strings.HasSuffix(name, ".srt") {
			continue
		}
		out = append(out, filepath.Join(dir, name))
	}
	return out
}
