#!/bin/bash

set -eu

pwd=$PWD
PROJECT_DIR="$(cd "$(dirname "$0")/.."; pwd)"

app_name=""
manifest_path=""
bootstrap_path=""
resolvers=""

function print_usage {
    echo "Usage: $0 [-a:m:b:p:h]"
    echo " -a application name (REQUIRED) - The given name (and route) for CF-FaaS."
    echo " -m manifest path (REQUIRED)    - The path to the YAML file that configures the endpoints."
    echo " -b bootstrap manifest path     - The path to the YAML file that configures the bootstrap endpoints."
    echo " -r resolver URLS               - Comma separated list of key values (e.g., key1:value1,key2:value2). "
    echo "                                  Each key-value pair is an event name and URL"
    echo "                                  (e.g., eventname:some.url/v1/path,other-event:/v2/bootstrap/path)."
    echo " -h help                        - Shows this usage."
    echo
    echo "More information available at https://github.com/apoydence/cf-faas"
}

function abs_path {
    case $1 in
        /*) echo $1 ;;
        *) echo $pwd/$1 ;;
    esac
}

function fail {
    echo $1
    exit 1
}

while getopts 'a:m:b:r:h' flag; do
  case "${flag}" in
    a) app_name="${OPTARG}" ;;
    m) manifest_path="$(abs_path "${OPTARG}")" ;;
    b) bootstrap_path="$(abs_path "${OPTARG}")" ;;
    r) resolvers="${OPTARG}" ;;
    h) print_usage ; exit 1 ;;
  esac
done

# Ensure we are starting from the project directory
cd $PROJECT_DIR

if [ -z "$app_name" ]; then
    echo "AppName is required via -a flag"
    print_usage
    exit 1
fi

if [ -z "$manifest_path" ]; then
    echo "Manifest is required via -m flag"
    print_usage
    exit 1
fi

TEMP_DIR=$(mktemp -d)

# CF-FaaS binaries
echo "building CF-FaaS binaries..."
GOOS=linux go build -o $TEMP_DIR/cf-faas ./cmd/cf-faas &> /dev/null || fail "failed to build cf-faas"
GOOS=linux go build -o $TEMP_DIR/task-runner ./cmd/task-runner &> /dev/null || fail "failed to build cf-faas' task-runner"
GOOS=linux go build -o $TEMP_DIR/worker ./cmd/worker &> /dev/null || fail "failed to build cf-faas' worker"
GOOS=linux go build -o $TEMP_DIR/manifest-parser ./cmd/manifest-parser &> /dev/null || fail "failed to build cf-faas' manifest-parser"
cp cmd/cf-faas/run.sh $TEMP_DIR
echo "done building CF-FaaS binaries."

# CF-Space-Security binaries
echo "building CF-Space-Security binaries..."
go get github.com/apoydence/cf-space-security/... &> /dev/null || fail "failed to get cf-space-security"
GOOS=linux go build -o $TEMP_DIR/proxy ../cf-space-security/cmd/proxy &> /dev/null || fail "failed to build cf-space-security proxy"
GOOS=linux go build -o $TEMP_DIR/reverse-proxy ../cf-space-security/cmd/reverse-proxy &> /dev/null || fail "failed to build cf-space-security reverse proxy"
echo "done building CF-Space-Security binaries."

echo "pushing $app_name..."
cf push $app_name --no-start -p $TEMP_DIR -b binary_buildpack -c ./run.sh &> /dev/null || fail "failed to push app $app_name"
echo "done pushing $app_name."

if [ -z ${CF_HOME+x} ]; then
    CF_HOME=$HOME
fi

# Configure
echo "configuring $app_name..."
cf set-env $app_name REFRESH_TOKEN "$(cat $CF_HOME/.cf/config.json | jq -r .RefreshToken)" &> /dev/null || fail "failed to set REFRESH_TOKEN"
cf set-env $app_name CLIENT_ID "$(cat $CF_HOME/.cf/config.json | jq -r .UAAOAuthClient)" &> /dev/null || fail "failed to set set CLIENT_ID"
cf set-env $app_name MANIFEST "$(cat $manifest_path)" &> /dev/null || fail "failed to set MANIFEST"
cf set-env $app_name RESOLVER_URLS "$resolvers" &> /dev/null || fail "failed to set RESOLVER_URLS"

skip_ssl_validation="$(cat $CF_HOME/.cf/config.json | jq -r .SSLDisabled)"
if [ $skip_ssl_validation = "true" ]; then
    cf set-env $app_name SKIP_SSL_VALIDATION true &> /dev/null || fail "failed to set SKIP_SSL_VALIDATION"
fi

if [ ! -z "$bootstrap_path" ]; then
    cf set-env $app_name BOOTSTRAP_MANIFEST "$(cat $bootstrap_path)" &> /dev/null || fail "failed to set BOOTSTRAP_MANIFEST"
fi
echo "done configuring $app_name."

echo "starting $app_name..."
cf start $app_name &> /dev/null || fail "failed to start $app_name"
echo "done starting $app_name."
