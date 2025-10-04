#!/bin/bash

# Uninstall the Service Router Operator and its dependencies

set -e

# Define the namespace where the operator is installed
NAMESPACE="default"

# Delete the Custom Resource Definitions (CRDs)
kubectl delete crd servicerouters.example.com || true

# Delete the operator deployment
kubectl delete deployment service-router-operator -n $NAMESPACE || true

# Delete the service account, role, and role binding
kubectl delete serviceaccount service-router-operator -n $NAMESPACE || true
kubectl delete role service-router-operator -n $NAMESPACE || true
kubectl delete rolebinding service-router-operator -n $NAMESPACE || true

# Optionally, delete the namespace if it was created for the operator
# kubectl delete namespace $NAMESPACE || true

echo "Service Router Operator uninstalled successfully."