#!/bin/bash

i=$(gh api /repos/Velocidex/cloudvelo/actions/runs?per_page=10  | jq 'limit(1; .workflow_runs[] | select (.name | contains("Tests")) | .id )')

rm -f artifact/*

gh run download $i
cd artifact/ && cp TestClientEventMonitoring.golden  TestEnrollment.golden  TestErrorLogs.golden  TestListDirectory.golden  TestVFSDownload.golden ../ingestion/fixtures/ && cd -
