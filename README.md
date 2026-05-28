# Worker Scaler Controller

A **custom Kubernetes Autoscaler**.

Instead of using HPA based on CPU or RAM, the K8s controller in this sandbox project monitors real-time application load (Redis Streams depth) to dynamically scale consumer replicas.

### Core Architecture

This implementation bridges K8s orchestration with application-level concurrency by leveraging:

* **Core Engine:** [`workerpool`](https://www.google.com/search?q=%5Bhttps://github.com/alesr/workerpool%5D(https://github.com/alesr/workerpool)) handles in-memory task execution and backpressure within the pods
* **Ingress Bridge:** [`redisstream`](https://www.google.com/search?q=%5Bhttps://github.com/alesr/workerpool/tree/main/adapters/redisstream%5D(https://github.com/alesr/workerpool/tree/main/adapters/redisstreams)) adapter to pull tasks from Redis Streams into the worker pool

---

1. **Infrastructure**: Redis-backed task pipeline (Producer + Worker)
2. **Observability**: Live stream metrics pulled via `XINFO GROUPS`
3. **Control Loop**: Go binary using `k8s.io/client-go` to check stream load and `PATCH` deployment replicas

---

## Workflow: The Scaling Lab

This project is configured to deploy directly to a remote K3s cluster (in my case Raspberry Pi 4) isolated within the `worker-scaler` namespace.

### 1. Boot the Environment

Ensure your `KUBECONFIG` is pointing to your remote cluster, then execute the full pipeline. This will build the cross-platform images, push them to your registry, create the namespace, and apply all manifests:

```bash
make deploy-all
```

### 2. Inject Load (Simulation)

To test how the autoscaler reacts, we need to simulate traffic. The producer component floods the stream with synthetic tasks, artificially inflating the `Lag` and `Pending` metrics so we can watch the controller trigger scale-up events in real time:

```bash
    kubectl apply -n worker-scaler -f manifests/producer-job.yaml
```

### 3. Introspection

```bash
# watch the controller logs to see scaling
kubectl logs -n worker-scaler -l app=scaler-controller -f

# worker activity
kubectl logs -n worker-scaler -l app=redis-worker --tail=-1 -f

# inspect stream data
kubectl exec -n worker-scaler -it deployment/redis -- redis-cli XINFO GROUPS orders_stream
```

---

## System Heuristics

These rules govern how the autoscaler calculates demand and protects cluster stability.

### 1. The Scaling Formula

The controller evaluates target replicas purely on backlog demand, bounded by safe limits.

$$\text{DesiredReplicas} = \text{clamp}\left( \left\lceil \frac{\text{Backlog}}{\text{TasksPerPod}} \right\rceil, \text{MinReplicas}, \text{MaxReplicas} \right)$$

* **Backlog**: total active work ($Lag + Pending$)
* **TasksPerPod**: target capacity per pod (Goroutines $\times$ target efficiency)
* **Clamp**: prevents scaling outside predefined boundaries

### 2. Backlog Measurement

> **Rule:** calculate backlog using `XINFO GROUPS`.

* **Lag**: messages sitting in the stream that have never been read
* **Pending**: msgs read by a worker but not yet acknowledged (`XACK`)

### 3. Concurrency

> **Rule:** Maximize vertical capacity before scaling horizontally.

* **Goroutines (Internal)**: cheap and fast. Controlled inside the application via `workerpool`
* **Pods (External)**: expensive and slow to provision. Controlled by the K8s API
* Keep the internal pool optimized for high throughput. Only scale pods horizontally when underlying node resources or network limits saturate.

### 4. Stability & Anti-Flicker

#### A. Keep MinReplicas at 1

Keeping at least one pod active eliminates cold-start latency and keeps the Redis connection warm.

#### B. Enforce Graceful Shutdown

To guarantee "at-least-once" processing, a pod must finish its accepted batch before exiting.

1. catch `SIGTERM`
2. stop fetching new tasks from the stream
3. complete processing for all active in-flight Goroutines
4. issue `XACK` to Redis for completed tasks, then exit

---

## Operator Reference Cheat Sheet

| Action / Metric | Strategy | Reason |
| --- | --- | --- |
| **Metrics Source** | `XINFO GROUPS` | captures both unread traffic and active in-flight load |
| **Sync Period** | eval loop every `10s` | balances responsiveness with API server overhead |
| **Scale Down Protection** | `MinReplicas: 1` | eliminates cold-start delays and system jitter |
| **Cluster Protection** | strict `MaxReplicas` cap | prevents runaway resource consumption during spikes |
| **Pod Exit Lifecycle** | `SIGTERM` $\rightarrow$ drain Pool $\rightarrow$ `XACK` $\rightarrow$ Exit | prevents orphaned tasks from getting trapped in the PEL |
