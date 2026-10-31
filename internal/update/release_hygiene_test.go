// SPDX-License-Identifier: AGPL-3.0-only

package update_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// task 26 acceptance is mostly release plumbing and documentation, which
// unit tests can only pin statically. The executable proof for the image
// itself is the docker job in .github/workflows/ci.yml (every PR) and the
// smoke check in release.yml (every tag); docker is not required here.

func TestDockerfileCarriesBuildIdentity(t *testing.T) {
	body := readHygieneDoc(t, filepath.Join("..", "..", "docker", "Dockerfile"))

	for _, want := range []string{
		"ARG VERSION",
		"ARG COMMIT",
		"ARG BUILD_DATE",
		"-X main.version=${VERSION}",
		"-X main.commit=${COMMIT}",
		"-X main.date=${BUILD_DATE}",
	} {
		assert.Contains(t, body, want, "docker/Dockerfile must carry the build identity")
	}

	// The stale arm64 premise is corrected in the source of the image.
	assert.NotContains(t, body, "has no arm64 build")
	assert.Contains(t, body, "never been built or tested for arm64")
	assert.Contains(t, body, "docs/container-architectures.md")
}

func TestReleaseWorkflowCarriesBuildIdentity(t *testing.T) {
	body := readHygieneDoc(t, filepath.Join("..", "..", ".github", "workflows", "release.yml"))

	for _, want := range []string{
		"VERSION=${{ needs.prepare.outputs.version }}",
		"COMMIT=${{ needs.prepare.outputs.commit }}",
		"BUILD_DATE=${{ needs.prepare.outputs.build_date }}",
		"org.opencontainers.image.created=${{ needs.prepare.outputs.build_date }}",
		"org.opencontainers.image.revision=${{ needs.prepare.outputs.commit }}",
		"Smoke test the version endpoint",
		"EXPECTED_COMMIT: ${{ needs.prepare.outputs.commit }}",
		`[ "$commit" != "$EXPECTED_COMMIT" ]`,
		`[ "$build_date" = "unknown" ]`,
		"load: true",
		"name: Push image",
	} {
		assert.Contains(t, body, want, "release.yml must pass and verify the build identity")
	}

	// The smoke check must run after the build and before the push, so a
	// bad image can never reach the registry.
	build := strings.Index(body, "Build image (loaded locally, not pushed)")
	smoke := strings.Index(body, "Smoke test the version endpoint")
	push := strings.Index(body, "name: Push image")
	require.NotEqual(t, -1, build)
	require.NotEqual(t, -1, smoke)
	require.NotEqual(t, -1, push)
	assert.Less(t, build, smoke)
	assert.Less(t, smoke, push)
}

func TestCIDockerJobProvesBuildIdentity(t *testing.T) {
	body := readHygieneDoc(t, filepath.Join("..", "..", ".github", "workflows", "ci.yml"))

	for _, want := range []string{
		`--build-arg VERSION="v0.0.0-ci"`,
		`--build-arg COMMIT="$GITHUB_SHA"`,
		`--build-arg BUILD_DATE="$BUILD_DATE"`,
		"Image reports version, commit and build date",
		"EXPECTED_COMMIT: ${{ github.sha }}",
		`[ "$(read_field commit)" = "$EXPECTED_COMMIT" ]`,
		`[ "$build_date" = "unknown" ]`,
	} {
		assert.Contains(t, body, want, "the CI docker job is the PR regression gate for task 26")
	}
}

