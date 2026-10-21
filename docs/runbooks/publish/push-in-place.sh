#!/usr/bin/env bash
# СЦЕНАРИЙ 1: force-push переписанной истории в существующий geone39/fireline.
# ЗАПУСКАТЬ ТОЛЬКО ПО КОМАНДЕ ВЛАДЕЛЬЦА. Использование: push-in-place.sh <scratch-dir>
set -euo pipefail
cd "${1:?usage}/rewrite.git"
git remote remove origin 2>/dev/null || true
git remote add origin https://github.com/geone39/fireline.git
git fetch origin 'refs/heads/*:refs/remotes/origin/*'
for b in $(git for-each-ref --format='%(refname:strip=2)' refs/heads); do
  old=$(git -C ../fireline-backup.git rev-parse "refs/heads/$b")   # ожидаемое старое значение
  git push --force-with-lease="refs/heads/$b:$old" origin "refs/heads/$b:refs/heads/$b"
done
git push --force --tags origin 2>/dev/null || true
