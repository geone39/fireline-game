#!/usr/bin/env bash
# Переносит ОТКРЫТЫЕ issues в новый репозиторий (комментарии и авторство не сохраняются).
# Использование: migrate-issues.sh geone39/fireline geone39/<новое-имя>
set -euo pipefail
OLD=${1:?}; NEW=${2:?}
gh issue list -R "$OLD" --state open --limit 500 --json number,title,body,labels |
python3 -c '
import json,subprocess,sys
for i in json.load(sys.stdin):
    body=(i["body"] or "")+"\n\n_Перенесено из %s#%d_"%(sys.argv[1],i["number"])
    cmd=["gh","issue","create","-R",sys.argv[2],"--title",i["title"],"--body",body]
    for l in i["labels"]: cmd+=["--label",l["name"]]
    subprocess.run(cmd,check=False)
' "$OLD" "$NEW"
# Метки предварительно: gh label list -R OLD ; gh label create -R NEW ...
