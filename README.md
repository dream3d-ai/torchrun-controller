# Torchrun Controller

The Torchrun Controller provides Kubernetes-native management of distributed PyTorch training jobs using torchrun. It consists of two main components:

## Motivation

The Torchrun Controller addresses fundamental challenges in scaling ML training infrastructure by creating a clear separation of concerns between DevOps teams and ML practitioners. This separation enables both teams to work more efficiently:

### For DevOps Teams

- **Centralized Resource Management**: Define and manage compute resources, GPU allocations, and scheduling policies through TorchrunQueue resources
- **Infrastructure as Code**: Control container images, dependencies, volume mounts, and worker coordination without touching training code
- **Standardized Operations**: Enforce security policies, resource limits, and cluster best practices across all training jobs

### For ML Practitioners

- **Focus on Research**: Submit jobs with just a command and node count - no need to understand Kubernetes internals
- **Rapid Iteration**: Local workspace syncing eliminates the Docker build/push cycle, enabling instant code changes
- **Familiar Development**: Work in your local environment with your preferred tools, then seamlessly scale to the cluster

### Key Differentiators

**vs. TorchX**: TorchX builds a new Docker image for each run, leading to:

- Slow iteration cycles waiting for builds
- Registry bloat with thousands of temporary images
- Complex cleanup of experimental artifacts

**vs. Kubeflow**: Kubeflow Training Operator requires:

- Single python file definitions with limited flexibility
- Pre-built Docker images with all dependencies including a stable codebase

**The Torchrun Controller Solution**:

- **Two Simple Abstractions**:
  - `TorchrunQueue`: Defines HOW jobs run (resources, images, policies)
  - `TorchrunJob`: Defines WHAT runs (command, nodes, workspace)
- **Local Workspace Syncing**: Your entire working directory is automatically packaged and mounted in pods
- **Zero Docker Builds**: Iterate on code without rebuilding images
- **Clean Separation**: DevOps configures queues, researchers submit jobs

This design philosophy enables ML teams to maintain the fast iteration speed of local development while seamlessly scaling to multi-node distributed training on Kubernetes.

## Controllers

### TorchrunQueue Controller

The TorchrunQueue controller manages job templates and configurations for a specific training queue. It defines:

- **Pod templates**: Base configuration for all jobs submitted to this queue
- **Resource limits**: GPU, CPU, and memory allocation per node
- **Container defaults**: Base image, working directory, and security settings
- **Storage templates**: Default PVC configurations for workspace, datasets, and checkpoints
- **Scheduling policies**: Node selectors, affinity rules, tolerations, and priority
- **Kai-scheduler integration**: Maps to kai-scheduler queues for gang scheduling

A TorchrunQueue acts as a template that defines HOW jobs should be created when submitted to this queue. Multiple jobs can reference the same queue, inheriting its configuration while adding job-specific details.

When you create a TorchrunQueue, the controller:

1. Creates a corresponding kai-scheduler Queue resource (if configured)
2. Validates the pod template configuration
3. Makes the queue available for TorchrunJob submissions

Example TorchrunQueue:

```yaml
apiVersion: torchrun.ai/v1alpha1
kind: TorchrunQueue
metadata:
  name: gpu-training-queue
spec:
  queue:
    name: "gpu-training" # kai-scheduler queue name
    parentQueue: "default" # parent queue in kai-scheduler hierarchy
    resources:
      gpu:
        quota: -1 # -1 means unlimited
        limit: -1
        overQuotaWeight: 1
      cpu:
        quota: -1
        limit: -1
        overQuotaWeight: 1
      memory:
        quota: -1
        limit: -1
        overQuotaWeight: 1
  distributed:
    backend: "nccl"
    rdzvBackend: "etcd-v2"
    rdzvEndpoint: "etcd.etcd-system.svc.cluster.local:2379"
    port: 29500
  podTemplate:
    metadata:
      labels:
        queue: gpu-training
    spec:
      containers:
        - name: trainer
          image: "pytorch/pytorch:2.0.1-cuda11.7-cudnn8-runtime"
          resources:
            requests:
              nvidia.com/gpu: 8
              cpu: 96
              memory: "512Gi"
            limits:
              nvidia.com/gpu: 8
              cpu: 96
              memory: "512Gi"
  serviceAccountName: "default"
```

### TorchrunJob Controller

The TorchrunJob controller manages actual training job instances. It:

- **References a TorchrunQueue**: Inherits pod template and configuration
- **Mounts local workspace**: Automatically uploads and mounts your local working directory for fast iteration
- **Manages job lifecycle**: Creates worker pods, services, and manages distributed setup
- **Handles storage**: Creates job-specific PVCs based on queue templates
- **Configures torchrun**: Sets up distributed training with proper environment variables
- **Supports suspend/resume**: Allows pausing and resuming jobs while preserving state

