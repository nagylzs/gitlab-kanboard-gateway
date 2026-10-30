#!/bin/bash
set -e
DIR="$(dirname $(realpath $0))"
DIST=${DIR}/../dist

if [ "${1:-}" == "all" ]; then
    # os/arch pairs; Go has no darwin/386
    targets=(linux/amd64 linux/386 windows/amd64 windows/386 darwin/amd64 darwin/arm64)
else
    eval $(go tool dist env)
    targets=($GOOS/$GOARCH)
fi

cmds=(gitlab-kanboard-gateway kanboard-task kanboard-mcp)

set -x

# See https://docs.gitlab.com/ee/ci/variables/predefined_variables.html
# CI_COMMIT_TIMESTAMP ???
BUILT=$(date +"%Y-%m-%dT%H:%M:%S")
BRANCH="$CI_COMMIT_BRANCH"
if [ -z "$BRANCH" ]; then
    BRANCH=$(git rev-parse --abbrev-ref HEAD)
fi
COMMIT="$CI_COMMIT_SHA"
if [ -z "$COMMIT" ]; then
  COMMIT=$(git rev-parse HEAD)
fi


mkdir -p ${DIST}
echo "$BUILT $BRANCH $COMMIT" > ${DIST}/version.txt

for target in ${targets[@]}
do
    os=${target%/*}
    arch=${target#*/}
    output_dir=${DIST}/${os}/${arch}
    rm -rf ${output_dir}   # drop binaries from previous builds
    mkdir -p ${output_dir}
    for cmd in ${cmds[@]}
    do
        cd ${DIR}/../cmd/${cmd}
        env GOOS=${os} GOARCH=${arch} \
            go build \
                -ldflags "
                    -X github.com/nagylzs/gitlab-kanboard-gateway/internal/version.Built=${BUILT}
                    -X github.com/nagylzs/gitlab-kanboard-gateway/internal/version.Commit=${COMMIT}
                    -X github.com/nagylzs/gitlab-kanboard-gateway/internal/version.Branch=${BRANCH}" \
                -o ${output_dir}/ ${cmd}.go
    done
done
