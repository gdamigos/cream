#!/usr/bin/env bash
# rebuilds the todo/inbox branch as main plus one docs/TODO.md line per pending issue and keeps one PR for it
# pending means open, labelled bug, feature or enhancement, not on main yet and within the spam limits
# the whole inbox is recomputed every run, so a skipped or cancelled run loses nothing
# needs GH_TOKEN, GH_REPO and OWNER
set -euo pipefail
export LC_ALL=C.UTF-8 # so the 60 character cut counts characters, not bytes

# spam limits, they don't apply to authors with write access and bots are never added
MIN_ACCOUNT_DAYS=14
MAX_PER_AUTHOR=3
MAX_TOTAL=20

branch="todo/inbox"
todo="docs/TODO.md"

git fetch --quiet origin main
git switch --quiet -C "$branch" origin/main
# start from main exactly, leftovers would read as items already on main
git reset --quiet --hard origin/main
grep -qx "## Bugs" "$todo" && grep -qx "## Improvements" "$todo" ||
  { echo "::error::$todo needs '## Bugs' and '## Improvements' sections"; exit 1; }

# oldest first, so the limits let earlier issues in before later ones
gh issue list --state open --limit 300 --search "label:bug,feature,enhancement sort:created-asc" \
  --json number,title,author,labels,url > "$RUNNER_TEMP/issues.json"

declare -A trusted age per_author
kept=0
body="$RUNNER_TEMP/body.md"
printf 'Pending TODO items from issues. Merge to accept all of them. To drop one, close its issue or remove its label and this PR updates itself.\n\n' > "$body"

while read -r issue; do
  number="$(jq -r .number <<< "$issue")"
  author="$(jq -r .author.login <<< "$issue")"
  if grep -qF "[#$number](" "$todo"; then continue; fi # already on main
  if [ "$(jq -r .author.is_bot <<< "$issue")" = true ]; then continue; fi

  if [ -z "${trusted[$author]:-}" ]; then
    perm="$(gh api "repos/$GH_REPO/collaborators/$author/permission" --jq .permission 2> /dev/null || echo none)"
    case "$perm" in admin | maintain | write) trusted[$author]=yes ;; *) trusted[$author]=no ;; esac
  fi
  if [ "${trusted[$author]}" = no ]; then
    if [ -z "${age[$author]:-}" ]; then
      created="$(gh api "users/$author" --jq .created_at)"
      age[$author]=$(( ($(date +%s) - $(date -d "$created" +%s)) / 86400 ))
    fi
    if [ "${age[$author]}" -lt "$MIN_ACCOUNT_DAYS" ]; then echo "waiting: #$number, account too new"; continue; fi
    if [ "${per_author[$author]:-0}" -ge "$MAX_PER_AUTHOR" ]; then echo "waiting: #$number, @$author is at the limit"; continue; fi
    if [ "$kept" -ge "$MAX_TOTAL" ]; then echo "waiting: #$number, inbox is full"; continue; fi
  fi

  if jq -e '.labels[] | select(.name == "bug")' <<< "$issue" > /dev/null; then heading="## Bugs"; else heading="## Improvements"; fi
  # the title is untrusted, keep it one plain line without markdown or html, cut at 60 characters
  title="$(jq -r .title <<< "$issue" | tr -d '\r\n' | sed 's/[][<>`\\|*_]//g')"
  if [ "${#title}" -gt 60 ]; then title="${title:0:57}..."; fi
  user="$(printf '%s' "$author" | tr -cd 'A-Za-z0-9-')"
  url="$(jq -r .url <<< "$issue")"
  # renders as "#N - title by @user", both linked, newest ends up first under its heading
  line="- [ ] [#$number]($url) - $title by [@$user](https://github.com/$user)"
  awk -v heading="$heading" -v line="$line" '{ print } $0 == heading { getline; print; print line }' "$todo" > "$RUNNER_TEMP/todo.tmp"
  mv "$RUNNER_TEMP/todo.tmp" "$todo"

  # no @ here, a mention in the PR body would notify the author on every update
  printf -- '- %s: #%s - %s by %s\n' "${heading#\#\# }" "$number" "$title" "$user" >> "$body"
  per_author[$author]=$(( ${per_author[$author]:-0} + 1 ))
  kept=$((kept + 1))
done < <(jq -c '.[]' "$RUNNER_TEMP/issues.json")

pr="$(gh pr list --head "$branch" --state open --json number --jq '.[0].number // empty')"

if [ "$kept" -eq 0 ]; then
  if [ -n "$pr" ]; then gh pr close "$pr" --comment "Nothing is pending anymore."; fi
  if git ls-remote --exit-code --heads origin "$branch" > /dev/null; then git push --quiet origin --delete "$branch"; fi
  echo "inbox is empty"
  exit 0
fi

# only push when the inbox changed, so the PR's checks don't rerun for nothing
if git ls-remote --exit-code --heads origin "$branch" > /dev/null; then
  git fetch --quiet origin "$branch"
  if git diff --quiet "origin/$branch" -- "$todo" && [ "$(git merge-base origin/main "origin/$branch")" = "$(git rev-parse origin/main)" ]; then
    echo "inbox unchanged ($kept items)"
    if [ -n "$pr" ]; then gh pr edit "$pr" --body-file "$body"; fi
    exit 0
  fi
fi

git -c user.name="github-actions[bot]" -c user.email="41898282+github-actions[bot]@users.noreply.github.com" \
  commit --quiet -am "docs: add $kept pending items to TODO"
git push --quiet --force origin "$branch"
if [ -n "$pr" ]; then
  gh pr edit "$pr" --body-file "$body"
else
  gh pr create --head "$branch" --base main --title "docs: TODO inbox" --body-file "$body" --assignee "$OWNER"
fi
echo "inbox has $kept items"
