#!/usr/bin/env bash
set -euo pipefail

# envtest binaries (etcd, kube-apiserver, kubectl) matching the k8s.io/* version in go.mod
assets=$(go run sigs.k8s.io/controller-runtime/tools/setup-envtest@release-0.24 use 1.36.x --bin-dir kubebuilder -p path)
mkdir -p kubebuilder/bin
cp -f "$assets"/* kubebuilder/bin/
