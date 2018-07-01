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

# CF-FaaS binaries
echo "building CF-FaaS binaries..."
GOOS=linux go build -o $TEMP_DIR/cf-faas ../cmd/cf-faas &> /dev/null || echo "failed to build cf-faas"
GOOS=linux go build -o $TEMP_DIR/task-runner ../cmd/task-runner &> /dev/null || echo "failed to build cf-faas' task-runner"
GOOS=linux go build -o $TEMP_DIR/worker ../cmd/worker &> /dev/null || echo "failed to build cf-faas' worker"
cp ../cmd/cf-faas/run.sh $TEMP_DIR
echo "done building CF-FaaS binaries."

# CF-Space-Security binaries
echo "building CF-Space-Security binaries..."
go get github.com/apoydence/cf-space-security/... &> /dev/null || echo "failed to get cf-space-security"
GOOS=linux go build -o $TEMP_DIR/proxy ../../cf-space-security/cmd/proxy &> /dev/null || echo "failed to build cf-space-security proxy"
GOOS=linux go build -o $TEMP_DIR/reverse-proxy ../../cf-space-security/cmd/reverse-proxy &> /dev/null || echo "failed to build cf-space-security reverse proxy"
echo "done building CF-Space-Security binaries."

echo "pushing $app_name..."
cf push $app_name --no-start -p $TEMP_DIR -b binary_buildpack -c ./run.sh &> /dev/null || echo "failed to push app $app_name"
echo "done pushing $app_name."

if [ -z ${CF_HOME+x} ]; then
    CF_HOME=$HOME
fi

# Configure
echo "configuring $app_name..."
cf set-env $app_name REFRESH_TOKEN "$(cat $CF_HOME/.cf/config.json | jq -r .RefreshToken)" &> /dev/null || echo "failed to set REFRESH_TOKEN"
cf set-env $app_name CLIENT_ID "$(cat $CF_HOME/.cf/config.json | jq -r .UAAOAuthClient)" &> /dev/null || echo "failed to set set CLIENT_ID"
cf set-env $app_name MANIFEST "$(cat $manifest_path)" &> /dev/null || echo "failed to set MANIFEST"
cf set-env $app_name PLUGIN_URLS "$plugins" &> /dev/null || echo "failed to set PLUGIN_URLS"

skip_ssl_validation="$(cat $CF_HOME/.cf/config.json | jq -r .SSLDisabled)"
if [ $skip_ssl_validation = "true" ]; then
    cf set-env $app_name SKIP_SSL_VALIDATION true &> /dev/null || echo "failed to set SKIP_SSL_VALIDATION"
fi

if [ ! -z "$bootstrap_path" ]; then
    cf set-env $app_name BOOTSTRAP_MANIFEST "$(cat $bootstrap_path)" &> /dev/null || echo "failed to set BOOTSTRAP_MANIFEST"
fi
echo "done configuring $app_name."

echo "starting $app_name..."
cf start $app_name &> /dev/null || echo "failed to start $app_name"
echo "done starting $app_name."
