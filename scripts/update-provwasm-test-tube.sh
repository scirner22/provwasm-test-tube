#!/usr/bin/env bash

set -euxo pipefail

SCRIPT_DIR=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )
PROVENANCE_REV=${1:-main}

LATEST_PROVENANCE_VERSION="main"

# if "$PROVENANCE_REV" is /v\d+/ then extract it as var
if [[ "$PROVENANCE_REV" =~ ^v[0-9]+ ]]; then
  PROVENANCE_VERSION=$(echo "$PROVENANCE_REV" | sed "s/\..*$//")
else
  PROVENANCE_VERSION="$LATEST_PROVENANCE_VERSION"
fi

########################################
## Update and rebuild provwasm-test-tube ##
########################################

# update all submodules
#git submodule update --init --recursive
#cd "$SCRIPT_DIR/../packages/provwasm-test-tube/provenance"
#
PROVENANCE_REV_NO_ORIGIN="$(echo "$PROVENANCE_REV" | sed "s/^origin\///")"
#
#git checkout "$PROVENANCE_REV_NO_ORIGIN"


# build and run update-provwasm-test-tube
cd "$SCRIPT_DIR/update-provwasm-test-tube-deps" && go build

# run update-provwasm-test-tube-deps which will replace the `replace directives` in provwasm-test-tube
# with provenance's replaces
"$SCRIPT_DIR/update-provwasm-test-tube-deps/update-provwasm-test-tube-deps" "$PROVENANCE_REV_NO_ORIGIN"

#cd "$SCRIPT_DIR/../packages/provenance-test-tube/provenance"
#PARSED_REV=$(git rev-parse --short "$PROVENANCE_REV")

cd "$SCRIPT_DIR/../packages/provwasm-test-tube/libprovwasmtesttube"

#go get "github.com/provenance-io/provenance/${PROVENANCE_VERSION}@${PARSED_REV}"

# tidy up updated go.mod
go mod tidy


########################################
## Update git revision if there is    ##
## any change                         ##
########################################

if [[ -n "${SKIP_GIT_UPDATE:-}" ]]; then
  echo '[SKIP] SKIP_GIT_UPDATE is set, skipping git update'
  exit 0
fi

# if dirty or untracked file exists
#if [[ $(git diff --stat) != '' ||  $(git ls-files  --exclude-standard  --others) ]]; then
#  # add, commit and push
#  git add "$SCRIPT_DIR/.."
#  git commit -m "rebuild with $(git rev-parse --short HEAD:dependencies/provenance)"
#
#  # remove "origin/"
#  PROVENANCE_REV=$(echo "$PROVENANCE_REV" | sed "s/^origin\///")
#  BRANCH="autobuild-$PROVENANCE_REV"
#
#  # force delete local "$BRANCH" if exists
#  git branch -D "$BRANCH" || true
#
#  git checkout -b "$BRANCH"
#  git push -uf origin "$BRANCH"
#else
#  echo '[CLEAN] No update needed for this build'
#fi
