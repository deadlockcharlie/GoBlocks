# Linux Internals Lab Series — SRE Track

A progressive set of hands-on labs for entry-level SRE preparation.
All labs use standard off-the-shelf software on a Linux VM (Ubuntu/Debian).

---

## Module 1 — Processes & the `/proc` Filesystem

**Software:** `nginx`

1. **Process Inspection via /proc:**
   - Start nginx: `systemctl start nginx` or `nginx`
   - Find PID: `pidof nginx` or `ps aux | grep nginx`
   - Explore `/proc/<PID>/status`: Note VmSize vs VmRSS difference
   - Check limits: `cat /proc/<PID>/limits` — See max open files, max processes
   - View environment: `cat /proc/<PID>/environ | tr '\0' '\n'`
   - Memory maps: `cat /proc/<PID>/maps` — Identify heap, stack, libraries

2. **File Descriptors:**
   - List FDs: `ls -l /proc/<PID>/fd`
   - Identify: 0→stdin, 1→stdout, 2→stderr (likely /dev/null for daemon)
   - Find sockets: `ls -l /proc/<PID>/fd | grep socket`
   - Count total: `ls /proc/<PID>/fd | wc -l`
   - Compare with: `lsof -p <PID> | wc -l`

3. **Graceful Reload (SIGHUP):**
   - Before: `pstree -p | grep nginx`
   - Send SIGHUP: `kill -HUP <master-PID>`
   - After: `pstree -p | grep nginx`
   - Observe: Master PID unchanged, worker PIDs changed
   - This is zero-downtime config reload

4. **Syscall Tracing:**
   - Attach strace: `strace -p <worker-PID>`
   - In another terminal: `curl http://localhost`
   - Observe: `epoll_wait()` → `recvfrom()` → `sendfile()` → `write()`
   - See zero-copy optimization with `sendfile()`