Key features for development workflow:

- **Local workspace sync**: Your current directory is automatically packaged and mounted in all worker pods
- **Hot reload support**: Changes to your local code can be synced without recreating the entire job
- **Ephemeral storage**: Workspace PVCs are cleaned up with the job by default
- **Persistent checkpoints**: Checkpoint storage can be configured to persist beyond job lifetime

Example TorchrunJob:

```yaml
apiVersion: torchrun.ai/v1alpha1
kind: TorchrunJob
metadata:
  name: training-job
spec:
  jobQueue: gpu-training-queue # Reference to TorchrunQueue
  numNodes: 2 # Number of nodes (not GPUs)
  command: "python train.py --epochs 100"
  setupCommand: "pip install -r requirements.txt" # Optional setup
  workspaceStorage:
    size: "20Gi"
    mountPath: "/app"
    source: "zip" # Local workspace from kubectl-torchrun plugin
  reliability:
    maxRestarts: 3
    restartPolicy: "OnFailure"
    ttlSecondsAfterFinished: 3600
  # Additional environment variables can be specified
  env:
    - name: WANDB_API_KEY
      value: "your-api-key"
```

## Installation

1. Install CRDs:

```bash
kubectl apply -f crd/
```

2. Deploy the controller:

```bash
kubectl apply -f config/deployment.yaml
```

3. Create RBAC resources:

```bash
kubectl apply -f config/rbac/role.yaml
```

## Features

### Development Workflow

- **Local workspace mounting**: Your local directory is automatically synced to all worker pods
- **Fast iteration**: Make changes locally and submit jobs without building images
- **Workspace isolation**: Each job gets its own workspace PVC with your code

### Queue Management

- **Reusable templates**: TorchrunQueues provide consistent configuration across jobs
- **Resource allocation**: Define node resources and pod templates per queue
- **Flexible overrides**: Jobs can override queue settings when needed

### Training Features

- **Automatic pod distribution**: Calculates optimal distribution across nodes based on available resources
- **Gang scheduling**: Integration with kai-scheduler for coordinated pod scheduling
- **Storage management**: Automatic PVC creation and lifecycle management
- **Distributed training support**: Automatic setup of torchrun with etcd rendezvous
- **Job lifecycle management**: Support for suspend/resume, TTL, and restart policies

## Development

Build the controller:

```bash
go build -o bin/controller .
```

Generate CRDs from types:

```bash
./bin/controller-gen crd:maxDescLen=0 paths="./..." output:dir=config/crd/bases
```

Generate deepcopy methods:

```bash
./bin/controller-gen object:headerFile="hack/boilerplate.go.txt" paths="./..."
```

## Usage

### 1. Create a TorchrunQueue

```yaml
apiVersion: torchrun.ai/v1alpha1
kind: TorchrunQueue
metadata:
  name: h100-training
spec:
  queue:
    name: gpu-h100-queue # Maps to kai-scheduler queue
    parentQueue: default
    resources:
      gpu:
        quota: -1
        limit: -1
      cpu:
        quota: -1
        limit: -1
      memory:
        quota: -1
        limit: -1
  distributed:
    backend: nccl
    rdzvBackend: etcd-v2
    rdzvEndpoint: "etcd.etcd-system.svc.cluster.local:2379"
  podTemplate:
    metadata:
      labels:
        node-type: h100
    spec:
      nodeSelector:
        gpu.nvidia.com/class: H100
      containers:
        - name: trainer
          image: dream3dml/pytorch:latest
          resources:
            requests:
              nvidia.com/gpu: 8
              cpu: 128
              memory: "1024Gi"
            limits:
              nvidia.com/gpu: 8
              cpu: 128
              memory: "1024Gi"
          volumeMounts:
            - name: dshm
              mountPath: /dev/shm
      volumes:
        - name: dshm
          emptyDir:
            medium: Memory
            sizeLimit: 64Gi
```

### 2. Submit a TorchrunJob

```yaml
apiVersion: torchrun.ai/v1alpha1
kind: TorchrunJob
metadata:
  name: vit-training
spec:
  jobQueue: h100-training
  command: "python train.py --model vit_large"
  numNodes: 4 # Will use 4 nodes with 8 GPUs each
  workspaceStorage:
    size: "100Gi"
    source: "zip"
    mountPath: "/app"
  reliability:
    maxRestarts: 3
    restartPolicy: "OnFailure"
```

### 3. Monitor the Job

```bash
# List jobs with key information
kubectl get torchrunjobs

# Get detailed status
kubectl describe torchrunjob vit-training

# Watch worker pods
kubectl get pods -l torchrun-job-name=vit-training -w
```

