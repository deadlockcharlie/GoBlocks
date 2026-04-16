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

### **Routing & Coordination**
- Smart request routing based on consistent hashing
- Any node can act as coordinator for any request
- Automatic forwarding to responsible nodes
- Fault-tolerant reads with replica fallback

## Architecture Overview

```
GoBlockStore/
├── config/           # Configuration management
├── storage/          # In-memory block storage layer
├── replication/      # Core distributed systems logic
│   ├── node.go       # Zookeeper integration & service discovery
│   ├── ring.go       # Consistent hash ring with vnodes
│   ├── vnode.go      # Virtual node implementation
│   └── client.go     # Replication client for inter-node communication
└── server/           # HTTP API handlers and routing
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

### **🚧 Phase 1: Core Rebalancing & Observability**
- [ ] **Ring Rebalancing & Handoff**: Automatic data migration when nodes join/leave
- [ ] **OpenTelemetry Integration**: Distributed tracing for request flows
- [ ] **Prometheus Metrics**: Request rates, latencies, error rates, storage usage
- [ ] **Grafana Dashboards**: Real-time visualization of cluster health
- [ ] **SLO Tracking**: Availability SLOs with error budget monitoring

### **📋 Phase 2: Reliability & Consistency**
- [ ] **Read Repair**: Detect and fix inconsistencies during reads
- [ ] **Hinted Handoff**: Temporary storage for failed writes
- [ ] **Anti-entropy (Merkle Trees)**: Background consistency checks
- [ ] **Configurable Consistency Levels**: Quorum reads/writes (R+W > N)
- [ ] **Sloppy Quorums**: Handle temporary failures gracefully

### **🏗️ Phase 3: Production Hardening**
- [ ] **Persistent Storage**: Replace in-memory storage with disk-backed solution (RocksDB/BadgerDB)
- [ ] **Write-Ahead Log (WAL)**: Durability for in-flight operations
- [ ] **Compression**: Block-level compression for storage efficiency
- [ ] **Security**: TLS for inter-node communication, Zookeeper ACLs
- [ ] **Admin API**: Cluster status, ring visualization, rebalancing controls
- [ ] **Rate Limiting**: Per-node and per-client rate limiting

### **🌍 Phase 4: Multi-Datacenter Replication**
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
- **Zookeeper Integration**: Production-grade service discovery
- **Coordinator Pattern**: Any-node request handling
- **Consistent Hashing**: Minimal data movement on topology changes

## Contributing

This is a learning project, but feedback and suggestions are welcome!

## License

MIT License

