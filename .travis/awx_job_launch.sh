#!/bin/bash

set -euo pipefail

readonly awx_username=${AWX_USERNAME:-}
readonly awx_password=${AWX_PASSWORD:-}
readonly awx_develop_v3_job_launch_uri=${AWX_V3_DEVELOP_JOB_LAUNCH_URI:-}
readonly awx_staging_v3_job_launch_uri=${AWX_V3_STAGING_JOB_LAUNCH_URI:-}

if [[ "$TRAVIS_BRANCH" == "v3-develop" ]]; then
    awx_job_launch_uri=$awx_v3_develop_job_launch_uri
elif [[ "$TRAVIS_BRANCH" == "v3-staging" ]]; then
    awx_job_launch_uri=$awx_v3_staging_job_launch_uri
else
    echo "branch is not set to call awx deploy"
    exit 0
fi

curl -X POST -o /dev/null -w "HTTP response code: %{http_code}\n" $awx_job_launch_uri --user "$awx_username:$awx_password"
