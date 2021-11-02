#!/usr/bin/env bash
# shellcheck disable=SC2069

# why this script you may ask?
# running `go run main.go server | jq` from the make target
# and then sending ctrl-c causes a broken pipe error and final log
# messages sent to stdout don't get displayed. This happens because
# ctrl-c kills both the `go run` and `jq` commands. And with the `jq` command
# dead there's is nothing on the other end of the pipe to read the stdout from `go run`

# trap ctrl-c and call ctrl_c()
trap ctrl_c INT

function ctrl_c() {
    kill "${RUN_PID}"
    sleep 3
    kill "$(<pid)"
    rm -rf temp.log pid
    exit 0
}

go run main.go -config tinklet.example.yaml docker 2>&1 > temp.log &
RUN_PID=$!
( tail -f temp.log & echo $! >&3 ) 3>pid | jq -R 'fromjson? | select(type == "object")' &
tail -F /dev/null