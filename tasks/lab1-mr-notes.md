# Lab 1: MapReduce 完成记录

> 记录 6.5840 Spring 2026 Lab 1 的设计、实现与调试过程。实验说明见同目录 `lab1-mr.md`。

## 1. 目标

实现一个分布式 MapReduce：

- 一个 coordinator 进程，一个或多个 worker 进程
- worker 通过 RPC 向 coordinator 请求任务
- coordinator 分配 Map / Reduce 任务，并在 worker 超时（10 秒）后重新分配
- Map 把中间结果按 `nReduce` 分桶
- Reduce 把结果写到 `mr-out-X`

## 2. 系统结构

```text
Coordinator（RPC server）
  ├── 任务表：状态 + 分配时间 + 任务信息
  ├── FetchTask：分配任务 / 等待 / 退出
  ├── ReportTask：标记任务完成
  └── Done：全部 reduce 完成后退出

Worker A / B / C ...（进程级并发）
  └── 循环：请求任务 → 执行 → 报告完成
```

- 并发来自多个 worker 进程，worker 内部是串行的，不需要额外 `go func`
- coordinator 的 RPC handler 天然并发，必须用 mutex 保护共享状态

## 3. 协议设计（`src/mr/rpc.go`）

三类消息：

| 方向 | 结构 | 内容 |
| --- | --- | --- |
| Worker → Coordinator | `TaskArgs` | 空 |
| Coordinator → Worker | `TaskReply` | `TaskType` + `MapInfo` / `ReduceInfo` |
| Worker → Coordinator | `ReportArgs` | `TaskType` + `Id` |

- `TaskType`：`MapTask` / `ReduceTask` / `WaitTask` / `ExitTask`
- `MapInfo`：`Id`、`Filename`、`NReduce`
- `ReduceInfo`：`Id`、`NMap`

设计要点：

- 报告必须携带 `TaskType`。map 0 和 reduce 0 编号相同，reduce 阶段的迟到 map 报告需要靠类型区分阶段。
- 任务状态（idle / inProgress / completed）和分配时间是 coordinator 内部账本，不放进 RPC 结构，避免内部状态泄漏到通信协议。

## 4. Coordinator（`src/mr/coordinator.go`）

内部任务记录：`state` + `assignedAt` + 任务信息（map 存文件名，reduce 存 NMap）。

状态机：

| 事件 | 前置状态 | 后置状态 |
| --- | --- | --- |
| 分配任务 | idle | inProgress，记录 `assignedAt=now` |
| 完成报告 | 任意非 completed | completed |
| 完成报告 | completed | 不变（幂等忽略） |
| 超时扫描 | inProgress 且超时 | idle |

- **锁分层**：公开入口 `Done` / `FetchTask` / `ReportTask` 负责加锁；`doneLocked` / `mapDoneLocked` / `fetchMapLocked` / `fetchReduceLocked` / `expireTasksLocked` 假定调用者已持锁，不再加锁。`sync.Mutex` 不可重入，`FetchTask` 加锁后再调用加锁的 `Done` 会死锁。
- **惰性超时**：`FetchTask` 开头调用 `expireTasksLocked`，把 `InProgress` 且 `now - assignedAt > 10s` 的任务标回 `Idle`。worker 会持续请求，所以不需要后台 goroutine。
- **Done**：所有 reduce 任务均 `completed` 才返回 true。
- **无空闲任务时**：distinguish 阶段——仍有 reduce 在执行返回 `WaitTask`，全部完成才返回 `ExitTask`。"没有空闲任务"不等于"任务完成"。

## 5. Worker（`src/mr/worker.go`）

主循环：请求任务 → 按类型分发 → 报告完成；`WaitTask` sleep 1 秒后重试，`ExitTask` 退出，未知类型直接退出。

Map 流程：

1. `os.ReadFile` 读取输入文件
2. 调用 `mapf`
3. 按 `ihash(key) % NReduce` 分桶
4. 每个桶调用 `writeIntermediateFile` 写 `mr-X-Y`

Reduce 流程：

1. 按 `NMap` 读取 `mr-0-Y ... mr-(NMap-1)-Y`（JSON）
2. 按 key 排序，合并相同 key 的 values
3. 调用 `reducef`，写到 `mr-out-Y`

错误处理：`doMapTask` / `doReduceTask` 返回 `error`；出错只记录日志、**不发送 ReportTask**，任务由 coordinator 超时后重新分配。

## 6. 关键不变量

1. **原子提交**：写唯一临时文件 → `Close` → `Rename`。只有 rename 成功才算完成。
2. **完成报告幂等**：只把非 completed 的任务标为 completed，重复 / 迟到报告无害。
3. **超时重发安全**：每次执行用 `os.CreateTemp` 生成唯一临时文件，旧尝试不会破坏新尝试的输出。
4. **中间文件用 JSON**：避免 key/value 含空格时文本解析出错。

## 7. 开发过程与遇到的问题

1. 先运行并阅读 `mrsequential.go` / `mrapps/wc.go`，理解 Map → 排序 → Reduce 的数据流。
2. `rpc.go`：确定三类消息；把误放进协议里的任务状态字段删除，内部状态改到 coordinator。
3. `coordinator.go`：
   - 内部类型与 RPC 常量重名（`MapTask` / `ReduceTask`），改用 `mapTask` / `reduceTask`
   - 死锁：`FetchTask` 加锁后又调用加锁的 `Done`，拆成 `Locked` helper
   - data race：所有共享状态访问补锁（测试带 `-race`）
   - `ExitTask` 判据错误：没有 idle reduce 就退出，改为用全部完成判断
   - 缺少 `assignedAt`，无法超时重发，补齐时间戳和扫描
4. `worker.go`：
   - RPC 名称 / 常量不匹配（`AssignTask` vs `FetchTask`、`TaskTypeExit` vs `ExitTask`）
   - 临时文件用固定名，超时重试会互相覆盖，改用 `os.CreateTemp` 唯一命名
   - 中间文件由文本改为 JSON
   - `defer` 写在错误检查之前，打开失败时 nil panic，改为先检查再 defer
   - helper 返回临时名让调用者 rename，却被自己 defer 的 `os.Remove` 提前删除；改为 helper 接收最终文件名并自己完成 close + rename
   - 任务错误被静默吞掉，补日志

## 8. 验证

`cd src && make mr`，全部通过（带 `-race`）：

- `TestWc`、`TestIndexer`：正确性
- `TestMapParallel`、`TestReduceParallel`：并行
- `TestJobCount`、`TestEarlyExit`、`TestCrashWorker`：任务次数、提前退出、崩溃恢复

同时 `go build ./mr`、`go vet ./mr`、`gofmt -l mr/*.go` 均无问题。

## 9. 经验

- 先设计协议、再写实现；协议里只放通信需要的信息
- 明确"提交点"：输出要么完整可见，要么完全不可见
- 锁只在一层加，内部 helper 约定已持锁
- 用官方测试 + `-race` 作为回归手段
