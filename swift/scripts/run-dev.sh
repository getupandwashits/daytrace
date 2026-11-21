#!/bin/bash
set -eu
cd "$(dirname "$0")/.."
./scripts/build.sh
mkdir -p .dev/home .dev/output
export DAYTRACE_HOME="$PWD/.dev/home"
export DAYTRACE_OUTPUT="$PWD/.dev/output"
export DAYTRACE_LEGACY_STATUS_DIR="$PWD/.dev/legacy/AutoPomodoro"
exec .build/DayTrace.app/Contents/MacOS/DayTrace "$@"
