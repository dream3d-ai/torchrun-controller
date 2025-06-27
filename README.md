# Torchrun Controller

The Torchrun Controller provides Kubernetes-native management of distributed PyTorch training jobs using torchrun. It consists of two main components:

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
  kaiQueue: "gpu-training" # kai-scheduler queue name
  parentKaiQueue: "default" # parent queue in kai-scheduler hierarchy
  nodeResources:
    gpus: 8 # GPUs per node
    cpus: 96 # CPUs per node
    memory: "512Gi" # Memory per node
  image: "pytorch/pytorch:2.0.1-cuda11.7-cudnn8-runtime"
  storage:
    workspace:
      enabled: true
      sizePerGPU: "10Gi"
    checkpoints:
      enabled: true
      sizePerGPU: "50Gi"
  scheduling:
    scheduler: "kai-scheduler"
    priorityClassName: "gpu-priority"
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
  numGPUs: 16 # Total GPUs needed
  command: "python train.py --epochs 100"
  distributed:
    backend: "nccl"
    rdzvBackend: "etcd-v2"
  # Local workspace from kubectl-torchrun plugin is automatically mounted
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
  kaiQueue: gpu-h100-queue # Maps to kai-scheduler queue
  image: dream3dml/pytorch:latest
  nodeResources:
    cpus: 128 # Total CPUs per node
    memory: "1024Gi" # Total memory per node
    gpus: 8 # Total GPUs per node
    gpuType: "h100-80gb"
  scheduling:
    scheduler: kai-scheduler
    nodeSelector:
      gpu.nvidia.com/class: H100
  storage:
    shmSizePerGPU: "8Gi"
    workspace:
      enabled: true
      sizePerGPU: "10Gi"
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
  numGPUs: 32 # Will use 4 nodes with 8 GPUs each
  distributed:
    backend: nccl
    rdzvBackend: etcd-v2
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
  --gpus 8 \
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
  --gpus 8 \
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
  nodeResources:
    gpus: 4
    cpus: 32
    memory: "128Gi"
  image: pytorch/pytorch:latest
---
# Production queue - full resources, optimized settings
apiVersion: torchrun.ai/v1alpha1
kind: TorchrunQueue
metadata:
  name: prod-queue
spec:
  nodeResources:
    gpus: 8
    cpus: 96
    memory: "512Gi"
  image: company/pytorch-optimized:latest
  scheduling:
    priorityClassName: production-priority
```

## Features

### Flexible GPU Distribution

The controller automatically calculates optimal pod distribution:

- Single node if possible (better performance)
- Multiple nodes when needed (scales out)
- Handles partial node utilization

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
