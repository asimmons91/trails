# Shared helpers for the release-related mise tasks (version-next,
# release-dry-run, release). Sourced, not executed directly — kept outside
# .mise/tasks/ so mise doesn't treat it as an invocable task.

# Reuses the exact submodule-discovery pattern already used by
# .mise/tasks/{vet,lint,test}.
mapfile -t SUBMODULES < <(find . -mindepth 2 -name go.mod | xargs -n1 dirname | sed 's|^\./||' | sort)

# Must match cliff.toml's [git] tag_pattern, so path-prefixed submodule tags
# (e.g. pack/driver/mysql/v0.1.0) are never mistaken for a root release tag.
ROOT_TAG_REGEX='^v[0-9]+\.[0-9]+\.[0-9]+$'

latest_root_tag() {
  git tag -l 'v*' | grep -E "$ROOT_TAG_REGEX" | sort -V | tail -n1
}

# git-cliff already handles both the bootstrap case (cliff.toml's
# [bump] initial_tag) and the "no conventional commits since last tag"
# case (falls back to a patch bump on its own), so no extra fallback logic
# is needed here beyond letting its exit code propagate.
compute_next_version() {
  git cliff --bumped-version 2>/dev/null
}

render_notes() {
  local next_version="$1"
  local current
  current=$(latest_root_tag)

  echo "Release ${next_version} - all trails modules bump together:"
  echo "- github.com/asimmons91/trails ${next_version}"
  for m in "${SUBMODULES[@]}"; do
    echo "- github.com/asimmons91/trails/${m} ${next_version}"
  done
  echo

  if [ -n "$current" ]; then
    git cliff --tag "$next_version" --strip header "${current}..HEAD"
  else
    git cliff --tag "$next_version" --strip header
  fi
}
