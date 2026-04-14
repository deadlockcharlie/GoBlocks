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

### This project is built as part of an intensive preparation program covering:
- Distributed systems 
- Site Reliability Engineering
- Database internals 
- Observability 

