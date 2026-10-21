#!/usr/bin/env bash
# Переносит ветку/worktree другой сессии на переписанную историю.
# Использование (внутри клона/worktree, ПОСЛЕ force-push main): rebase-branch.sh <commit-map.txt> <ветка>
# commit-map.txt — из rewrite-history.sh (формат: old new).
set -euo pipefail
MAP=${1:?}; BR=${2:?}
git fetch origin
# Надёжный путь: найти последний старый коммит ветки, присутствующий в карте, и взять его новый хэш.
BASE_OLD=$(git rev-list "$BR" | while read -r c; do grep -q "^$c " "$MAP" && { echo "$c"; break; }; done)
BASE_NEW=$(awk -v c="$BASE_OLD" '$1==c{print $2}' "$MAP")
echo "старая база $BASE_OLD -> новая $BASE_NEW"
git rebase --onto "$BASE_NEW" "$BASE_OLD" "$BR"
git push --force-with-lease origin "$BR"
