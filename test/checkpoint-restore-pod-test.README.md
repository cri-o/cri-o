# Pod Checkpoint/Restore Test Script

## Overview

This script tests CRI-O's pod-level checkpoint and restore
functionality. It creates a pod with multiple containers,
checkpoints it to an OCI image, removes the pod, and then
restores it from the checkpoint image.

## Prerequisites

1. **Root access**: The script must be run as root
2. **CRIU**: Must be installed with pod checkpoint support
3. **CRI-O**: Built with checkpoint/restore support (`make bin/crio`)
4. **crictl**: Built from cri-tools with pod checkpoint support
   (`cd ../cri-tools && make crictl`)
   - **IMPORTANT**: Must support `checkpointp` and `restorep` commands
   - The script will verify these commands are available and exit if not
5. **buildah**: For managing checkpoint OCI images
6. **pinns**: Built (`make bin/pinns`)

## Usage

### Basic Usage

```bash
sudo ./test/checkpoint-restore-pod-test.sh
```

The test uses two container images:

- Container 1: `quay.io/adrianreber/wildfly-hello`
- Container 2: `quay.io/adrianreber/counter`

## What the Script Does

1. **Prerequisites Check**
   - Verifies root access
   - Builds `checkcriu` tool if needed
   - Checks for CRIU availability
   - Validates all required binaries exist

2. **CRI-O Setup**
   - Starts CRI-O with debug logging
   - Waits for CRI-O socket to be ready

3. **Pod Creation**
   - Creates a test pod with unique UID
   - Creates two containers in the pod using different images:
     - Container 1: `quay.io/adrianreber/wildfly-hello`
     - Container 2: `quay.io/adrianreber/counter`

4. **Checkpoint**
   - Waits for containers to be running
   - Checkpoints entire pod to OCI image: `localhost/checkpoint-pod:test-<PID>`
   - Verifies checkpoint image was created
   - Displays checkpoint image annotations

5. **Cleanup**
   - Removes all pods (including the checkpointed pod)
   - Verifies pod removal

6. **Restore**
   - Restores pod from checkpoint image
   - Waits for containers to be restored
   - Verifies pod and containers are running

7. **Verification**
   - Checks that both containers are in RUNNING state
   - Verifies container names match original
   - Checks for test files in containers

8. **Cleanup on Exit**
   - Stops CRI-O
   - Removes checkpoint images
   - Removes test directories
   - Removes all pods

## Test Output

The script provides colored output:

- **Green [INFO]**: Normal progress messages
- **Yellow [WARN]**: Warning messages (non-fatal)
- **Red [ERROR]**: Error messages (test failures)

## Example Output

```text
[INFO] === CRI-O Pod Checkpoint/Restore Test ===
[INFO] Test timestamp: Thu Jan 23 09:54:00 UTC 2026
[INFO] Checking prerequisites...
[INFO] All prerequisites satisfied
[INFO] Starting CRI-O server...
[INFO] CRI-O started with PID 12345
[INFO] Waiting for CRI-O socket...
[INFO] CRI-O socket ready
[INFO] Pulling container 1 image: quay.io/adrianreber/wildfly-hello
[INFO] Pulling container 2 image: quay.io/adrianreber/counter
[INFO] === Testing Pod Checkpoint and Restore ===
[INFO] Creating pod...
[INFO] Pod created: abc123...
[INFO] Creating first container...
[INFO] Container 1 created: def456...
[INFO] Creating second container...
[INFO] Container 2 created: ghi789...
[INFO] Starting containers...
[INFO] Waiting for containers to be ready...
[INFO] Verifying containers are running...
[INFO] Both containers are running
[INFO] Checkpointing pod to image: localhost/checkpoint-pod:test-12345
[INFO] Checkpoint image created successfully
[INFO] Removing all pods...
[INFO] Pod removed successfully
[INFO] Restoring pod from image: localhost/checkpoint-pod:test-12345
[INFO] Pod restored: xyz123...
[INFO] Found 2 containers in restored pod
[INFO] Container container1 (aaa111) is running
[INFO] Container container2 (bbb222) is running
[INFO] Pod checkpoint and restore test completed successfully!
[INFO] All tests passed!
[INFO] Cleaning up test environment...
[INFO] Test completed successfully!
```

