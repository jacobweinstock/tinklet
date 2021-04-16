#!/usr/bin/env bash
# this scripts must be run from the root of the tinklet repo
set -e

file_totals=$(go test -coverprofile=cover.out ./...)
out=$(go tool cover -func=cover.out)
percentage=$(echo ${out} | grep total | awk '{print $NF}')
date_time=$(date -u)
fullOut=$(echo "${date_time}
====================================================================================================
${file_totals}
====================================================================================================
${out}
====================================================================================================
")
rm -rf cover.out

# percentage will equal something like 92.4%
# this sets baseNum to be everything before the decimal
# e.g. 92.4% becomes 92
# needed for bash number comparisons
baseNum=${percentage%.*}
if [ "${baseNum}" -ge "80" ]; then
  coverageColor=green
elif [ "${baseNum}" -le "79" ] && [ "${baseNum}" -ge "70" ]; then
  coverageColor=yellow
else
  coverageColor=red
fi

coverageJSON='{"schemaVersion":1, "label":"Code Coverage", "message":"'${percentage}'", "color":"'${coverageColor}'"}'
echo ${coverageJSON}

# this updates the following GIST: https://gist.github.com/jacobweinstock/9d00cc54b39121e62d88ab6e02cec6dd#file-branch-main-json
(cd scripts/code_coverage && go run main.go -accesstoken ${GIST_TOKEN} -filename branch-main.json -id 9d00cc54b39121e62d88ab6e02cec6dd -content "${coverageJSON}")

# this updates the following GIST: https://gist.github.com/jacobweinstock/9d00cc54b39121e62d88ab6e02cec6dd#file-branch-main-coverage
(cd scripts/code_coverage && go run main.go -accesstoken ${GIST_TOKEN} -filename branch-main.coverage -id 9d00cc54b39121e62d88ab6e02cec6dd -content "${fullOut}")