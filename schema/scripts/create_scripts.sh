#!/usr/bin/env bash

set -euo pipefail
IFS=$'\n\t'

DIR=$1
OPENSEARCH_HOST=$2

PATTERN="$DIR/*.json"
echo -e "Reading stored script files from $DIR"

for FILE in $PATTERN; do
  mapping=$(<$FILE)
  index=$(basename "$FILE" .json)
  url="$OPENSEARCH_HOST/_scripts/$index"
  echo -e "\nCreating stored script: $url\n$mapping\n"

  result=$(curl -XPUT "$url" -s -H 'Content-Type: application/json' -d "$mapping")
  echo -e "> $result"
done
