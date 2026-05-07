# GoBlocks - Distributed Block Store

A distributed block storage system built in Go with configurable replication, implementing consistent hashing with virtual nodes (vnodes) for optimal data distribution. Inspired by Amazon Dynamo, Apache Cassandra, and Ceph.

## Features

### **Storage & Replication**
- Fixed 4KB block storage for efficient and predictable data management
- Consistent hashing with virtual nodes (vnodes) for balanced data distribution
- Configurable replication factor (default: 3-way replication)
- Leaderless architecture: any node can coordinate reads/writes
- Synchronous replication for strong consistency guarantees

### **Service Discovery & Fault Tolerance**
- Zookeeper-based service discovery for dynamic cluster membership
- Automatic detection of node joins and leaves
- Real-time hash ring updates on topology changes

### **Ring Rebalancing & Migration** 
- Automatic data migration when nodes join or leave the cluster
- ZooKeeper-coordinated two-phase migration protocol
- Zero-downtime rebalancing with graceful handoff
- Stale block management with configurable grace period
- Background garbage collection after migration completes
- Deferred migration for rapid topology changes during active migration

### **Routing & Coordination**
- Smart request routing based on consistent hashing
- Any node can act as coordinator for any request
- Automatic forwarding to responsible nodes
- Fault-tolerant reads with replica fallback

## Architecture Overview

```
GoBlockStore/
├── config/           # Configuration management
├── storage/          # In-memory block storage layer with stale block tracking
├── replication/      # Core distributed systems logic
│   ├── node.go       # Zookeeper integration, service discovery & migration coordination
│   ├── ring.go       # Consistent hash ring with vnodes & migration state
│   ├── vnode.go      # Virtual node implementation
│   └── client.go     # Replication client for inter-node communication
├── server/           # HTTP API handlers and routing
└── k8s/              # Kubernetes deployment manifests
    ├── ns.yaml
    ├── zookeeper/    # ZooKeeper deployment
    └── blockstore/   # StatefulSet, Services, ConfigMap
```

## How It Works

### **1. Service Discovery**
- Each node registers itself with Zookeeper using ephemeral sequential nodes
- Nodes watch for membership changes and maintain an up-to-date replica list
- Logical node names are stored in Zookeeper data to handle ephemeral node naming

### **2. Consistent Hashing with VNodes**
- Each physical node is assigned 20 virtual nodes (vnodes) on the hash ring
- Block placement is determined by hashing the block ID and finding the next vnodes on the ring
- Vnodes ensure balanced distribution even with few physical nodes
- Replication factor determines how many unique physical nodes store each block

### **3. Request Coordination**
- External clients can send requests to any node (coordinator pattern)
- The coordinator checks the hash ring to find responsible nodes
- If the coordinator is responsible, it serves the request locally
- Otherwise, it forwards the request to the appropriate node(s)

### **4. Data Placement**
- Blocks are distributed across nodes based on consistent hashing
- Each block is replicated to R nodes (where R = replication factor)
- The hash ring ensures minimal data movement when nodes join/leave

### **5. Ring Rebalancing**

When nodes join or leave the cluster, GoBlockStore automatically rebalances data using a coordinated migration protocol:

#### **Migration Protocol**

**Phase 1: Detection**
- Nodes detect topology changes via ZooKeeper watches
- First node to detect creates migration barrier in ZooKeeper
- All existing nodes register as participants
- New/departing nodes trigger ring recalculation

