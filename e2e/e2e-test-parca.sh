################################################################################
#
# This script is meant to be run from the root of this project
#
################################################################################

#!/bin/bash

set -euo pipefail

function e2erun() {
  minikube_cluster_up
  deploy
  # minikube_cluster_down genius, spin deploy and delete ugh
}

# Create local minikube cluster and deploys the dev env for parca and parca agent 
function minikube_cluster_up() {
  echo "Spinning up a parca dev cluster"
  minikube start -p parca-dev \
    --driver=kvm2 \
    --kubernetes-version=v1.22.3 \
    --cpus=4 \
    --memory=16gb \
    --disk-size=20gb \
    --docker-opt dns=8.8.8.8 \
    --docker-opt default-ulimit=memlock=9223372036854775807:9223372036854775807
}

# Delete minikube instance
function minikube_cluster_down() {
  echo "deleting parca dev cluster"
  minikube delete -p parca-dev
}

# Fetches latest binaries for parca and parca agent 
function deploy() {
  SERVER_LATEST_VERSION=$(curl -s https://api.github.com/repos/parca-dev/parca/releases/latest | grep -oP '"tag_name": "\K(.*)(?=")' | xargs echo -n)
  AGENT_LATEST_VERSION=$(curl -s https://api.github.com/repos/parca-dev/parca-agent/releases/latest | grep -oP '"tag_name": "\K(.*)(?=")' | xargs echo -n)
  
  echo "Server version: $SERVER_LATEST_VERSION"
  echo "Agent version: $AGENT_LATEST_VERSION"

  build
  
  kubectl apply -f https://github.com/parca-dev/parca/releases/download/"$SERVER_LATEST_VERSION"/kubernetes-manifest.yaml
  kubectl apply -f ./manifests/local/manifest-e2e.yaml
  
  sleep 1m

  echo "Connecting to Parca"

  kubectl port-forward -n parca service/parca 7070
}

function vendor() {
  jb install
}

function build() {
#  ifeq ($(GITHUB_BRANCH_NAME),)
#    BRANCH=$(shell git rev-parse --abbrev-ref HEAD)-
#  else
#    BRANCH=$(GITHUB_BRANCH_NAME)-
#  endif
#  ifeq ($(GITHUB_SHA),)
#    COMMIT=$(shell git describe --no-match --dirty --always --abbrev=8)
#  else
#    COMMIT=$(shell echo $(GITHUB_SHA) | cut -c1-8)
#  endif
#  VERSION=$(if $(RELEASE_TAG),$(RELEASE_TAG),$(shell $(CMD_GIT) describe --tags 2>/dev/null || echo '$(BRANCH)$(COMMIT)'))
#  
 
 # VERSION=`if $(RELEASE_TAG),$(RELEASE_TAG),$(shell $(CMD_GIT) describe --tags 2>/dev/null || echo "sumo-79866")`
  



 # jb install $(find . -name 'vendor' -prune -o -name '*.libsonnet' -print -o -name '*.jsonnet' -print)
	
  echo blah
  rm -rf manifests/local 
  mkdir -p manifests/local
  
  echo blahhh
  jsonnet --tla-str version="e2e-f655897a0ab80c18" -J vendor main.jsonnet -m manifests/local | xargs -I{} sh -c 'cat {} | gojsontoyaml > {}.yaml; rm -f {}' -- {}
  awk 'BEGINFILE {print "---"}{print}' manifests/local/* > manifests/local/manifest-e2e.yaml	
}
