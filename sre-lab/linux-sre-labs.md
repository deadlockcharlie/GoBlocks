# Linux Internals Lab Series — SRE Track

A progressive set of hands-on labs for entry-level SRE preparation.
All labs use standard off-the-shelf software on a Linux VM (Ubuntu/Debian).

---

## Module 1 — Processes & the `/proc` Filesystem

**Software:** `nginx`

1. Start nginx. Find its PID. Explore `/proc/<PID>/` — read `status`, `maps`, `limits`, `cmdline`, `environ`.
2. Count open file descriptors: `ls /proc/<PID>/fd | wc -l`. Understand what each symlink points to.
3. Send `SIGHUP` to nginx (config reload). Watch what happens to the PID tree with `pstree` before and after.
4. Use `strace -p <PID>` to watch nginx's syscalls while making an HTTP request. Identify `accept4`, `read`, `write`, `epoll_wait`.
5. Force a zombie process (small C program or shell trick). Find it with `ps`, understand why it exists, and how to reap it.

**Goal:** Be able to explain what the `D` process state means and why `kill -9` won't fix it.

---

## Module 2 — Networking

**Software:** `nginx`, `curl`, `redis`

1. Use `ss -tnp` while making HTTP requests to nginx. Identify `ESTABLISHED`, `TIME_WAIT`, `CLOSE_WAIT` states. Understand when each appears.
2. Lower `net.ipv4.tcp_fin_timeout` via `sysctl`. Observe how `TIME_WAIT` duration changes.
3. Use `tcpdump -i lo port 80` and capture a full HTTP request/response. Read the TCP handshake manually in the output.
4. Start redis. Use `ss` to confirm it's listening. Connect with `redis-cli` and simultaneously watch the socket state change.
5. Deliberately exhaust the connection backlog: set `net.core.somaxconn` very low, hammer nginx with `hey` or `ab`, and observe what happens in `dmesg` and `ss`.

**Goal:** Be able to explain why `CLOSE_WAIT` connections pile up and what application bug causes it.

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

## Module 4 — Storage & I/O

**Software:** `postgresql`

1. Start postgres. Write a query that does a sequential scan on a large table. Watch `iostat -x 1` — identify `await`, `%util`, `r/s`.
2. Use `iotop -o` to confirm postgres is the I/O source.
3. Drop the page cache (`echo 3 > /proc/sys/vm/drop_caches`) and re-run the query. Measure the difference.
4. Create a filesystem, fill it with files until `df` shows space but `touch` fails — inode exhaustion. Fix it.
5. Use `lsof +D /var/lib/postgresql` to see all file handles postgres has open on its data directory.

**Goal:** Be able to explain what dirty pages are and why a write-heavy workload can cause periodic latency spikes even with plenty of free RAM.

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

## Suggested Order

```
Module 1 → Module 2 → Module 3 → Module 4 → Module 6 → Module 5
```

Module 5 (eBPF) builds on everything before it — most powerful when you already know what you're looking for.

---

## Reference Material

- *The Linux Programming Interface* — Kerrisk. Use as a reference, not front-to-back reading.
- *Linux Observability with BPF* — Calavera & Fontana.
- Brendan Gregg's blog and USE Method: https://www.brendangregg.com/usemethod.html
