# Lab 2: Key/Value Server 完成记录

> 记录 6.5840 Spring 2026 Lab 2 的设计、实现与调试过程。实验说明见同目录 `lab2-kvsrv1.md`。

## 1. 目标

实现一个支持版本控制的单机 Key/Value Server，并基于它构造分布式锁：

- `Get` 读取 key 的值和版本
- `Put` 只有在版本匹配时才更新
- Clerk 在 RPC 丢失时重试
- 重试后的版本冲突返回 `ErrMaybe`
- Lock 使用条件 Put 实现互斥

## 2. 系统结构

```text
Client / Lock
  └── Clerk：Get / Put、RPC 重试、ErrMaybe
        └── tester.Clnt / RPC
              └── KVServer：data + mutex
```

- KVServer 保存共享状态，`mu` 保护 map
- Clerk 是客户端通信封装，不保存服务器 KV 数据
- Lock 把 KV 原语组合成 `Acquire` / `Release`

## 3. KV 状态与版本语义（`src/kvsrv1/server.go`）

服务器使用以下状态：

```text
data[key] = Entry{Value, Version}
```

Put 状态转移：

| 当前状态 | 请求版本 | 结果 |
| --- | --- | --- |
| key 不存在 | `0` | 创建，版本为 `1`，返回 `OK` |
| key 不存在 | 非 `0` | 不修改，返回 `ErrNoKey` |
| key 存在 | 等于当前版本 | 更新并将版本加一，返回 `OK` |
| key 存在 | 不等于当前版本 | 不修改，返回 `ErrVersion` |

设计要点：

- map 查询必须使用存在标志，区分不存在 key 和空字符串 value
- 版本检查与更新必须在同一个 mutex 临界区内完成
- 成功 Put 的线性化点是服务器完成 map 更新的时刻

## 4. Clerk（`src/kvsrv1/client.go`）

Get 流程：

1. 构造 `GetArgs`
2. 调用 `KVServer.Get`
3. 没有收到回复就重试
4. 收到回复后返回服务器的 value、version 和 error

Put 流程：

1. 使用相同的 key、value、version 重发逻辑请求
2. `Call` 返回 `false` 时等待后重试，并记录已经发生过重试
3. 收到 `OK` 或 `ErrNoKey` 时直接返回
4. 初次收到 `ErrVersion` 返回 `ErrVersion`
5. 重试后收到 `ErrVersion` 返回 `ErrMaybe`

`ErrMaybe` 表示客户端无法判断之前的 Put 是否已经执行，不能把重试后的版本冲突简单解释为失败。

## 5. 分布式锁（`src/kvsrv1/lock/lock.go`）

Lock 保存：

```text
ck       -> KV Clerk
lockname -> 锁对应的 key
clientID -> 当前客户端标识
```

锁状态约定：

```text
key 不存在              -> 初始空闲
key 存在且 value == ""  -> 已释放、空闲
key 存在且 value != ""  -> value 是持有者的 clientID
```

使用 `kvtest.RandValue(8)` 在创建 Lock 时生成 clientID，并在其生命周期内保持不变。

## 6. Acquire / Release 协议

Acquire：

1. `Get(lockname)` 读取当前值和版本
2. key 不存在时使用版本 `0` 尝试创建
3. value 为空时使用当前版本尝试 Put 自己的 clientID
4. value 是其他客户端 ID 时等待后重试
5. `Put` 返回 `ErrVersion` 或 `ErrMaybe` 后重新读取状态
6. 重新读取到自己的 clientID 时认为自己已经持有锁

Release：

1. 读取当前值和版本
2. key 不存在或持有者不是自己时直接返回
3. 只有 value 等于自己的 clientID 时，才使用当前版本 Put 空字符串
4. `ErrVersion` 或 `ErrMaybe` 后重新读取，不能覆盖新状态

两个客户端若基于同一版本同时 Put，服务器只允许一个成功；成功的条件 Put 就是获取锁的竞争线性化点。

## 7. 关键不变量

1. 每个存在的 KV key 同时拥有 value 和有效 version
2. 失败的 Put 不修改服务器状态
3. 条件 Put 的检查和更新不可分离
4. Lock 不能使用 Put 覆盖其他 clientID
5. Release 只能释放当前客户端持有的锁
6. `ErrMaybe` 必须通过后续 Get 重新确认状态
7. 当前实现允许重复 Acquire 直接返回，但不维护重入深度；一次 Release 仍可能释放锁

## 8. 开发过程与遇到的问题

1. 先实现 `KVServer.data` 和 `MakeKVServer`，确保 map 已初始化。
2. `Get` 使用 map 的存在标志，避免把空字符串误判为不存在。
3. `Put` 按新 key、版本匹配、版本冲突分别处理，并在锁内完成更新。
4. Clerk 初版把 RPC 无回复误判为 `ErrNoKey`，改为持续重试。
5. `Put` 初次和重试后的 `ErrVersion` 语义不同，增加重试状态标志区分 `ErrVersion` / `ErrMaybe`。
6. Lock 初版把所有 `Get` 的 `OK` 都当作空闲，可能覆盖其他客户端；修正为只接受空字符串 value。
7. Lock 的 `ErrMaybe` 必须通过下一轮 Get 确认，不能使用 Put 前的旧 value。

## 9. 验证与经验

`cd src && make kvsrv1`，全部通过（带 `-race`）：

- `TestReliablePut`
- `TestPutConcurrentReliable`
- `TestMemPutManyClientsReliable`
- `TestUnreliableNet`

`cd src && make lock1`，全部通过（带 `-race`）：

- `TestReliableBasic`
- `TestReliableNested`
- `TestOneClientReliable`
- `TestManyClientsReliable`
- `TestOneClientUnreliable`
- `TestManyClientsUnreliable`

经验：

- 版本号既是数据版本，也是并发条件
- RPC 重试必须区分“没有回复”和“服务器返回错误”
- `ErrMaybe` 不能直接当作成功或失败
- 共享锁状态放在 KV Server，释放前验证持有者