## Debugging

### CRI-O Output

The script redirects CRI-O output to a log file to keep test output clean:

**On Success:**

- Logs are saved to `/tmp/crio-test.XXXXXX/crio.log`
- Log directory is cleaned up automatically
- Only test progress messages are shown

**On Failure:**

- Last 50 lines of CRI-O logs are displayed automatically
- Full log path is shown for further investigation
- Log directory is preserved for debugging

### View CRI-O Logs Manually

```bash
# The log path is shown at startup:
# [INFO] CRI-O started with PID 12345 (logs: /tmp/crio-test.XXXXXX/crio.log)

# Or find the latest log:
ls -ltr /tmp/crio-test.*/crio.log | tail -1

# View logs:
tail -f /tmp/crio-test.XXXXXX/crio.log
```

### Manual Testing Steps

If the script fails, you can manually test with:

```bash
# Start CRI-O
sudo bin/crio --pinns-path bin/pinns --default-runtime runc --log-level debug &

# Create pod with containers
POD_ID=$(crictl runp /path/to/pod-config.json)
CONTAINER_ID=$(crictl create $POD_ID /path/to/container-config.json /path/to/pod-config.json)
CONTAINER2_ID=$(crictl create $POD_ID /path/to/container2-config.json /path/to/pod-config.json)

# Start containers
crictl start $CONTAINER_ID $CONTAINER2_ID

# Checkpoint
crictl -t 20s checkpointp --export=localhost/checkpoint-pod:latest $POD_ID

# Remove pod
crictl rmp -fa

# Restore
crictl -t 20s restorep -l localhost/checkpoint-pod:latest
```

## Common Issues

### CRIU Not Found

```text
[ERROR] CRIU is not available or version is too old
```

**Solution**: Install CRIU with pod checkpoint support:

```bash
sudo dnf install criu  # Fedora/RHEL
sudo apt install criu  # Debian/Ubuntu
```

### Permission Denied

```text
[ERROR] This script must be run as root
```

**Solution**: Run with sudo:

```bash
sudo ./test/checkpoint-restore-pod-test.sh
```

### CRI-O Already Running

```text
[ERROR] Timeout waiting for CRI-O socket
```

**Solution**: Stop existing CRI-O instance:

```bash
sudo systemctl stop crio
sudo pkill -9 crio
```

### crictl Missing Pod Checkpoint Commands

```text
[ERROR] crictl does not support 'checkpointp' command
[ERROR] crictl does not support 'restorep' command
```

**Solution**: Build crictl from cri-tools with pod checkpoint support:

```bash
cd ../cri-tools
make crictl
# Verify commands are available:
./build/bin/linux/amd64/crictl --help | grep -E "checkpointp|restorep"
```

## Architecture

Based on containerd's checkpoint-restore-cri-test.sh, adapted for:

- Pod-level checkpointing (not just containers)
- CRI-O-specific paths and binaries
- Multiple containers per pod
- OCI image-based checkpoints
- Sources `test/common.sh` for standard CRI-O test variables and utilities

## Files Created

- `/tmp/crio-test.XXXXXX/` - Temporary test directory
  - `crio.log` - CRI-O server logs
  - `crictl.yaml` - crictl configuration (suppresses warnings)
  - `pod-config.json` - Pod configuration
  - `container1-config.json` - First container configuration
  - `container2-config.json` - Second container configuration
- `localhost/checkpoint-pod:test-<PID>` - Checkpoint OCI image

All files are cleaned up on script exit (unless test fails, then logs are preserved).
