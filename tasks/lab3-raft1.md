# 6.5840 Lab 3: Raft

**Spring 2026**

[Collaboration policy](http://nil.csail.mit.edu/6.5840/2026/labs/collab.html) //
[Submit lab](http://nil.csail.mit.edu/6.5840/2026/labs/submit.html) //
[Setup Go](http://nil.csail.mit.edu/6.5840/2026/labs/go.html) //
[Guidance](http://nil.csail.mit.edu/6.5840/2026/labs/guidance.html) //
[Piazza](https://piazza.com/mit/spring2026/65840)

## Introduction

This is the first in a series of labs in which you'll build a fault-tolerant key/value storage system. In this lab you'll implement Raft, a replicated state machine protocol. In the next lab you'll build a key/value service on top of Raft. Then you will "shard" your service over multiple replicated state machines for higher performance.

A replicated service achieves fault tolerance by storing complete copies of its state on multiple replica servers. Replication allows the service to continue operating even if some servers experience failures (crashes or a broken or flaky network). The challenge is that failures may cause the replicas to hold differing copies of the data.

Raft organizes client requests into a sequence, called the log, and ensures that all the replica servers see the same log. Each replica executes client requests in log order, applying them to its local copy of the service's state. Since all the live replicas see the same log contents, they all execute the same requests in the same order, and thus continue to have identical service state. If a server fails but later recovers, Raft takes care of bringing its log up to date. Raft will continue to operate as long as at least a majority of the servers are alive and can talk to each other. If there is no such majority, Raft will make no progress, but will pick up where it left off as soon as a majority can communicate again.

In this lab you'll implement Raft as a Go object type with associated methods, meant to be used as a module in a larger service. A set of Raft instances talk to each other with RPC to maintain replicated logs. Your Raft interface will support an indefinite sequence of numbered commands, also called log entries. The entries are numbered with index numbers. The log entry with a given index will eventually be committed. At that point, your Raft should send the log entry to the larger service for it to execute.

You should follow the design in the [extended Raft paper](http://nil.csail.mit.edu/6.5840/2026/papers/raft-extended.pdf), with particular attention to Figure 2. You'll implement most of what's in the paper, including saving persistent state and reading it after a node fails and then restarts. You will not implement cluster membership changes (Section 6).

This lab is due in four parts. You must submit each part on the corresponding due date.

## Getting Started

If you have done Lab 1, you already have a copy of the lab source code. If not, you can find directions for obtaining the source via git in the [Lab 1 instructions](lab-mr.html).

We supply you with skeleton code [src/raft1/raft.go](../src/raft1/raft.go). We also supply a set of tests, which you should use to drive your implementation efforts, and which we'll use to grade your submitted lab. The tests are in [src/raft1/raft_test.go](../src/raft1/raft_test.go).

When we grade your submissions, we will run the tests without the [`-race`](https://go.dev/blog/race-detector) flag. However, you should test with `-race`.

To get up and running, execute the following commands. Don't forget to `git pull` to get the latest software.

```bash
$ cd ~/6.5840
$ git pull
...
$ cd src
$ make raft1
go build -race -o main/raft1d main/raft1d.go
cd raft1 && go test -v -race
=== RUN   TestInitialElection3A
Test (3A): initial election (reliable network)...
Fatal: expected one leader, got none
        /Users/rtm/824-process-raft/src/raft1/test.go:151
        /Users/rtm/824-process-raft/src/raft1/raft_test.go:36
info: wrote visualization to /var/folders/x_/vk0xmxwn1sj91m89wsn5b1yh0000gr/T/porcupine-2242138501.html
--- FAIL: TestInitialElection3A (5.51s)
...
$
```

## The code

Implement Raft by adding code to `raft1/raft.go`. In that file you'll find skeleton code, plus examples of how to send and receive RPCs.

Your implementation must support the following interface, which the tester and (eventually) your key/value server will use. You'll find more details in comments in `raft.go` and in `raftapi/raftapi.go`.

```go
// create a new Raft server instance:
rf := Make(peers, me, persister, applyCh)

// start agreement on a new log entry:
rf.Start(command interface{}) (index, term, isleader)

// ask a Raft for its current term, and whether it thinks it is leader
rf.GetState() (term, isLeader)

// each time a new entry is committed to the log, each Raft peer
// should send an ApplyMsg to the service (or tester).
type ApplyMsg
```

A service calls `Make(peers,me,…)` to create a Raft peer. The `peers` argument is an array of network identifiers of the Raft peers (including this one), for use with RPC. The `me` argument is the index of this peer in the peers array. `Start(command)` asks Raft to start the processing to append the command to the replicated log. `Start()` should return immediately, without waiting for the log appends to complete. The service expects your implementation to send an `ApplyMsg` for each newly committed log entry to the `applyCh` channel argument to `Make()`.

`raft.go` contains example code that sends an RPC (`sendRequestVote()`) and that handles an incoming RPC (`RequestVote()`). Your Raft peers should exchange RPCs using the `labrpc` Go package (source in `src/labrpc`). The tester can tell `labrpc` to delay RPCs, re-order them, and discard them to simulate various network failures. While you can temporarily modify `labrpc`, make sure your Raft works with the original `labrpc`, since that's what we'll use to test and grade your lab. Your Raft instances must interact only through RPC; for example, they are not allowed to communicate using shared Go variables or files.

Subsequent labs build on this lab, so it is important to give yourself enough time to write solid code.

## Part 3A: leader election

Implement Raft leader election and heartbeats (`AppendEntries` RPCs with no log entries). The goal for Part 3A is for a single leader to be elected, for the leader to remain the leader if there are no failures, and for a new leader to take over if the old leader fails or if packets to/from the old leader are lost. Run `make RUN="-run 3A" raft1` in the `src` directory to test your 3A code.

### Hints

- Follow the paper's Figure 2. At this point you care about sending and receiving `RequestVote` RPCs, the Rules for Servers that relate to elections, and the State related to leader election.
- Add the Figure 2 state for leader election to the `Raft` struct in `raft.go`.
- Fill in the `RequestVoteArgs` and `RequestVoteReply` structs. Modify `Make()` to create a background goroutine that will kick off leader election periodically by sending out `RequestVote` RPCs when it hasn't heard from another peer for a while. Implement the `RequestVote()` RPC handler so that servers will vote for one another.
- To implement heartbeats, define an `AppendEntries` RPC struct (though you may not need all the arguments yet), and have the leader send them out periodically. Write an `AppendEntries` RPC handler method.
- The tester requires that the leader send heartbeat RPCs no more than ten times per second.
- The tester requires your Raft to elect a new leader within five seconds of the failure of the old leader if a majority of peers can still communicate.
- The paper's Section 5.2 mentions election timeouts in the range of 150 to 300 milliseconds. Such a range only makes sense if the leader sends heartbeats considerably more often than once per 150 milliseconds. Because the tester limits you to ten heartbeats per second, you will have to use an election timeout larger than the paper's 150 to 300 milliseconds, but not too large, because then you may fail to elect a leader within five seconds.
- You may find Go's [`rand`](https://golang.org/pkg/math/rand/) useful.
- The easiest way to perform periodic actions is a goroutine with a loop that calls `time.Sleep()`; see the `ticker()` goroutine that `Make()` creates. Don't use Go's `time.Timer` or `time.Ticker`, which are difficult to use correctly.
- If your code has trouble passing the tests, read the paper's Figure 2 again; the full logic for leader election is spread over multiple parts of the figure.
- Don't forget to implement `GetState()`.
- Go RPC sends only struct fields whose names start with capital letters. Sub-structures must also have capitalized field names. The `labgob` package will warn you about this; don't ignore the warnings.
- The most challenging part of this lab may be the debugging. Refer to the [Guidance](guidance.html) page for debugging tips.
- If you fail a test, the tester produces a file that visualizes a timeline with events marked along it, including network partitions, crashed servers, and checks performed. You can add annotations with `tester.Annotate("Server 0", "short description", "details")`.

Be sure you pass the 3A tests before submitting Part 3A.

```text
$ make RUN="-run 3A" raft1
go build -race -o main/raft1d main/raft1d.go
cd raft1 && go test -v -race -run 3A
=== RUN   TestInitialElection3A
Test (3A): initial election (reliable network)...
  ... Passed --  time  3.5s #peers 3 #RPCs    32 #Ops    0
--- PASS: TestInitialElection3A (3.84s)
=== RUN   TestReElection3A
Test (3A): election after network failure (reliable network)...
  ... Passed --  time  6.2s #peers 3 #RPCs    68 #Ops    0
--- PASS: TestReElection3A (6.54s)
=== RUN   TestManyElections3A
Test (3A): multiple elections (reliable network)...
  ... Passed --  time  9.8s #peers 7 #RPCs   684 #Ops    0
--- PASS: TestManyElections3A (10.68s)
PASS
ok      6.5840/raft1    22.095s
$
```

Each `Passed` line contains the test time, number of Raft peers, RPC count, total RPC bytes, and committed log entries. The grading script fails if all Lab 3, 4, and 5 tests take more than 600 seconds or an individual test takes more than 120 seconds. Test with `-race` even though grading omits it.

## Part 3B: log

Implement the leader and follower code to append new log entries, so that `make RUN="-run 3B" raft1` passes all tests.

### Hints

- Run `git pull` to get the latest lab software.
- The Raft paper views the log as 1-indexed, but a 0-indexed implementation with a dummy entry at index 0 and term 0 is suggested.
- First pass `TestBasicAgree3B()`. Start with `Start()`, then send and receive new log entries via `AppendEntries`, following Figure 2. Send each newly committed entry on `applyCh` on every peer.
- Implement the election restriction from Section 5.4.1.
- Loops that repeatedly check events must pause; use condition variables or `time.Sleep(10 * time.Millisecond)`.
- Write clean, clear code for future labs.
- If a test fails, read `raft_test.go` and trace what it checks.

Run:

```bash
$ make RUN="-run 3B" raft1
```

The 3B tests include basic agreement, RPC byte counts, follower and leader failure, reconnect and rejoin, backup over incorrect logs, concurrent starts, and RPC count limits. The implementation should not spend excessive time sleeping, waiting for RPC timeouts, or sending unnecessary RPCs.

## Part 3C: persistence

If a Raft-based server reboots it should resume service where it left off. Persistent state must survive a reboot. The paper's Figure 2 identifies the state that must be persistent.

The lab uses a `Persister` object rather than disk. `Make()` receives the most recently persisted state, and Raft should initialize from `ReadRaftState()` and save changed persistent state with `Save()`.

Complete `persist()` and `readPersist()` in `raft.go`. Serialize state with `labgob`, and pass `nil` as the snapshot argument to `Save()` until snapshots are implemented. Insert calls to `persist()` wherever persistent state changes.

You will probably need the optimization that backs up `nextIndex` by more than one entry at a time. One possible rejection reply includes:

```text
XTerm:  term in the conflicting entry (if any)
XIndex: index of first entry with that term (if any)
XLen:   log length
```

Then the leader can use:

```text
Case 1: leader doesn't have XTerm:
  nextIndex = XIndex
Case 2: leader has XTerm:
  nextIndex = (index of leader's last entry for XTerm) + 1
Case 3: follower's log is too short:
  nextIndex = XLen
```

Run `git pull` for updates. The 3C tests are more demanding than 3A and 3B, and failures may originate in earlier parts. Run the tests multiple times before submitting.

```bash
$ make RUN="-run 3C" raft1
```

The 3C tests cover basic and extended persistence, partitioned leaders and restarts, Figure 8, unreliable agreement, and reliable/unreliable churn. Your code must pass 3A, 3B, and 3C as well.

## Part 3D: log compaction

Without compaction, a rebooting server replays the complete Raft log. You will modify Raft to cooperate with services that persistently store snapshots, after which Raft discards entries before the snapshot. A lagging follower may then need a snapshot plus the remaining log to catch up.

Raft must provide:

```go
Snapshot(index int, snapshot []byte)
```

The tester calls `Snapshot()` periodically in Lab 3. In Lab 4, the key/value service will call it with a serialized snapshot of its table. The service calls `Snapshot()` on every peer, not only the leader.

The `index` argument is the highest log entry reflected in the snapshot. Raft should discard log entries through that point and operate with a trimmed log. Implement the `InstallSnapshot` RPC so a leader can send a snapshot to a lagging follower.

When a follower receives `InstallSnapshot`, it can send the snapshot to the service through `applyCh` in an `ApplyMsg`. Snapshots must only advance the service state, never move it backwards.

If a server crashes, persist both Raft state and the corresponding snapshot using the second argument to `persister.Save()`. If there is no snapshot, pass `nil`.

Implement `Snapshot()`, `InstallSnapshot`, and the required trimmed-log changes. The solution is complete when all 3D tests and all previous Lab 3 tests pass.

### Hints

- Run `git pull` for updates.
- First store only the part of the log starting at some index X. Initially set X to zero and pass the 3B/3C tests. Then make `Snapshot(index)` discard entries before `index` and set X to `index`.
- A common reason for failing the first 3D test is that followers take too long to catch up.
- Have the leader send `InstallSnapshot` when it no longer has the entries needed to bring a follower up to date.
- Send the entire snapshot in one RPC; do not implement Figure 13's offset mechanism.
- Discard old log entries so the Go garbage collector can free them; no reachable references may remain.
- On restart, `Persister` contains both Raft state and the application snapshot. Raft must include a non-nil snapshot with every `Save()` after log trimming, so `Make()` should read the persisted snapshot and preserve it.
- A reasonable full Lab 3 runtime without `-race` is about six minutes of wall-clock time and one minute of CPU time. With `-race`, about ten minutes of wall-clock time and two minutes of CPU time.

```bash
$ make RUN="-run 3D" raft1
```

The 3D tests cover basic snapshots, snapshot installation after disconnect, unreliable networks, crashes and restarts, all-server crashes, and snapshot initialization after crash. Your code must pass 3A, 3B, and 3C as well.

```text
PASS
ok      6.5840/raft1    301.406s
```
