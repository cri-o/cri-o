#!/bin/bash

# Script to deploy pods incrementally until 200 are reached
# Initial deployment of 5 pods, then adding 5 more every 30 seconds

# Set variables
TOTAL_PODS_TARGET=100
PODS_PER_ITERATION=5
WAIT_TIME=15  # seconds
DELETE_WAIT_TIME=1
DECREASE_WAIT_TIME=5

# Initialize counter
current_pods=0

echo "Starting pod deployment process..."
echo "Target: $TOTAL_PODS_TARGET pods"
echo "Will deploy $PODS_PER_ITERATION pods every $WAIT_TIME seconds"
echo "$(date -u +"%Y-%m-%dT%H:%M:%SZ") - Deploying 5 pods..."

cat << EOF | kubectl apply -f -
apiVersion: apps/v1
kind: Deployment
metadata:
  name: test
  labels:
    app: test
spec:
  replicas: 5
  selector:
    matchLabels:
      app: test
  template:
    metadata:
      labels:
        app: test
    spec:
      containers:
      - name: test-container0
        image: quay.io/libpod/busybox
        command: ["/bin/sh", "-c", "while true; do sleep 1; done"]
      - name: test-container1
        image: quay.io/libpod/busybox
        command: ["/bin/sh", "-c", "while true; do sleep 1; done"]
      - name: test-container2
        image: quay.io/libpod/busybox
        command: ["/bin/sh", "-c", "while true; do sleep 1; done"]
      - name: test-container3
        image: quay.io/libpod/busybox
        command: ["/bin/sh", "-c", "while true; do sleep 1; done"]
      - name: test-container4
        image: quay.io/libpod/busybox
        command: ["/bin/sh", "-c", "while true; do sleep 1; done"]
EOF

sleep $WAIT_TIME

# Continue until we reach or exceed the target
while [ $current_pods -lt $TOTAL_PODS_TARGET ]; do
    # Calculate how many pods to deploy in this iteration
    # Make sure we don't exceed the target
    pods_to_deploy=$PODS_PER_ITERATION
    if [ $((current_pods + pods_to_deploy)) -gt $TOTAL_PODS_TARGET ]; then
        pods_to_deploy=$((TOTAL_PODS_TARGET - current_pods))
    fi

    echo "$(date -u +"%Y-%m-%dT%H:%M:%SZ") - Deploying $pods_to_deploy pods to $current_pods..."

    new_pod_count=$((current_pods + pods_to_deploy))

    # Command to deploy pods (replace with your actual deployment command)
    # Example using kubectl scale:
    kubectl scale deployment/test --replicas=$new_pod_count > /dev/null
    # For demonstration, we'll just echo the command


    # Update the counter
    current_pods=$new_pod_count

    # Exit the loop if we've reached the target
    if [ $current_pods -ge $TOTAL_PODS_TARGET ]; then
        break
    fi

    sleep $WAIT_TIME
done

while [ $(kubectl get pods -l "app=test" | grep Running | wc -l ) -lt "$TOTAL_PODS_TARGET" ]; do
  echo "$(date +"%Y-%m-%d %H:%M:%S") waiting to be stable (running: $(kubectl get pods -l "app=test" | grep Running | wc -l))"
  sleep 5
done
