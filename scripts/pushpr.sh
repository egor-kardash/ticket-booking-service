#!/usr/bin/env bash
#
# Пуш текущей ветки и создание PR с заголовком и описанием из коммитов.
# Требуется gh (https://cli.github.com) и выполненный `gh auth login`.
#
# Использование:
#   scripts/pushpr.sh
#   BASE=develop scripts/pushpr.sh
#   DRAFT=1 scripts/pushpr.sh
#   TITLE="feat(auth): add authentication" scripts/pushpr.sh
#
set -euo pipefail

base="${BASE:-main}"
branch="$(git rev-parse --abbrev-ref HEAD)"

if [ "$branch" = "$base" ]; then
  echo "refusing to create a PR from $base" >&2
  exit 1
fi

if ! command -v gh >/dev/null 2>&1; then
  echo "gh is not installed" >&2
  exit 1
fi

git push -u origin "$branch"

if existing="$(gh pr view --json url --jq .url 2>/dev/null)"; then
  echo "PR already exists: $existing"
  exit 0
fi

subjects=()
while IFS= read -r subject; do
  subjects+=("$subject")
done < <(git log --reverse --format=%s "$base"..HEAD)

if [ "${#subjects[@]}" -eq 0 ]; then
  echo "no commits between $base and $branch" >&2
  exit 1
fi

title="${TITLE:-${subjects[0]}}"

body=""
for subject in "${subjects[@]}"; do
  item="$(printf '%s' "$subject" | sed -E 's/^[a-zA-Z]+(\([^)]*\))?!?: //')"
  body+="- ${item}"$'\n'
done

args=(--base "$base" --head "$branch" --title "$title" --body "$body")
if [ "${DRAFT:-0}" = "1" ]; then
  args+=(--draft)
fi

gh pr create "${args[@]}"
