#!/usr/bin/env bash
# СЦЕНАРИЙ 2: новый публичный репозиторий из переписанной истории.
# ЗАПУСКАТЬ ТОЛЬКО ПО КОМАНДЕ ВЛАДЕЛЬЦА. Использование: push-new-repo.sh <scratch-dir> <имя-репозитория>
set -euo pipefail
W=${1:?usage}; NAME=${2:?имя репозитория}
cd "$W/rewrite.git"
gh repo create "geone39/$NAME" --public --description "Fireline (Плацдарм) — early development"
git push "https://github.com/geone39/$NAME.git" refs/heads/main:refs/heads/main   # только main; ветки PR не переносим
