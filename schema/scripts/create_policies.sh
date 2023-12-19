#!/usr/bin/env bash

set -euo pipefail
IFS=$'\n\t'

DIR=$1
OPENSEARCH_HOST=$2

PATTERN="$DIR/*.json"
echo -e "Reading policy files from $DIR"

for FILE in $PATTERN; do
  policy_definition=$(<$FILE)
  policy=$(basename "$FILE" .json)
  url="$OPENSEARCH_HOST/_plugins/_ism/policies/$policy"
  echo -e "\nCreating index policy: $url\n$policy_definition\n"

  result=$(curl -XPUT "$url" -s -H 'Content-Type: application/json' -d "$policy_definition")
  echo -e "> $result"

  if [[ $result =~ "resource_already_exists_exception" ]]; then
    echo "==> Index template already exists"
  elif [[ $result =~ "acknowledged" ]]; then
    echo "==> Index template created"
  else
    echo -e "==> Unknown result: $result"
    exit 1
  fi
  echo -e "\n\n"

done