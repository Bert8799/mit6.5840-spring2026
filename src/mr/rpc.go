package mr

//
// RPC definitions.
//
// remember to capitalize all names.
//

//
// example to show how to declare the arguments
// and reply for an RPC.
//

type ExampleArgs struct {
	X int
}

type ExampleReply struct {
	Y int
}

// Add your RPC definitions here.

type MapTaskInfo struct {
	Id       int
	Filename string
	NReduce  int
}

type ReduceTaskInfo struct {
	Id   int
	NMap int
}

type TaskType int

const (
	MapTask TaskType = iota
	ReduceTask
	WaitTask
	ExitTask
)

type TaskArgs struct{}

type TaskReply struct {
	TaskType   TaskType
	MapInfo    MapTaskInfo
	ReduceInfo ReduceTaskInfo
}

type ReportArgs struct {
	TaskType TaskType
	Id       int
}

type ReportReply struct{}
