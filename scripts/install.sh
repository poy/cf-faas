#!/bin/bash

set -eu

app_name=""
manifest_path=""
bootstrap_path=""
plugins=""

while getopts 'a:m:b:p:' flag; do
  case "${flag}" in
    a) app_name="${OPTARG}" ;;
    m) manifest_path="${OPTARG}" ;;
    b) bootstrap_path="${OPTARG}" ;;
    p) plugins="${OPTARG}" ;;
  esac
done

if [ -z "$app_name" ]; then
    echo "AppName is required via -a flag"
    exit 1
fi

if [ -z "$manifest_path" ]; then
    echo "Manifest is required via -m flag"
    exit 1
fi

TEMP_DIR=$(mktemp -d)

GOOS=linux go build -o $TEMP_DIR/cf-faas ../cmd/cf-faas
GOOS=linux go build -o $TEMP_DIR/task-runner ../cmd/task-runner
GOOS=linux go build -o $TEMP_DIR/worker ../cmd/worker
cp ../cmd/cf-faas/run.sh $TEMP_DIR

# CF-Space-Security
go get github.com/apoydence/cf-space-security/...
GOOS=linux go build -o $TEMP_DIR/proxy ../../cf-space-security/cmd/proxy
GOOS=linux go build -o $TEMP_DIR/reverse-proxy ../../cf-space-security/cmd/reverse-proxy

cf push $app_name --no-start -p $TEMP_DIR -b binary_buildpack -c ./run.sh

if [ -z ${CF_HOME+x} ]; then
    CF_HOME=$HOME
fi

skip_ssl_validation="$(cat $CF_HOME/.cf/config.json | jq -r .SSLDisabled)"

cf set-env $app_name REFRESH_TOKEN "$(cat $CF_HOME/.cf/config.json | jq -r .RefreshToken)"
cf set-env $app_name CLIENT_ID "$(cat $CF_HOME/.cf/config.json | jq -r .UAAOAuthClient)"
cf set-env $app_name MANIFEST "$(cat $manifest_path)"
cf set-env $app_name PLUGIN_URLS "$plugins"

if [ $skip_ssl_validation = "true" ]; then
    cf set-env $app_name SKIP_SSL_VALIDATION true
fi

if [ ! -z "$bootstrap_path" ]; then
    cf set-env $app_name BOOTSTRAP_MANIFEST "$(cat $bootstrap_path)"
fi

cf start $app_name