func TestChangelogMatchesGitHistory(t *testing.T) {
	body := readHygieneDoc(t, filepath.Join("..", "..", "CHANGELOG.md"))

	// Newest first, and the three versions the task names.
	unreleased := strings.Index(body, "## [0.1.2] - unreleased")
	older := strings.Index(body, "## [0.1.1] - 2026-10-30")
	oldest := strings.Index(body, "## [0.1.0] - 2026-10-29")
	require.NotEqual(t, -1, unreleased)
	require.NotEqual(t, -1, older)
	require.NotEqual(t, -1, oldest)
	assert.Less(t, unreleased, older)
	assert.Less(t, older, oldest)

	for _, want := range []string{
		"https://github.com/PeiceOfToast/shrinkyourshit/compare/v0.1.1...HEAD",
		"https://github.com/PeiceOfToast/shrinkyourshit/compare/v0.1.0...v0.1.1",
		"https://github.com/PeiceOfToast/shrinkyourshit/releases/tag/v0.1.0",
		// 0.1.0 shipped the MVP but no archives because of this.
		"Windows/amd64 binary did not compile",
		// 0.1.1 is the release-engineering release.
		"checksums.txt",
		"update --rollback",
		// Task 20 shipped in v0.1.1 (tag is after commit ce04748); the
		// mobile audit and 44px controls belong there, not unreleased.
		"44px",
		// The unreleased section covers the real v1.6 wave.
		"Health check",
		"webhooks",
		"OpenAPI",
		"commit and build date",
	} {
		assert.Contains(t, squashWhitespace(body), want)
	}
}

func TestReleasingProcedureIsComplete(t *testing.T) {
	body := readHygieneDoc(t, filepath.Join("..", "..", "docs", "releasing.md"))

	for _, want := range []string{
		"## 1. Bump the version where it lives",
		"## 2. Update `CHANGELOG.md`",
		"## 3. Run the pipeline on demand",
		"## 4. Confirm all five archives build",
		"## 5. Tag",
		"## 6. Verify the GitHub release",
		"## 7. Verify both image tags pull anonymously",
		"## 8. Verify the container's build identity, then update",
		"## 9. Rollback",
		"gh workflow run release.yml --ref main",
		"gh run watch",
		`git tag -a v0.1.2 -m "ShrinkYourShit v0.1.2"`,
		"git push origin v0.1.2",
		"sha256sum -c checksums.txt --ignore-missing",
		"docker logout ghcr.io",
		"docker compose pull",
		"docker compose up -d",
		"docker buildx imagetools create",
	} {
		assert.Contains(t, body, want, "docs/releasing.md must be copy-pasteable")
	}

	// Step 1 must name where the version actually lives.
	assert.Contains(t, body, "not stored in a source file")
	assert.Contains(t, body, "git tag")

	// FFmpeg pin maintenance: the exact URL and digest, the rolling
	// channel, and the intentionally loud failure.
	for _, want := range []string{
		"https://repo.jellyfin.org/files/ffmpeg/debian/latest-7.x/amd64/jellyfin-ffmpeg7_7.1.4-3-bookworm_amd64.deb",
		"0cf62bc2423822c9ec7a38dbc8f526d9a58671bd01843daa7817ece35619fc1c",
		"rolling channel",
		"fails loudly by design",
		"sha256sum jellyfin-ffmpeg7_7.1.4-3-bookworm_amd64.deb",
		"| sort -u",
	} {
		assert.Contains(t, body, want, "the FFmpeg pin procedure must stay accurate")
	}
}

func TestContainerArchitecturesStatesArm64Facts(t *testing.T) {
	body := readHygieneDoc(t, filepath.Join("..", "..", "docs", "container-architectures.md"))

	for _, want := range []string{
		// The URL that was checked and what it returned.
		"https://repo.jellyfin.org/files/ffmpeg/debian/latest-7.x/arm64/jellyfin-ffmpeg7_7.1.4-3-bookworm_arm64.deb",
		"HTTP 200",
		"15,555,600 bytes",
		// The corrected reason, not the false one.
		"never been built or tested for arm64",
		"That was wrong",
		// Both options and exactly one recommendation.
		"Option A",
		"Option B",
		"## Recommendation",
		"linux/arm64` binary",
	} {
		assert.Contains(t, squashWhitespace(body), want)
	}
}

// squashWhitespace collapses markdown line wrapping so prose assertions
// do not fail when a sentence is rewrapped. Structural assertions (heading
// order, code blocks) still use the raw file.
func squashWhitespace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// readHygieneDoc reads a repository file relative to this package. The
// tests deliberately read the real files: a doc or workflow that drifts
// from the release procedure should fail make test, not a review.
func readHygieneDoc(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err, "read %s", path)
	return string(data)
}
