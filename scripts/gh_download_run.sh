#!/bin/bash

i=$(gh api /repos/Velocidex/cloudvelo/actions/runs?per_page=10  | jq 'limit(1; .workflow_runs[] | select (.name | contains("Tests")) | .id )')

gh run download $i
cp artifact/TestIngestor.golden ./ingestion/fixtures/
