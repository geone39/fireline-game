#!/usr/bin/env bash
# Переписывает автора/коммиттера во ВСЕХ ветках и тегах и проверяет результат. Ничего не пушит.
# Использование: OLD_EMAILS="a@example.com b@example.com" rewrite-history.sh <рабочая-папка-вне-репозитория>
# Старые адреса передаются ТОЛЬКО через переменную окружения — в репозиторий их литералами не вносить.
# Нужны: git, git-filter-repo (pip install git-filter-repo).
set -euo pipefail
W=${1:?usage: OLD_EMAILS="a@x b@y" rewrite-history.sh <scratch-dir>}
: "${OLD_EMAILS:?задайте OLD_EMAILS (адреса через пробел)}"
NEW='geone39 <50493709+geone39@users.noreply.github.com>'
mkdir -p "$W"; cd "$W"
rm -rf fireline-backup.git rewrite.git
git clone --mirror https://github.com/geone39/fireline.git fireline-backup.git   # неизменная копия
git clone --mirror fireline-backup.git rewrite.git
for e in $OLD_EMAILS; do echo "$NEW <$e>"; done > mailmap.txt
cd rewrite.git
git for-each-ref --format='%(refname)' refs/pull | xargs -r -n1 git update-ref -d   # refs/pull не переписываем
git filter-repo --mailmap ../mailmap.txt --force
echo "== авторы/коммиттеры после:"; git log --all --format='%ae %ce' | sort -u
if git log --all --format='%ae %ce' | grep -F -f <(printf '%s
' $OLD_EMAILS); then echo "FAIL: почта осталась"; exit 1; fi
for b in $(git for-each-ref --format='%(refname)' refs/heads refs/tags); do
  [ "$(git -C ../fireline-backup.git rev-parse "$b^{tree}")" = "$(git rev-parse "$b^{tree}")" ] || { echo "FAIL: дерево $b"; exit 1; }
done
[ "$(git -C ../fireline-backup.git rev-list --count --branches --tags)" = "$(git rev-list --count --all)" ] || { echo "FAIL: число коммитов"; exit 1; }
# карта старых -> новых хэшей нужна для переноса веток (filter-repo кладёт её сюда):
cp filter-repo/commit-map ../commit-map.txt
echo "OK: $W/rewrite.git готов (карта хэшей: $W/commit-map.txt)"
