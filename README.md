# Torchrun Controller

The Torchrun Controller provides Kubernetes-native management of distributed PyTorch training jobs using torchrun. It consists of two main components:

## Controllers

### JobQueue Controller

The JobQueue controller manages training queue configurations and automatically creates corresponding kai-scheduler Queue resources. JobQueues define:

- **Kai-scheduler integration**: Maps to kai-scheduler queues for gang scheduling
- **Node resources**: GPU, CPU, and memory available per node for optimal pod distribution
- **Container configuration**: Base image, working directory, and security settings
- **Storage configuration**: Workspace, datasets, and checkpoints with configurable PVC settings
- **Networking and scheduling**: DNS, affinity, tolerations, and priority settings

When you create a JobQueue, the controller:

1. Creates a corresponding kai-scheduler Queue resource
2. Monitors the Queue status
3. Updates the JobQueue status with conditions

Example JobQueue:

```yaml
apiVersion: torchrun.ai/v1alpha1
kind: JobQueue
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

The TorchrunJob controller manages distributed PyTorch training jobs. It:

- References a JobQueue for configuration inheritance
- Calculates optimal node and pod distribution based on requested GPUs
- Creates Kubernetes Jobs with proper torchrun configuration
- Manages PVCs for workspace, datasets, and checkpoints
- Handles distributed training setup (master service, environment variables)
- Supports job suspension and resumption

Example TorchrunJob:

```yaml
apiVersion: torchrun.ai/v1alpha1
kind: TorchrunJob
metadata:
  name: training-job
spec:
  jobQueue: gpu-training-queue # Reference to JobQueue
  numGPUs: 16 # Total GPUs needed
  command: "python train.py --epochs 100"
  distributed:
    backend: "nccl"
    rdzvBackend: "etcd-v2"
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

- **Automatic pod distribution**: Calculates optimal distribution across nodes based on available resources
- **Gang scheduling**: Integration with kai-scheduler for coordinated pod scheduling
- **Storage management**: Automatic PVC creation and lifecycle management
- **Flexible configuration**: JobQueues provide reusable configurations for multiple jobs
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

### 1. Create a JobQueue

```yaml
apiVersion: torchrun.ai/v1alpha1
kind: JobQueue
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
- Custom workspace sources (git, S3)
- Queue overrides for priority jobs

## Best Practices

1. **Resource Allocation**: Set resources in JobQueue based on your cluster's node configuration
2. **Storage Classes**: Use fast storage for checkpoints, cheaper storage for datasets
3. **Elastic Training**: Use min/max nodes for fault tolerance and better cluster utilization
4. **TTL Settings**: Set appropriate TTL to automatically clean up completed jobs
5. **Monitoring**: Use labels and annotations for better observability

## Troubleshooting

### Job Stuck in Provisioning

- Check if JobQueue exists: `kubectl get jobqueue <name>`
- Verify resource availability: `kubectl describe nodes`
- Check scheduler logs if using kai-scheduler

### Workers Not Connecting

- Verify networking settings in JobQueue
- Check if service was created for multi-node jobs
- Ensure etcd is accessible for distributed training

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