## Development Workflow

The controller is designed to support fast iteration during development:

### 1. Local Development Setup

```bash
# In your project directory
cd ~/my-training-project

# Submit job with local workspace
kubectl torchrun submit \
  --queue h100-training \
  --num-nodes 1 \
  "python train.py --epochs 10"
```

Your local directory is automatically:

- Archived and uploaded to the cluster
- Mounted in all worker pods at `/app`
- Available immediately without building Docker images

### 2. Iterative Development

```bash
# Make changes to your code locally
vim train.py

# Submit another job - changes are included
kubectl torchrun submit \
  --queue h100-training \
  --num-nodes 1 \
  "python train.py --epochs 20"
```

### 3. Using Different Queues

Create queues for different purposes:

```yaml
# Development queue - smaller resources, faster scheduling
apiVersion: torchrun.ai/v1alpha1
kind: TorchrunQueue
metadata:
  name: dev-queue
spec:
  queue:
    name: dev
    parentQueue: default
  podTemplate:
    spec:
      containers:
        - name: trainer
          image: pytorch/pytorch:latest
          resources:
            requests:
              nvidia.com/gpu: 4
              cpu: 32
              memory: "128Gi"
            limits:
              nvidia.com/gpu: 4
              cpu: 32
              memory: "128Gi"
---
# Production queue - full resources, optimized settings
apiVersion: torchrun.ai/v1alpha1
kind: TorchrunQueue
metadata:
  name: prod-queue
spec:
  queue:
    name: production
    parentQueue: default
    resources:
      gpu:
        quota: 64 # Reserve 64 GPUs for production
  distributed:
    backend: nccl
    rdzvBackend: etcd-v2
  podTemplate:
    metadata:
      labels:
        tier: production
    spec:
      priorityClassName: production-priority
      containers:
        - name: trainer
          image: company/pytorch-optimized:latest
          resources:
            requests:
              nvidia.com/gpu: 8
              cpu: 96
              memory: "512Gi"
            limits:
              nvidia.com/gpu: 8
              cpu: 96
              memory: "512Gi"
```

## Features

### Flexible Node Distribution

The controller manages distributed training across nodes:

- Single node if possible (better performance)
- Multiple nodes when needed (scales out)
- Each node runs the number of GPUs defined in the TorchrunQueue pod template

### Storage Management

- **Workspace**: Per-job storage for code/data (size scales with GPUs)
- **Datasets**: Shared read-only storage across jobs
- **Checkpoints**: Shared read-write storage for model checkpoints

### Distributed Training

- Automatic torchrun configuration
- Support for elastic training (min/max nodes)
- etcd-based rendezvous for coordination
- Proper environment variable setup

### Lifecycle Management

- Job suspend/resume support
- TTL-based cleanup
- Restart policies
- Active deadline enforcement

## Examples

See the `examples/` directory for more usage patterns:

- Basic single/multi-node training
- Elastic training with node scaling
- CPU-only jobs
- Development workflow with local workspace mounting
- Multiple queues for different workload types
- Queue overrides for priority jobs

## Best Practices

1. **Queue Design**: Create TorchrunQueues for different workload types (e.g., development, production, long-running)
2. **Local Development**: Use the kubectl-torchrun plugin to automatically sync your local workspace
3. **Resource Allocation**: Set resources in TorchrunQueue based on your cluster's node configuration
4. **Storage Classes**: Use fast storage for checkpoints, cheaper storage for datasets
5. **Elastic Training**: Use min/max nodes for fault tolerance and better cluster utilization
6. **TTL Settings**: Set appropriate TTL to automatically clean up completed jobs
7. **Monitoring**: Use labels and annotations for better observability

## Troubleshooting

### Job Stuck in Provisioning

- Check if TorchrunQueue exists: `kubectl get torchrunqueue <name>`
- Verify resource availability: `kubectl describe nodes`
- Check scheduler logs if using kai-scheduler

### Workers Not Connecting

- Verify networking settings in TorchrunQueue
- Check if service was created for multi-node jobs
- Ensure etcd is accessible for distributed training

### Workspace Issues

- Ensure local workspace was properly archived by kubectl-torchrun plugin
- Check sync pod logs: `kubectl logs <job-name>-sync-0`
- Verify workspace PVC was created: `kubectl get pvc <job-name>-workspace`

### Storage Issues

- Verify storage classes exist: `kubectl get storageclass`
- Check PVC status: `kubectl get pvc -l torchrun-job-name=<job-name>`
- Ensure sufficient storage quota

## Contributing

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

## License

This project is licensed under the Apache 2.0 License.
