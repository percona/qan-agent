#!/usr/bin/env bash

# Retries a command a configurable number of times with backoff.
#
# The retry count is given by ATTEMPTS (default 120), the backoff
# timeout is given by TIMEOUT in seconds (default 1.)
#
# Based on: https://stackoverflow.com/a/8351489/916440
function with_backoff {
  local max_attempts=${ATTEMPTS-120}
  local timeout=${TIMEOUT-1}
  local attempt=1
  local exitCode=0

  while (( $attempt < $max_attempts ))
  do
    if "$@"
    then
      return 0
    else
      exitCode=$?
    fi

    echo "Failure! Retrying in $timeout.." 1>&2
    sleep $timeout
    attempt=$(( attempt + 1 ))
  done

  if [[ $exitCode != 0 ]]
  then
    echo "Giving up! ($@)" 1>&2
  fi

  return $exitCode
}

with_backoff "$@"