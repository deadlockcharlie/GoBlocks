# GoBlocks - Distributed Block Store

A distributed block storage system built in Go with 3-way replication, implementing concepts from GFS and Ceph.

## Features

- Fixed 4KB block storage
- Primary-backup replication (3-way)
- Leader-follower architecture
- Synchronous replication for durability
- Clean modular architecture

## Architecture

```
├── config/       - Configuration management
├── storage/      - Block storage layer
├── replication/  - Replication client
└── server/       - HTTP handlers and routing
```

## Running Locally

### Single Node
```bash
go run main.go
```

### 3-Node Cluster

Terminal 1 (Primary):
```bash
ROLE=primary REPLICAS=localhost:8081,localhost:8082 go run main.go -port 8080
```

Terminal 2 (Replica 1):
```bash
ROLE=secondary go run main.go -port 8081
```

Terminal 3 (Replica 2):
```bash
ROLE=secondary go run main.go -port 8082
```

## API

### Write a block
```bash
dd if=/dev/zero bs=4096 count=1 | curl -X PUT \
  -H "Content-Type: application/octet-stream" \
  --data-binary @- \
  http://localhost:8080/block/test-1
```

### Read a block
```bash
curl http://localhost:8080/block/test-1
```

### Delete a block
```bash
curl -X DELETE http://localhost:8080/block/test-1
```

### Health check
```bash
curl http://localhost:8080/health
```
<!-- 
## Known Limitations

- No rollback mechanism for partial replication failures (will be replaced with Raft consensus)
- In-memory storage only (no persistence yet)
- Sequential replication (not parallel) -->

<!-- ## Roadmap

- [ ] Week 3: Add failure detection
- [ ] Week 4: Consistent hashing for block placement
- [ ] Week 5: Prometheus metrics + Grafana dashboards
- [ ] Week 6: OpenTelemetry tracing
- [ ] Week 7: Raft leader election and log replication
- [ ] Week 8: Production deployment on GKE

## Part of 8-Week SRE/Storage Engineer Preparation

This project is built as part of an intensive preparation program covering:
- Distributed systems (GFS, Ceph, Dynamo, Colossus)
- Site Reliability Engineering (SRE Book, SRE Workbook)
- Database internals (Kleppmann's DDIA)
- Observability (Prometheus, Grafana, OpenTelemetry)
- Kubernetes and GKE deployment -->