5. **Zombie Process:**
   - Create zombie:
   ```c
   // zombie.c
   #include <unistd.h>
   int main() { 
       if(fork() == 0) return 0; 
       sleep(60); 
   }
   ```
   - Compile: `gcc -o zombie zombie.c && ./zombie`
   - Find it: `ps aux | grep defunct` or `ps -eo pid,stat,cmd | grep Z`
   - Try kill: `kill -9 <zombie-PID>` (won't work!)
   - Kill parent instead: `kill <parent-PID>` (zombie disappears)

**Key Insight:** Understand D state (uninterruptible sleep) — process waiting on I/O that can't be interrupted. Usually means disk/storage problem. `kill -9` won't work because process is in kernel space waiting for I/O completion.

---

## Module 2 — Networking & Socket States

**Software:** `nginx`, `curl`, `redis`, `ab`

1. **Socket State Transitions:**
   - Start nginx: `systemctl start nginx`
   - Monitor sockets: `watch -n 0.5 'ss -tan | grep :80'`
   - In another terminal: `curl http://localhost`
   - Observe states: LISTEN → SYN-SENT → ESTABLISHED → TIME-WAIT
   - Count TIME_WAIT: `ss -tan | grep TIME-WAIT | wc -l`
   - These last 60 seconds (2×MSL) to prevent old packets in new connections

2. **Understanding tcp_fin_timeout (Common Misconception):**
   - Check current: `sysctl net.ipv4.tcp_fin_timeout`
   - Note: This cont Management

**Software:** `redis`, `stress-ng`

1. **Memory Layout Exploration:**
   - Start redis with data: `redis-server &`
   - Load data: `redis-cli set key1 "$(head -c 10K /dev/urandom | base64)"`
   - View layout: `pmap -x $(pidof redis-server)`
   - Identify regions:
     - Executable code (`r-x--`)
     - Heap (`[heap]` or large `rw---` anonymous regions)
     - Stack (`[stack]`)
     - Shared libraries (`libc.so`, etc.)
   - Note: Shared libs counted in VmSize but shared across processes
 Analysis

**Software:** `postgresql`, `fio`
To setup postgres, first set /etc/postgresql/<version>/main/pg_hba.conf to trust connections. This disables the need for a password and allows you to connect with `psql` directly. Then start postgres with `service postgresql start` or the appropriate command for your environment. This solution is not production-safe but is fine for labs. 

1. **I/O Metrics Understanding:**
   - Start postgres: `service postgresql start`
   - Create test table (from shell): `sudo -u postgres psql postgres -c "CREATE TABLE test AS SELECT generate_series(1,10000000) AS id;"`
   - OR from psql shell: Connect with `sudo -u postgres psql postgres`, then run:
     ```sql
     CREATE TABLE test AS SELECT generate_series(1,10000000) AS id;
     \dt
     ```
   - Monitor: `iostat -x 1`
   - Run sequential scan: `sudo -u postgres psql postgres -c "SELECT count(*) FROM test WHERE id > 0;"`
   - OR from psql shell: `SELECT count(*) FROM test WHERE id > 0;`
   - Key metrics:
     - `r/s`, `w/s`: Reads/writes per second (IOPS)
     - `await`: Average I/O latency (ms) — <10ms good for SSD, <20ms for HDD
     - `%util`: Percent of time disk busy — >80% means saturated
     - `avgqu-sz`: Average queue depth

2. **Process-Level I/O:**
   - Run query: `sudo -u postgres psql postgres -c "SELECT * FROM test" > /dev/null &`
   - Identify source: `iotop -o` (only show processes doing I/O)
   - See postgres reading blocks
   - Check cumulative I/O: `cat /proc/$(pgrep -o postgres)/io`

3. **Page Cache Impact:**
   - First run (cold cache): `time sudo -u postgres psql postgres -c "SELECT count(*) FROM test WHERE id > 0"`
   - Note time (slow, reading from disk)
   - Check cache: `free -h` (see large "buff/cache")
   - Second run (warm cache): `time sudo -u postgres psql postgres -c "SELECT count(*) FROM test WHERE id > 0"`
   - Much faster! Data served from RAM
   - Drop cache: `sudo sh -c 'echo 3 > /proc/sys/vm/drop_caches'`
   - Third run: Slow again (cache cleared)
   - Lesson: Page cache is critical for performance

4. **Inode Exhaustion:**
   - Create small filesystem: `dd if=/dev/zero of=/tmp/fs.img bs=1M count=100`
   - Format: `mkfs.ext4 /tmp/fs.img`
   - Mount: `mount -o loop /tmp/fs.img /mnt/test`
   - Check inodes: `df -i /mnt/test`
   - Fill with tiny files: `for i in {1..100000}; do touch /mnt/test/file$i; done`
   - Eventually fails: "No space left on device" (ran out of inodes, not disk space!)
   - Verify: `df -h /mnt/test` shows space, `df -i /mnt/test` shows 100% inodes
   - Fix: Recreate filesystem with more inodes or fewer small files

5. **Open File Handles:**
   - Find main PID: `pgrep -o postgres` (or `head -1 /var/lib/postgresql/data/postmaster.pid`)
   - List all files: `lsof +D /var/lib/postgresql | head -20`
   - See: Data files, WAL logs, temp files
   - Count: `lsof -p $(pgrep -o postgres) | wc -l`
   - Compare to limit: `cat /proc/$(pgrep -o postgres)/limits | grep "open files"`

**Dirty Pages & Latency Spikes:**
- Writes go to page cache (dirty pages) first
- Background writeback flushes to disk gradually
- If dirty pages accumulate faster than writeback can flush:
  - Eventually hit `vm.dirty_ratio` threshold
  - All writes block until flush completes
  - Causes sudeBPF-Based Observability

**Software:** `nginx`, `bpftrace`, `bcc-tools`, `perf`

1. **Live Profiling:**
   - Generate load: `ab -n 100000 -c 50 http://localhost/ &`
   - Profile: `perf top`
   - Identify hot functions in nginx (likely `ngx_http_*` functions)
   - See kernel functions too (`__tcp_transmit_skb`, `copy_user_enhanced_fast_string`)
   - This is sampling-based profiling (low overhead)

2. **Syscall Counting:**
   - Count writes: `bpftrace -e 'tracepoint:syscalls:sys_enter_write { @[comm] = count(); }'`
   - Let run for 30 seconds, Ctrl+C
   - See which processesContainer Resource Control

**Software:** `systemd-run`, `stress-ng`, `docker`

1. **Memory Limits (Cgroup OOM):**
   - Run with limit: `systemd-run --scope --property=MemoryMax=50M stress-ng --vm 1 --vm-bytes 200M`
   - Watch logs: `journalctl -f` or `dmesg -w`
   - See: "Memory cgroup out of memory: Killed process"
   - Key: Host is fine! Only the cgroup was killed
   - This is how Kubernetes enforces memory limits
   - Check surviving processes: Host processes unaffected

2. **CPU Throttling:**
   - Run cpu-bound: `systemd-run --scope --property=CPUQuota=20% stress-ng --cpu 1 --timeout 60s`
   - Monitor: `top` (should show ~20% CPU usage)
   - Check throttling stats:
     ```bash
     # Find cgroup path
     systemctl status run-*.scope
     # Read stats
     cat /sys/fs/cgroup/system.slice/run-*.scope/cpu.stat
     ```
   - Look for `nr_throttled` and `throttled_usec`
   - Process wants 100% but gets throttled to 20%

3. **Docker Container Cgroups:**
   - Run container: `docker run -d --name test nginx`
   - Find cgroup: `docker inspect test | grep -i cgroup`
   - Or explore: `find /sys/fs/cgroup -name "*docker*" -type d`
   - Check memory: `cat /sys/fs/cgroup/system.slice/docker-*.scope/memory.current`
   - Check CPU: `cat /sys/fs/cgroup/system.slice/docker-*.scope/cpu.stat`
   - This is how Docker/Kubernetes track resource usage

4. **CPU Quota Performance Impact:**
   - Run unthrottled: `stress-ng --cpu 1 --timeout 30s`
   - Measure: `perf stat stress-ng --cpu 1 --timeout 30s`
   - Note: `context-switches`, `cpu-migrations`
   - Run throttled: `systemd-run --scope --property=CPUQuota=10% stress-ng --cpu 1 --timeout 30s`
   - Measure: `perf stat` on the PID
   - See higher context switches (getting scheduled on/off more)

5. **Live Monitoring:**
   - Run: `systemd-cgtop`
   - See all cgroups with resource usage
   - Columns: Tasks, %CPU, Memory, I/O
   - Start container: `docker run -d stress-ng --cpu 2`
   - Watch it appear in systemd-cgtop

**Kubernetes Pod Limits Explained:**
```yaml
resources:
  limits:
    memory: "128Mi"  # MemoryMax in cgroup
    cpu: "500m"      # CPUQuota=50%
```
- Pod exceeds memory → cgroup OOM killer (only that pod dies)
- Pod exceeds CPU → throttled (slowed down, not killed)
- This is cgroup v2 on modern Kubernetes
- Check: `cat /proc/cgroups` to see if v1 or v2

**Critical SRE Concept:**
When pod is OOMKilled, it's not Linux OOM killer (host-level).
It's cgroup memory controller killing the process.
Host stays healthy, only container affected.
This is why container isolation works!
   - Measure: `biolatency-bpfcc 5 1` (5-second intervals)
   - See latency histogram in microseconds
   - Generate I/O: `dd if=/dev/sda of=/dev/null bs=4k count=10000`
   - See latency distribution change
   - This measures actual disk I/O, not page cache hits

5. **Flame Graphs:**
   - Record: `perf record -F 99 -a -g -- sleep 30` (while load runs)
   - Generate: `perf script | ./flamegraph.pl > flame.svg`
   - Open in browser: Widest bars = most CPU time
   - Click to zoom into call stacks
   - Identify bottlenecks visually

**How eBPF Works:**
- Small programs run in kernel space (sandboxed for safety)
- Attach to hook points: syscalls, function entry/exit, network events
- Kernel verifies program won't crash or loop infinitely
- Collect data via BPF maps (shared kernel-userspace memory)
- Zero application changes, minimal overhead (~1-5%)
- Production-safe observability

**Why It Matters for SRE:**
- Debug production without redeployment
- See what application actually does (not what it logs)
- Measure kernel-level metrics (I/O, network, scheduler)
- Modern observability tools (Datadog, Pixie) use eBPF
   - Check THP: `cat /sys/kernel/mm/transparent_hugepage/enabled`
   - Enable: `echo always > /sys/kernel/mm/transparent_hugepage/enabled`
   - Benchmark: `redis-benchmark -t set,get -n 1000000 -q`
   - Note p99 latency (higher due to defragmentation)
   - Disable: `echo never > /sys/kernel/mm/transparent_hugepage/enabled`
   - Re-benchmark: p99 should be lower
   - Trade-off: Better average performance vs latency spikes

5. **Swap Activity:**
   - Monitor: `vmstat 1`
   - Columns: `si` (swap in), `so` (swap out)
   - Trigger pressure: `stress-ng --vm 1 --vm-bytes 80%`
   - Watch `so` increase (pages moving to disk)
   - Then `si` increases (pages coming back)
   - Both non-zero = thrashing (severe performance issue)
   - Check swap usage: `free -h`

**Why Apps Get OOM-Killed Despite Low Heap:**
- Kernel counts RSS (total physical memory): heap + stacks + runtime + libraries + fragmentation
- Go: Goroutine stacks, runtime metadata, freed but not returned memory
- Java: Native memory, off-heap caches, JVM overhead
- App reports heap, kernel sees total RSS
- Trust kernel's view, not application metrics!
     - ACK (client→server): ack=Y+1
   - Then HTTP data follows

4. **Redis Connection Lifecycle:**
   - Start redis: `redis-server &`
   - Terminal 1: `watch -n 0.2 'ss -tnp | grep :6379'`
   - Terminal 2: `redis-cli` (observe ESTABLISHED)
   - Terminal 2: `quit` (observe TIME-WAIT on client side)
   - Server doesn't enter TIME_WAIT (passive close side)

5. **Connection Backlog Exhaustion:**
   - Set low backlog: `sysctl -w net.core.somaxconn=5`
   - Edit nginx config: Add `listen 80 backlog=5;` to server block
   - Reload: `nginx -s reload`
   - Hammer: `ab -n 10000 -c 200 http://localhost/`
   - Check logs: `dmesg | grep "possible SYN flooding"`
   - Watch SYN cookies kick in (connections still work!)
   - Restore: `sysctl -w net.core.somaxconn=4096`

**Critical Concept - CLOSE_WAIT:** 
- Remote side closed (sent FIN), local side ACKed
- But local application hasn't called `close()` yet
- Application bug: Leaked file descriptor or exception before close
- Cannot fix with sysctl — must fix application code
- If hundreds persist: You have a connection leak!

---

## Module 3 — Memory

**Software:** `redis`, `stress-ng`

1. Use `pmap -x <redis-PID>` to see its memory layout. Identify heap, stack, shared libraries.
2. Read `/proc/<PID>/smaps` and calculate the actual RSS vs virtual size difference. Understand why VSZ is misleading.
3. Use `stress-ng --vm 1 --vm-bytes 90%` to push the system toward OOM. Watch `/var/log/kern.log` or `dmesg` for the OOM killer. See which process it kills and why (check `/proc/<PID>/oom_score` beforehand).
4. Enable transparent huge pages, then disable them. Use `redis-benchmark` and compare latency variance between the two states.
5. Use `vmstat 1` during a memory-pressure event. Understand the `si`/`so` (swap in/out) columns.

**Goal:** Be able to explain why a Go or Java process can report low heap usage but still get OOM-killed.

---

## Module 4 — Storage & I/O Analysis

**Software:** `postgresql`, `fio`

**Starting PostgreSQL (choose based on your environment):**
- With systemd: `systemctl start postgresql`
- Without systemd (Docker/containers): 
  - Option 1: `su - postgres -c 'pg_ctl -D /var/lib/postgresql/data start'`
  - Option 2: `su - postgres -c '/usr/lib/postgresql/*/bin/postgres -D /var/lib/postgresql/data &'`
  - Option 3: `service postgresql start` (if available)
- Verify it's running: `ps aux | grep postgres` or `pg_isready`

1. **I/O Metrics Understanding:**
   - Start postgres (see above)
   - Connect: `sudo -u postgres psql postgres`
   - Create test table: `CREATE TABLE test AS SELECT generate_series(1,10000000) AS id;`
   - Monitor: `iostat -x 1`
   - Run sequential scan (from psql): `SELECT count(*) FROM test WHERE id > 0;`
   - OR from shell: `sudo -u postgres psql postgres -c "SELECT count(*) FROM test WHERE id > 0"`
   - Key metrics:
     - `r/s`, `w/s`: Reads/writes per second (IOPS)
     - `await`: Average I/O latency (ms) — <10ms good for SSD, <20ms for HDD
     - `%util`: Percent of time disk busy — >80% means saturated
     - `avgqu-sz`: Average queue depth

2. **Process-Level I/O:**
   - Run query: `sudo -u postgres psql postgres -c "SELECT * FROM test" > /dev/null &`
   - Identify source: `iotop -o` (only show processes doing I/O)
   - See postgres reading blocks
   - Check cumulative I/O: `cat /proc/$(pgrep -o postgres)/io`

3. **Page Cache Impact:**
   - First run (cold cache): `time sudo -u postgres psql postgres -c "SELECT count(*) FROM test WHERE id > 0"`
   - Note time (slow, reading from disk)
   - Check cache: `free -h` (see large "buff/cache")
   - Second run (warm cache): `time sudo -u postgres psql -c "SELECT count(*) FROM test WHERE id > 0"`
   - Much faster! Data served from RAM
   - Drop cache: `sudo sh -c 'echo 3 > /proc/sys/vm/drop_caches'`
   - Third run: Slow again (cache cleared)
   - Lesson: Page cache is critical for performance

4. **Inode Exhaustion:**
   - Create small filesystem: `dd if=/dev/zero of=/tmp/fs.img bs=1M count=100`
   - Format: `mkfs.ext4 /tmp/fs.img`
   - Mount: `mount -o loop /tmp/fs.img /mnt/test`
   - Check inodes: `df -i /mnt/test`
   - Fill with tiny files: `for i in {1..100000}; do touch /mnt/test/file$i; done`
   - Eventually fails: "No space left on device" (ran out of inodes, not disk space!)
   - Verify: `df -h /mnt/test` shows space, `df -i /mnt/test` shows 100% inodes
   - Fix: Recreate filesystem with more inodes or fewer small files

5. **Open File Handles:**
   - Find main PID: `pgrep -o postgres` (or `head -1 /var/lib/postgresql/data/postmaster.pid`)
   - List all files: `lsof +D /var/lib/postgresql | head -20`
   - See: Data files, WAL logs, temp files
   - Count: `lsof -p $(pgrep -o postgres) | wc -l`
   - Compare to limit: `cat /proc/$(pgrep -o postgres)/limits | grep "open files"`

**Dirty Pages & Latency Spikes:**
- Writes go to page cache (dirty pages) first
- Background writeback flushes to disk gradually
- If dirty pages accumulate faster than writeback can flush:
  - Eventually hit `vm.dirty_ratio` threshold
  - All writes block until flush completes
  - Causes sudden latency spike (100ms+ on spinning disk)
- Monitor: `watch -n 1 'grep -i dirty /proc/meminfo'`
- Tune: Lower `vm.dirty_ratio` for more consistent latency
- Trade-off: Lower ratio = more consistent but lower throughput

---

## Module 5 — Observability Without Application Logs

**Software:** `nginx`, `bpftrace` or `bcc` tools

1. Use `perf top` while nginx serves requests. Identify hot functions in the call stack.
2. Use `bpftrace -e 'tracepoint:syscalls:sys_enter_write { @[comm] = count(); }'` — count writes by process name system-wide.
3. Use `opensnoop` (from bcc) to watch every `open()` call nginx makes when serving a request.
4. Use `biolatency` (from bcc) to measure I/O latency distribution in real time without changing any application config.
5. Generate a CPU flamegraph for nginx using `perf record` + `flamegraph.pl`. Identify the hottest code path.

**Goal:** Be able to explain how eBPF lets you trace kernel and application behaviour without restarting or recompiling the target process.

---

## Module 6 — cgroups & Resource Control

**Software:** `systemd-run`, `stress-ng`, Docker

1. Use `systemd-run --scope --property=MemoryMax=50M stress-ng --vm 1 --vm-bytes 200M`. Watch the OOM kill happen inside the cgroup, not the host.
2. Use `systemd-run --scope --property=CPUQuota=20%` to throttle a process. Verify with `top` and `/sys/fs/cgroup/.../cpu.stat`.
3. Inspect `/sys/fs/cgroup/` hierarchy. Find a running Docker container's cgroup. Read its `memory.current` and `cpu.stat`.
4. Set `CPUQuota=10%` on a cpu-intensive process. Use `perf stat` to observe throttling (`cpu-migrations`, `context-switches`).
5. Use `systemd-cgtop` to view live resource usage per cgroup slice.

**Goal:** Be able to explain what happens when a Kubernetes pod hits its memory limit (and why it's a cgroup OOM, not the host OOM killer).

---

---

## Module 7 — Advanced Storage: I/O Schedulers & Filesystems

**Software:** `fio`, `postgresql`, `redis`

1. **I/O Scheduler Trade-offs:**
   - Check current scheduler: `cat /sys/block/sda/queue/scheduler`
   - Benchmark random reads with `mq-deadline`: `fio --name=randread --rw=randread --bs=4k --numjobs=4 --runtime=60`
   - Switch to `none`: `echo none > /sys/block/sda/queue/scheduler`
   - Repeat benchmark. Which is faster for SSD? Why?
   
2. **Direct I/O vs Buffered I/O:**
   - Run postgres with default buffered I/O. Measure throughput with `pgbench`.
   - Configure postgres for direct I/O (`wal_sync_method=open_datasync`). Retest.
   - Drop page cache between runs: `echo 3 > /proc/sys/vm/drop_caches`
   - Compare: When is direct I/O better? When is buffered better?

3. **Filesystem Comparison (ext4 vs XFS):**
   - Create ext4 filesystem: `mkfs.ext4 /dev/loop0`
   - Create XFS filesystem: `mkfs.xfs /dev/loop1`
   - Benchmark large file writes on both with `fio --name=seqwrite --rw=write --bs=1M --size=10G`
   - Benchmark many small files (simulate database workload)
   - Trade-off: Which filesystem for what workload?

4. **fsync Durability:**
   - Run redis with `appendfsync always` (fsync every write)
   - Measure latency with `redis-benchmark -t set -n 100000`
   - Change to `appendfsync everysec`. Retest.
   - Change to `appendfsync no`. Retest.
   - Trade-off: Durability vs performance. What happens on crash?

5. **Dirty Page Tuning:**
   - Set `vm.dirty_ratio=10` and `vm.dirty_background_ratio=5`
   - Run write-heavy workload while monitoring: `watch -n 1 'grep -i dirty /proc/meminfo'`
   - Increase to `vm.dirty_ratio=40` and `vm.dirty_background_ratio=10`
   - Observe write latency spikes with `iostat -x 1`
   - Trade-off: Large dirty ratio = better throughput but higher latency variance

**Goal:** Understand trade-offs between durability, performance, and consistency. Know when to tune what.

---

## Module 8 — Network Performance & Tuning

**Software:** `nginx`, `iperf3`, `netcat`

1. **TCP Window Scaling:**
   - Check current values: `sysctl net.ipv4.tcp_rmem` and `net.ipv4.tcp_wmem`
   - Test bandwidth: `iperf3 -s` (server), `iperf3 -c <server-ip>` (client)
   - Reduce buffers: `sysctl -w net.ipv4.tcp_rmem="4096 8192 16384"`
   - Retest. Observe throughput drop, especially over high-latency links.
   - Restore defaults. Trade-off: Memory vs throughput.

2. **Connection Pooling:**
   - Benchmark nginx without keep-alive: `ab -n 10000 -c 100 http://localhost/`
   - Count TIME_WAIT sockets: `ss -tan | grep TIME-WAIT | wc -l`
   - Enable keep-alive in nginx config: `keepalive_timeout 65;`
   - Retest. Observe fewer connections and better throughput.
   - Trade-off: Connection overhead vs resource holding.

3. **SYN Queue Tuning:**
   - Set low limits: `sysctl -w net.ipv4.tcp_max_syn_backlog=128`
   - Hammer with: `ab -n 50000 -c 500 http://localhost/`
   - Check for SYN drops: `netstat -s | grep -i "SYNs to LISTEN"`
   - Increase: `sysctl -w net.ipv4.tcp_max_syn_backlog=4096`
   - Retest. Trade-off: Memory vs handling burst traffic.

4. **TCP Fast Open:**
   - Enable: `sysctl -w net.ipv4.tcp_fastopen=3`
   - Benchmark first request latency (client sends data with SYN)
   - Compare with TFO disabled. When does it help?

5. **Monitoring Network Saturation:**
   - Generate traffic: `iperf3 -c <server> -P 10`
   - Monitor with: `sar -n DEV 1` or `nicstat 1`
   - Identify: `rxkB/s`, `txkB/s`, and saturation (%util equivalent)
   - Trade-off: Know your NIC limits before adding capacity.

**Goal:** Understand TCP tunables and when they matter. Recognize network bottlenecks vs application bottlenecks.

---

## Module 9 — Production Debugging Scenarios

**Software:** `nginx`, `postgresql`, `stress-ng`, `redis`

**Scenario 1: "The server is slow"**

Setup: Run postgres + nginx + redis simultaneously. Add load.

Task:
1. Identify bottleneck using USE method:
   - CPU: `mpstat 1`, `top`
   - Memory: `free -h`, `vmstat 1`
   - Disk: `iostat -x 1`
   - Network: `sar -n DEV 1`
2. Is it CPU-bound? Memory-bound? I/O-bound? Network-bound?
3. Which process is the culprit? Use `top`, `iotop`, `nethogs`.
4. What's the fix?

**Scenario 2: "Disk is 100% utilized but throughput is low"**

Setup: 
```bash
# Create I/O contention
fio --name=randread --rw=randread --bs=4k --iodepth=1 --numjobs=20
```

Task:
1. Check `iostat -x 1`: High `%util`, low `r/s` or `w/s`?
2. Check `await` — is it high? (Latency problem)
3. Check `avgqu-sz` — is queue depth reasonable?
4. Identify: Sequential vs random I/O problem?
5. Solution: Change I/O scheduler? Add SSD? Reduce queue depth?

**Scenario 3: "Application reports success but data is lost after crash"**

Setup: Run redis with `appendfsync no`. Write data. Kill process.

Task:
1. Restart redis. Is data there?
2. Check if writes were in page cache: `grep -i dirty /proc/meminfo`
3. Understand: What does `fsync` do? When does kernel flush?
4. Fix: Change `appendfsync` setting. Trade durability vs performance.

**Scenario 4: "Memory is full but no process is using it"**

Setup: Run workload that reads large files.

Task:
1. Check `free -h`: Is it in "buff/cache"?
2. Is this bad? No! Linux uses free memory for cache.
3. Drop caches: `echo 3 > /proc/sys/vm/drop_caches`
4. Did performance drop? Cache was helping!
5. Lesson: "Free" memory in Linux is misleading.

**Scenario 5: "Network latency spikes randomly"**

Setup: Run network traffic with `iperf3` while doing disk I/O.

Task:
1. Monitor latency: `ping -i 0.2 <target>`
2. Check for packet loss: `netstat -s | grep -i retrans`
3. Check CPU steal time (if VM): `top` (look for `%st`)
4. Check for NIC saturation: `sar -n DEV 1`
5. Identify: Is it network congestion? CPU contention? Disk I/O blocking network stack?

**Goal:** Develop systematic debugging process. Don't guess — measure, isolate, fix.

---

## Module 10 — High Availability & Replication

**Software:** `postgresql` with replication, `keepalived`, `haproxy`

1. **Primary-Backup Replication:**
   - Set up postgres primary-backup replication (streaming replication)
   - Monitor replication lag: `SELECT pg_last_wal_receive_lsn(), pg_last_wal_replay_lsn();`
   - Introduce lag by pausing replica: `SELECT pg_wal_replay_pause();`
   - Measure lag accumulation. Resume with `SELECT pg_wal_replay_resume();`
   - Trade-off: Sync vs async replication (consistency vs performance)

2. **Failover Testing:**
   - Run writes to postgres primary
   - Kill primary: `kill -9 <postgres-PID>`
   - Promote replica: `pg_ctl promote`
   - Measure downtime. What was lost?
   - Trade-off: Automatic failover (fast but risky) vs manual (slow but safe)

3. **Load Balancing with HAProxy:**
   - Set up HAProxy in front of postgres (read/write splitting)
   - Configure health checks: `option httpchk`
   - Kill one backend. Watch HAProxy detect and route around it.
   - Measure failover time. Trade-off: Health check frequency vs false positives.

4. **Split-Brain Prevention:**
   - Simulate network partition (use `iptables` to block traffic)
   - Both nodes think they're primary. What happens?
   - Implement fencing: Use `keepalived` with VRRP
   - Only one gets VIP. Trade-off: Availability vs data corruption risk.

5. **Quorum-Based Replication:**
   - Set up 3 postgres replicas with synchronous replication
   - Configure: `synchronous_standby_names = 'ANY 2 (replica1, replica2, replica3)'`
   - Kill one replica. Writes still succeed (quorum of 2/3).
   - Kill second replica. Writes block (can't reach quorum).
   - Trade-off: Durability guarantees vs availability.

**Goal:** Understand replication trade-offs. Know the CAP theorem in practice: consistency, availability, partition tolerance — pick two.

---

## Module 11 — Kernel Tunables & Performance

**Software:** `nginx`, `redis`, System utilities

1. **File Descriptor Limits:**
   - Check limits: `ulimit -n`
   - Set low: `ulimit -n 1024`
   - Start nginx, hammer with `ab -n 100000 -c 2000 http://localhost/`
   - Watch for "too many open files" errors
   - Increase: `ulimit -n 65536` or edit `/etc/security/limits.conf`
   - Trade-off: Resource limits prevent runaway processes vs limiting scale

2. **vm.swappiness Tuning:**
   - Default: `sysctl vm.swappiness` (usually 60)
   - Run memory-intensive workload: `redis-server` with large dataset
   - Set `vm.swappiness=10` (prefer to reclaim cache over swapping)
   - Trigger memory pressure with `stress-ng --vm 1 --vm-bytes 80%`
   - Watch swap usage: `vmstat 1`
   - Trade-off: Swapping vs OOM killer. What's worse?

3. **TCP Keepalive:**
   - Check defaults: `sysctl net.ipv4.tcp_keepalive_time`
   - Open connection: `nc -l 8888` (server), `nc localhost 8888` (client)
   - Disconnect network (if VM) or block with iptables
   - Time how long before kernel detects dead connection
   - Reduce keepalive: `sysctl -w net.ipv4.tcp_keepalive_time=60`
   - Trade-off: Fast detection vs extra network traffic

4. **Huge Pages:**
   - Check usage: `grep Huge /proc/meminfo`
   - Allocate: `sysctl -w vm.nr_hugepages=128`
   - Run database workload (postgres or redis)
   - Measure TLB misses: `perf stat -e dTLB-load-misses`
   - Trade-off: TLB efficiency vs memory fragmentation and allocation failures

5. **Interrupt Affinity:**
   - Check NIC interrupts: `cat /proc/interrupts | grep eth0`
   - See CPU distribution: Are all interrupts on CPU 0?
   - Balance interrupts: `irqbalance` or manual: `echo 2 > /proc/irq/<IRQ>/smp_affinity`
   - Run network benchmark: `iperf3 -c <server> -P 4`
   - Measure CPU usage per core: `mpstat -P ALL 1`
   - Trade-off: Interrupt distribution vs cache locality

**Goal:** Understand kernel tunables. Don't cargo-cult — measure before and after tuning.

---

## Module 12 — Container Performance & Isolation

**Software:** Docker, `systemd-run`

1. **CPU Throttling:**
   - Run CPU-intensive container: `docker run --cpus=0.5 ubuntu stress-ng --cpu 1 --timeout 60s`
   - Monitor throttling: `cat /sys/fs/cgroup/cpu,cpuacct/docker/<container-id>/cpu.stat`
   - Look for `nr_throttled` and `throttled_time`
   - Remove limit: `docker run --cpus=2` and compare
   - Trade-off: Fair sharing vs performance

2. **Memory Limits & OOM:**
   - Run: `docker run -m 100m ubuntu stress-ng --vm 1 --vm-bytes 200m`
   - Watch container get OOM killed (not host!)
   - Check: `docker inspect <container> | grep OOMKilled`
   - Increase limit: `docker run -m 500m` and retry
   - Trade-off: Resource isolation vs container restarts

3. **I/O Limits (blkio):**
   - Create I/O without limits: `docker run ubuntu fio --name=test --rw=write --bs=1M --size=1G`
   - Add limit: `docker run --device-write-bps /dev/sda:10mb ubuntu fio ...`
   - Measure throughput with `iostat -x 1`
   - Trade-off: I/O fairness vs noisy neighbor problem

4. **Network Namespaces:**
   - Create network namespace: `ip netns add test`
   - Run process in namespace: `ip netns exec test bash`
   - Try to access host network: `curl http://localhost` (fails!)
   - Create veth pair to connect namespaces
   - Understand: Container networking isolation

5. **PID Limits:**
   - Run fork bomb in container: `docker run --pids-limit=10 ubuntu bash -c ":(){ :|:& };:"`
   - Watch it get killed before affecting host
   - Remove limit and see host impact (don't actually do this!)
   - Trade-off: Process isolation vs resource overhead

**Goal:** Understand how Kubernetes resource limits actually work. Debug "why is my container getting killed?"

---

## Suggested Order

```
Module 1 → Module 2 → Module 3 → Module 4 → Module 7 → Module 6 → Module 8 → Module 9 → Module 5 → Module 10 → Module 11 → Module 12
```

**Progression:**
- Modules 1-4: Fundamentals (processes, network, memory, storage)
- Module 7: Advanced storage (builds on Module 4)
- Module 6: cgroups basics
- Module 8: Network performance (builds on Module 2)
- Module 9: Debugging practice (integrates all knowledge)
- Module 5: eBPF (after you know what to trace)
- Module 10-12: Production systems (HA, tuning, containers)

---

## Reference Material

- *The Linux Programming Interface* — Kerrisk. Use as a reference, not front-to-front reading.
- *Linux Observability with BPF* — Calavera & Fontana.
- *Systems Performance* — Brendan Gregg. The SRE bible for performance.
- Brendan Gregg's blog and USE Method: https://www.brendangregg.com/usemethod.html
- *Understanding the Linux Kernel* — Bovet & Cesati (for deep dives)

## Quick Command Reference

**Process Analysis:**
```bash
top, htop              # Live CPU/memory view
ps aux                 # Process list
pstree                 # Process tree
strace -p <PID>        # Syscall tracing
lsof -p <PID>          # Open files
/proc/<PID>/*          # Process details
```

**Network Analysis:**
```bash
ss -tanp               # Socket states
netstat -s             # Network statistics
tcpdump -i any         # Packet capture
sar -n DEV 1           # Network throughput
iftop                  # Live bandwidth by connection
```

**Memory Analysis:**
```bash
free -h                # Memory overview
vmstat 1               # VM statistics
pmap -x <PID>          # Process memory map
/proc/meminfo          # System memory details
slabtop                # Kernel slab cache
```

**Storage Analysis:**
```bash
iostat -x 1            # I/O statistics
iotop -o               # I/O by process
lsblk                  # Block devices
df -h                  # Disk usage
/proc/diskstats        # Detailed disk stats
```

**Performance Analysis:**
```bash
perf top               # Live profiling
perf record/report     # Profile and analyze
mpstat -P ALL 1        # Per-CPU statistics
pidstat 1              # Per-process stats
sar -A                 # All system activity
```
