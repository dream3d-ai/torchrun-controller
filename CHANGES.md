# Torchrun Controller Changes

## Summary

The torchrun controller has been updated with the following major changes:

### 1. Workspace Sync Pod Workflow

The controller now implements a two-stage deployment process:
1. **Workspace PVC Creation**: Creates a persistent volume for workspace storage
2. **Sync Pod Creation**: Deploys a sync pod that prepares the workspace
3. **Worker Job Creation**: Only creates worker pods after sync pod succeeds

This ensures workspace data is properly prepared before training begins.

### 2. Switch from numGPUs to numNodes

- Replaced `numGPUs` field with `numNodes` in TorchrunJob spec
- Each node is now a single pod (simplified from previous pod distribution logic)
- The number of GPUs per pod is determined by the JobQueue's node resources
- Updated print columns to show "Nodes" instead of "GPUs"

### 3. Key Implementation Details

#### Workspace Sync Process
- Sync pod validates and extracts workspace.zip 
- Supports multiple workspace sources: zip, git, s3, existing PVC
- Worker pods wait for `.sync_success` marker before copying workspace
- Uses single-writer (sync pod), multiple-reader (worker pods) pattern

#### Pod Configuration
- Uses Kubernetes Job's indexed completion mode for predictable pod names
- Removed need for separate Service - pods use direct DNS names
- Master node is always `{job-name}-0` for multi-node training
- Each worker pod copies workspace from shared PVC to local volume

#### Status Updates
- Added "Syncing" phase while workspace is being prepared
- Added "WorkspaceReady" condition to track sync status
- Updated worker status calculation for single pod per node

### 4. Example Usage

```yaml
apiVersion: torchrun.ai/v1alpha1
kind: TorchrunJob
metadata:
  name: multi-node-training
spec:
  jobQueue: gpu-queue
  numNodes: 4  # 4 nodes, each with resources defined in JobQueue
  command: "python train.py"
  
  volumes:
    workspace:
      enabled: true
      source: zip  # Will wait for workspace.zip upload
```

### 5. Benefits

- **Reliability**: Workspace preparation is validated before training starts
- **Efficiency**: Single workspace preparation for all worker pods
- **Flexibility**: Supports various workspace sources
- **Simplicity**: Each node = one pod (no complex pod distribution)
- **Scalability**: Optimized for distributed training with shared storage 