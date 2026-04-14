# GoBlocks - Distributed Block Store

A distributed block storage system built in Go with replication, inspired by concepts from GFS and Ceph.

## Features

- Fixed 4KB block storage for efficient and predictable data management
- Leaderless replication using consistent hashing for high availability and scalability
- Synchronous replication to ensure strong durability guarantees
- Zookeeper-based service discovery for dynamic membership and fault tolerance
- Clean, modular architecture for easy extensibility and maintenance

## Architecture Overview

```
GoBlockStore/
├── config/       # Configuration management
├── storage/      # Block storage layer
├── replication/  # Replication logic and Zookeeper integration
└── server/       # HTTP handlers and routing
```

## How It Works

- Each node registers itself with Zookeeper and discovers peers dynamically.
- Data is sharded and replicated across nodes using consistent hashing.
- All replication is synchronous for strong consistency.
- The system is leaderless: any node can serve reads/writes for blocks it owns.
- Nodes can join/leave at runtime; membership changes are handled automatically.

## Getting Started

1. Clone the repository
2. Start a Zookeeper instance (standalone or ensemble)
3. Build and run multiple GoBlockStore nodes with different configs
4. Use the HTTP API to store and retrieve blocks

## Example HTTP API

- `PUT /block/{id}`: Store a block
- `GET /block/{id}`: Retrieve a block
- `DELETE /block/{id}`: Delete a block
- `GET /health`: Health check endpoint

## Project Motivation

This project was built as part of an intensive preparation program covering:
- Distributed systems
- Site Reliability Engineering
- Database internals
- Observability

## License

MIT License
