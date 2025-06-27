# TorchrunQueue Examples

This directory contains example TorchrunQueue resources for the torchrun controller.

## What is TorchrunQueue?

TorchrunQueue is a custom resource that maps torchrun jobs to kai-scheduler queues for gang scheduling. It allows you to:

- Map jobs to specific kai-scheduler queues
- Apply common pod configurations to all jobs in a queue
- Set resource quotas and limits via kai-scheduler integration
- Organize workloads by team, priority, or workload type

## Files

- `simple-queue.yaml` - A minimal example to get started
- `torchrun-queue.yaml` - Comprehensive examples showing various configurations

## Quick Start

1. First, ensure you have kai-scheduler installed with appropriate queues:

   ```bash
   kubectl get queues -A
   ```

2. Create a basic TorchrunQueue:

   ```bash
   kubectl apply -f simple-queue.yaml
   ```

3. Reference the queue in your TorchrunJob:
   ```yaml
   apiVersion: torchrun.ai/v1
   kind: TorchrunJob
   metadata:
     name: my-job
   spec:
     queue: my-training-queue # Reference the TorchrunQueue name
     # ... rest of job spec
   ```

## Key Fields

### Required Fields

- `spec.kaiQueue` - The kai-scheduler queue name this TorchrunQueue maps to

### Optional Fields

- `spec.parentKaiQueue` - Parent queue in kai-scheduler hierarchy (default: "default")
- `spec.serviceAccountName` - Service account for pods (default: "default")
- `spec.podTemplate` - Pod template to apply to all jobs in this queue
  - `metadata` - Labels and annotations to add to pods
  - `spec` - Any valid pod spec fields (merged with job's pod spec)

## Common Use Cases

### 1. Team-based Queues

Create separate queues for different teams with appropriate resource quotas:

```yaml
apiVersion: torchrun.ai/v1
kind: TorchrunQueue
metadata:
  name: ml-team-queue
spec:
  kaiQueue: ml-team
  podTemplate:
    metadata:
      labels:
        team: ml
        cost-center: ml-research
```

### 2. Priority-based Queues

Set up queues for different priority levels:

```yaml
apiVersion: torchrun.ai/v1
kind: TorchrunQueue
metadata:
  name: production-queue
spec:
  kaiQueue: production
  podTemplate:
    spec:
      priorityClassName: high-priority
      nodeSelector:
        workload: production
```

### 3. Hardware-specific Queues

Route jobs to specific hardware:

```yaml
apiVersion: torchrun.ai/v1
kind: TorchrunQueue
metadata:
  name: a100-queue
spec:
  kaiQueue: gpu-a100
  podTemplate:
    spec:
      nodeSelector:
        gpu-type: a100
      tolerations:
        - key: nvidia.com/gpu
          operator: Equal
          value: a100
          effect: NoSchedule
```

## Pod Template Merging

The `podTemplate` in TorchrunQueue is merged with the job's pod spec:

- Labels and annotations are merged (job takes precedence)
- Node selectors are merged (job takes precedence)
- Tolerations are appended
- Environment variables are merged (job takes precedence)
- Volumes and volume mounts are appended

## Monitoring

Check queue status:

```bash
# List all TorchrunQueues
kubectl get torchrunqueues

# Describe a specific queue
kubectl describe torchrunqueue my-training-queue

# Check jobs in a queue
kubectl get torchrunjobs -l queue=my-training-queue
```

## Best Practices

1. **Naming Convention**: Use descriptive names that indicate the queue's purpose (e.g., `research-gpu-queue`, `prod-training-queue`)

2. **Resource Organization**: Align TorchrunQueue names with kai-scheduler queue names for clarity

3. **Pod Templates**: Keep pod templates minimal and focused on queue-wide settings

4. **Service Accounts**: Use appropriate service accounts with necessary RBAC permissions

5. **Monitoring**: Add consistent labels and annotations for observability

## Troubleshooting

If jobs are not being scheduled:

1. Verify the kai-scheduler queue exists: `kubectl get queues`
2. Check TorchrunQueue status: `kubectl describe torchrunqueue <name>`
3. Ensure the queue has available resources in kai-scheduler
4. Check job events: `kubectl describe torchrunjob <name>`
