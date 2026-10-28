#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
# Fails when a Go or TypeScript source file does not start with the SPDX
# license header required by AGENTS.md rule 7.
set -euo pipefail

header='// SPDX-License-Identifier: AGPL-3.0-only'
missing=0

while IFS= read -r -d '' file; do
  if [[ "$(head -n 1 "$file")" != "$header" ]]; then
    echo "missing SPDX header (first line must be exactly: $header): $file" >&2
    missing=1
  fi
done < <(
  find . \
    \( -type d \( \
      -name .git \
      -o -name node_modules \
      -o -name dist \
      -o -name coverage \
      -o -name bin \
    \) -prune \) \
    -o -type f \( -name '*.go' -o -name '*.ts' -o -name '*.tsx' \) -print0
)

exit "$missing"