**Phase 2: Data Transfer**
- Each node independently calculates which blocks need migration
- Blocks are transferred to new owners (nodes that didn't have them before)
- Blocks no longer owned by current node are marked as **stale**
- Stale blocks remain readable during grace period (zero-downtime)

**Phase 3: Coordination**
- Nodes mark themselves "ready" in ZooKeeper after completing transfers
- Any node can act as coordinator to check if all participants are ready
- When all ready, coordinator deletes the migration barrier

**Phase 4: Atomic Ring Swap**
- All nodes watch for barrier deletion
- Upon deletion, all nodes atomically swap to the new ring topology
- During migration, routing uses OLD ring for stability
- After swap, routing uses NEW ring

**Phase 5: Garbage Collection**
- After configurable grace period (default: 5 minutes)
- Stale blocks are garbage collected in background
- Memory/disk footprint reduced

#### **Handling Rapid Topology Changes**

If nodes join/leave during active migration:
- Topology changes are **deferred** using a `PendingMigration` flag
- Ring watches are temporarily disabled during migration
- When current migration completes, pending migration starts automatically
- Multiple topology changes are batched into single migration cycle

This prevents migration storms and ensures cluster stability.

## Deployment

### **Kubernetes Deployment** (Recommended)

GoBlockStore is designed to run on Kubernetes using StatefulSets.

#### **Quick Start**

```bash
# 1. Start Minikube (or use existing cluster)
minikube start

# 2. Build image in Minikube's Docker
eval $(minikube docker-env)
docker build -t goblocks:latest .

# 3. Deploy namespace, ZooKeeper, and BlockStore
kubectl apply -f k8s/ns.yaml
kubectl apply -f k8s/zookeeper/deploy.yaml
kubectl apply -f k8s/zookeeper/service.yaml
kubectl apply -f k8s/blockstore/configmap.yaml
kubectl apply -f k8s/blockstore/service-headless.yaml
kubectl apply -f k8s/blockstore/statefulset.yaml
kubectl apply -f k8s/blockstore/service-external.yaml

# 4. Wait for pods to be ready
kubectl get pods -n goblocks -w

# 5. Access the service
minikube service blockstore -n goblocks --url
```

#### **Why StatefulSet?**

- **Stable pod names**: `blockstore-0`, `blockstore-1`, `blockstore-2`
- **Stable network identities**: Each pod gets DNS entry via headless service
- **Ordered deployment**: Pods created sequentially (important for ring formation)
- **ZooKeeper registration**: Each pod registers with unique, predictable name

#### **Testing Ring Rebalancing**

```bash
# Get service URL
SERVICE_URL=$(minikube service blockstore -n goblocks --url)

# Write test data
for i in {1..20}; do
  dd if=/dev/urandom bs=4096 count=1 2>/dev/null | \
  curl -X PUT "$SERVICE_URL/block/test-$i" --data-binary @-
done

# Scale up to trigger rebalancing
kubectl scale statefulset blockstore --replicas=4 -n goblocks

# Watch migration logs
kubectl logs -f blockstore-0 -n goblocks | grep -E "Migration|transfer|barrier|swap"

# Verify data still accessible
for i in {1..20}; do
  curl "$SERVICE_URL/block/test-$i" > /dev/null && echo "✅ test-$i" || echo "❌ test-$i"
done

# Scale down
kubectl scale statefulset blockstore --replicas=3 -n goblocks
```

#### **Key Kubernetes Concepts Used**

1. **StatefulSet**: Stable identities for replicated blockstore nodes
2. **Headless Service**: Pod-to-pod DNS for ring communication
3. **NodePort Service**: External access for clients
4. **ConfigMap**: ZooKeeper connection configuration
5. **Namespaces**: Isolated deployment environment

#### **Monitoring Migration**

```bash
# Watch ZooKeeper migration state
kubectl exec -it deploy/zookeeper -n goblocks -- zkCli.sh
# Then: ls /migration/participants

# Check migration logs across all nodes
kubectl logs -l app=blockstore -n goblocks --tail=50 | grep Migration

# Monitor garbage collection
kubectl logs -l app=blockstore -n goblocks | grep "Garbage collection"

# Check for errors
kubectl logs -l app=blockstore -n goblocks | grep -i error
```

#### **Troubleshooting**

**Migration stuck?**
```bash
# Check if all participants are registered
kubectl exec -it deploy/zookeeper -n goblocks -- zkCli.sh
ls /migration/participants
get /migration/participants/blockstore-0

# Check for network issues
kubectl exec -it blockstore-0 -n goblocks -- sh
wget -O- http://blockstore-1.blockstore-headless.goblocks.svc.cluster.local:3000/health
```

**Data inconsistency after migration?**
```bash
# Check which nodes think they own a block
for pod in blockstore-{0,1,2,3}; do
  echo "=== $pod ==="
  kubectl exec $pod -n goblocks -- wget -qO- http://localhost:3000/block/test-1 && echo "HAS BLOCK" || echo "NO BLOCK"
done
```

## Getting Started

### **Prerequisites**
- Go 1.21 or higher
- Zookeeper 3.x (standalone or ensemble)

### **Setup**

1. **Start Zookeeper**
```bash
# Using Docker
docker run -d -p 2181:2181 --name zookeeper zookeeper:3.8
```

2. **Build the project**
```bash
go build -o blockstore
```

3. **Run multiple nodes**
```bash
# Node 1
export ZKAddress="localhost:2181"
export ReplicaName="node1"
export ReplicaAddress="localhost"
export ReplicationFactor=3
./blockstore -port 3001

# Node 2
export ReplicaName="node2"
./blockstore -port 3002

# Node 3
export ReplicaName="node3"
./blockstore -port 3003
```

### **Environment Variables**
- `ZKAddress`: Zookeeper server address (default: localhost:2181)
- `ReplicaName`: Unique name for this node (required)
- `ReplicaAddress`: Address where this node is reachable (required)
- `ReplicationFactor`: Number of replicas per block (default: 3)

## HTTP API

### **Block Operations**
- `PUT /block/{id}`: Store a 4KB block (automatically routed to responsible nodes)
- `GET /block/{id}`: Retrieve a block (fetched from responsible nodes)
- `DELETE /block/{id}`: Delete a block
- `GET /health`: Health check endpoint

### **Internal Endpoints** (inter-node communication)
- `PUT /internal/block/{id}`: Direct replication endpoint
- `DELETE /internal/block/{id}`: Direct deletion endpoint

### **Example Usage**
```bash
# Store a block
dd if=/dev/urandom of=block.bin bs=4096 count=1
curl -X PUT --data-binary @block.bin http://localhost:3001/block/myblock

# Retrieve a block
curl http://localhost:3002/block/myblock -o retrieved.bin

# Delete a block
curl -X DELETE http://localhost:3003/block/myblock
```

## Roadmap

### **✅ Phase 1: Core Rebalancing & Foundation** (Completed)
- [x] **Ring Rebalancing**: Automatic data migration when nodes join/leave
- [x] **ZooKeeper Coordination**: Barrier-based migration protocol
- [x] **Graceful Migration**: Zero-downtime rebalancing with stale block management
- [x] **Garbage Collection**: Background cleanup after grace period
- [x] **Kubernetes Deployment**: StatefulSet-based deployment with stable identities

### **🚧 Phase 2: Observability & Monitoring**
- [ ] **OpenTelemetry Integration**: Distributed tracing for request flows
- [ ] **Prometheus Metrics**: Request rates, latencies, error rates, storage usage, migration metrics
- [ ] **Grafana Dashboards**: Real-time visualization of cluster health and migration status
- [ ] **SLO Tracking**: Availability SLOs with error budget monitoring
- [ ] **Migration Observability**: Track data transfer progress, participant status

### **📋 Phase 3: Reliability & Consistency**
- [ ] **Hinted Handoff**: Temporary storage for failed writes with pull-based delivery
- [ ] **Read Repair**: Detect and fix inconsistencies during reads
- [ ] **Anti-entropy (Merkle Trees)**: Background consistency checks
- [ ] **Configurable Consistency Levels**: Quorum reads/writes (R+W > N)
- [ ] **Sloppy Quorums**: Handle temporary failures gracefully

### **🏗️ Phase 4: Production Hardening**
- [ ] **Persistent Storage**: Replace in-memory storage with disk-backed solution (RocksDB/BadgerDB)
- [ ] **Write-Ahead Log (WAL)**: Durability for in-flight operations
- [ ] **Compression**: Block-level compression for storage efficiency
- [ ] **Security**: TLS for inter-node communication, Zookeeper ACLs
- [ ] **Admin API**: Cluster status, ring visualization, rebalancing controls
- [ ] **Rate Limiting**: Per-node and per-client rate limiting

### **🌍 Phase 5: Multi-Datacenter Replication**
- [ ] **Log-Based Replication**: Asynchronous replication across datacenters
- [ ] **Rack-Aware Placement**: Topology-aware replica placement
- [ ] **Cross-DC Consistency**: Vector clocks or last-write-wins (LWW)
- [ ] **Datacenter Failover**: Automatic failover between datacenters
- [ ] **Geo-replication Lag Monitoring**: Track replication delays

## Project Motivation

This project is built as part of an intensive learning program covering:
- **Distributed Systems**: Consensus, replication, partitioning, fault tolerance
- **Site Reliability Engineering**: Observability, SLOs, error budgets, incident response
- **Database Internals**: Storage engines, indexing, consistency models
- **Production Systems**: Monitoring, alerting, capacity planning

## Technical Highlights

- **Virtual Nodes (VNodes)**: Better load distribution and faster rebalancing
- **ZooKeeper Integration**: Production-grade service discovery and migration coordination
- **Coordinator Pattern**: Any-node request handling
- **Consistent Hashing**: Minimal data movement on topology changes
- **Zero-Downtime Rebalancing**: Graceful migration with stale block management
- **Barrier Synchronization**: ZooKeeper-coordinated atomic ring swaps
- **Deferred Migration**: Batches rapid topology changes to prevent migration storms
- **Kubernetes-Native**: StatefulSet-based deployment with stable pod identities

## Contributing

This is a learning project, but feedback and suggestions are welcome!

## License

MIT License

