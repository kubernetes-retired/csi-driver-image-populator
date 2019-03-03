#!/usr/bin/env bash

# This script captures the steps required to successfully
# deploy the image plugin driver.  This should be considered
# authoritative and all updates for this process should be
# done here and referenced elsewhere.

# The script assumes that kubectl is available on the OS path 
# where it is executed.

set -e
set -o pipefail

BASE_DIR=$(dirname "$0")

# deploy image plugin and registrar sidecar
echo "deploying image components"
kubectl apply -f ${BASE_DIR}/image
