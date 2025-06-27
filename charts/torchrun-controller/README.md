# Torchrun Controller Helm Chart

This Helm chart deploys the Torchrun Controller and creates TorchrunQueue resources for managing distributed PyTorch training jobs on Kubernetes.

## Prerequisites

- Kubernetes 1.19+
- Helm 3.0+
- kai-scheduler installed (for queue management)
- NVIDIA GPU operator (if using GPUs)

## Installation

### Add the Helm repository (if published):

```bash
helm repo add dream3d https://charts.dream3d.ai
helm repo update
```

### Install from local directory:

```bash
helm install torchrun-controller ./k8s/torchrun-controller/charts/torchrun-controller \
  --namespace torchrun-system \
  --create-namespace
```

### Install with custom values:

```bash
helm install torchrun-controller ./k8s/torchrun-controller/charts/torchrun-controller \
  --namespace torchrun-system \
  --create-namespace \
  --values my-values.yaml
```

## Uninstallation

```bash
helm uninstall torchrun-controller --namespace torchrun-system
```

## Configuration

The following table lists the configurable parameters and their default values.

### Controller Configuration

| Parameter                              | Description                   | Default                         |
| -------------------------------------- | ----------------------------- | ------------------------------- |
| `controller.replicaCount`              | Number of controller replicas | `1`                             |
| `controller.image.repository`          | Controller image repository   | `dream3dml/torchrun-controller` |
| `controller.image.tag`                 | Controller image tag          | `latest`                        |
| `controller.image.pullPolicy`          | Image pull policy             | `IfNotPresent`                  |
| `controller.resources.limits.cpu`      | CPU limit                     | `500m`                          |
| `controller.resources.limits.memory`   | Memory limit                  | `128Mi`                         |
| `controller.resources.requests.cpu`    | CPU request                   | `10m`                           |
| `controller.resources.requests.memory` | Memory request                | `64Mi`                          |
| `controller.nodeSelector`              | Node selector                 | `{}`                            |
| `controller.tolerations`               | Tolerations                   | `[]`                            |
| `controller.affinity`                  | Affinity rules                | `{}`                            |

### Namespace Configuration

| Parameter          | Description                          | Default           |
| ------------------ | ------------------------------------ | ----------------- |
| `namespace.create` | Create namespace if it doesn't exist | `true`            |
| `namespace.name`   | Namespace name                       | `torchrun-system` |

### Queue Configuration

| Parameter        | Description                       | Default         |
| ---------------- | --------------------------------- | --------------- |
| `queues.enabled` | Enable creation of default queues | `true`          |
| `queues.items`   | List of queues to create          | See values.yaml |

### RBAC Configuration

| Parameter              | Description                            | Default |
| ---------------------- | -------------------------------------- | ------- |
| `rbac.create`          | Create RBAC resources                  | `true`  |
| `rbac.additionalRules` | Additional rules to add to ClusterRole | `[]`    |

### Monitoring Configuration

| Parameter                  | Description                          | Default |
| -------------------------- | ------------------------------------ | ------- |
| `serviceMonitor.enabled`   | Enable ServiceMonitor creation       | `false` |
| `serviceMonitor.interval`  | Scrape interval                      | `30s`   |
| `serviceMonitor.namespace` | ServiceMonitor namespace             | `""`    |
| `serviceMonitor.labels`    | Additional labels for ServiceMonitor | `{}`    |

## Queue Examples

### Default Queue Configuration

```yaml
queues:
  enabled: true
  items:
    - name: production
      namespace: production
      queue:
        name: prod
        parentQueue: root
        resources:
          gpu:
            quota: 16
            limit: 32
      podTemplate:
        spec:
          tolerations:
            - key: "nvidia.com/gpu"
              operator: "Equal"
              effect: "NoSchedule"
          nodeSelector:
            workload-type: "gpu"
```

### Creating Additional Queues Post-Installation

```yaml
apiVersion: torchrun.ai/v1alpha1
kind: TorchrunQueue
metadata:
  name: research-queue
  namespace: research
spec:
  queue:
    name: research
    parentQueue: root
    resources:
      gpu:
        quota: 8
        limit: 16
  serviceAccountName: default
  podTemplate:
    spec:
      tolerations:
        - key: "nvidia.com/gpu"
          operator: "Equal"
          effect: "NoSchedule"
```

## Using TorchrunJobs

Once the controller is installed, you can create TorchrunJobs:

```yaml
apiVersion: torchrun.ai/v1alpha1
kind: TorchrunJob
metadata:
  name: bert-training
  namespace: default
spec:
  queue: default
  image: nvcr.io/nvidia/pytorch:24.01-py3
  workers: 4
  script: train.py
  args:
    - --model=bert-large
    - --epochs=10
    - --batch-size=32
  workspace:
    size: 10Gi
  resources:
    requests:
      nvidia.com/gpu: 1
    limits:
      nvidia.com/gpu: 1
```

## Troubleshooting

### Check controller logs:

```bash
kubectl logs -n torchrun-system deployment/torchrun-controller-manager
```

### Verify queues are created:

```bash
kubectl get torchrunqueues --all-namespaces
```

### Check if CRDs are installed:

```bash
kubectl get crd torchrunjobs.torchrun.ai
kubectl get crd torchrunqueues.torchrun.ai
```

## Support

For issues and feature requests, please visit: https://github.com/dream3d-ai/dream3d/issues
