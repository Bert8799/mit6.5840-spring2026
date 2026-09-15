package mr

import "log"
import "net"
import "os"
import "net/rpc"
import "net/http"
import "sync"
import "time"
import "fmt"

type state int

const (
	Idle state = iota
	InProgress
	Completed
)

type mapTask struct {
	State       state
	AssignedAt  time.Time
	MapTaskInfo MapTaskInfo
}

type reduceTask struct {
	State          state
	AssignedAt     time.Time
	ReduceTaskInfo ReduceTaskInfo
}

type Coordinator struct {
	// Your definitions here.
	mu          sync.Mutex
	MapTasks    []*mapTask
	ReduceTasks []*reduceTask
}

// Your code here -- RPC handlers for the worker to call.

// an example RPC handler.
//
// the RPC argument and reply types are defined in rpc.go.
func (c *Coordinator) Example(args *ExampleArgs, reply *ExampleReply) error {
	reply.Y = args.X + 1
	return nil
}

// start a thread that listens for RPCs from worker.go
func (c *Coordinator) server(sockname string) {
	rpc.Register(c)
	rpc.HandleHTTP()
	os.Remove(sockname)
	l, e := net.Listen("unix", sockname)
	if e != nil {
		log.Fatalf("listen error %s: %v", sockname, e)
	}
	go http.Serve(l, nil)
}

// main/mrcoordinator.go calls Done() periodically to find out
// if the entire job has finished.
func (c *Coordinator) Done() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.doneLocked()
}

func (c *Coordinator) doneLocked() bool {
	ret := false

	for _, reduceTask := range c.ReduceTasks {
		if reduceTask.State != Completed {
			return ret
		}
	}
	ret = true

	return ret
}

// create a Coordinator.
// main/mrcoordinator.go calls this function.
// nReduce is the number of reduce tasks to use.
func MakeCoordinator(sockname string, files []string, nReduce int) *Coordinator {
	c := Coordinator{}

	// Your code here.
	c.MapTasks = make([]*mapTask, len(files))
	for i, filename := range files {
		c.MapTasks[i] = &mapTask{
			State: Idle,
			MapTaskInfo: MapTaskInfo{
				Id:       i,
				Filename: filename,
				NReduce:  nReduce,
			},
		}
	}
	c.ReduceTasks = make([]*reduceTask, nReduce)
	for i := 0; i < nReduce; i++ {
		c.ReduceTasks[i] = &reduceTask{
			State: Idle,
			ReduceTaskInfo: ReduceTaskInfo{
				Id:   i,
				NMap: len(files),
			},
		}
	}

	c.server(sockname)
	return &c
}

func (c *Coordinator) mapDoneLocked() bool {
	ret := false

	for _, mapTask := range c.MapTasks {
		if mapTask.State != Completed {
			return ret
		}
	}
	ret = true

	return ret
}

func (c *Coordinator) fetchMapLocked() *MapTaskInfo {
	for _, mapTask := range c.MapTasks {
		if mapTask.State == Idle {
			mapTask.State = InProgress
			mapTask.AssignedAt = time.Now()
			return &mapTask.MapTaskInfo
		}
	}
	return nil
}

func (c *Coordinator) fetchReduceLocked() *ReduceTaskInfo {
	for _, reduceTask := range c.ReduceTasks {
		if reduceTask.State == Idle {
			reduceTask.State = InProgress
			reduceTask.AssignedAt = time.Now()
			return &reduceTask.ReduceTaskInfo
		}
	}
	return nil
}

func (c *Coordinator) expireTasksLocked() {
	now := time.Now()
	for _, mapTask := range c.MapTasks {
		if mapTask.State == InProgress && now.Sub(mapTask.AssignedAt) > 10*time.Second {
			mapTask.State = Idle
		}
	}
	for _, reduceTask := range c.ReduceTasks {
		if reduceTask.State == InProgress && now.Sub(reduceTask.AssignedAt) > 10*time.Second {
			reduceTask.State = Idle
		}
	}
}

func (c *Coordinator) FetchTask(args *TaskArgs, reply *TaskReply) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.expireTasksLocked()
	if !c.mapDoneLocked() {
		mapTaskInfo := c.fetchMapLocked()
		if mapTaskInfo != nil {
			reply.TaskType = MapTask
			reply.MapInfo = *mapTaskInfo
			return nil
		}
		reply.TaskType = WaitTask
		return nil
	}

	reduceTaskInfo := c.fetchReduceLocked()
	if reduceTaskInfo != nil {
		reply.TaskType = ReduceTask
		reply.ReduceInfo = *reduceTaskInfo
		return nil
	}
	if !c.doneLocked() {
		reply.TaskType = WaitTask
		return nil
	}
	reply.TaskType = ExitTask
	return nil
}

func (c *Coordinator) ReportTask(args *ReportArgs, reply *ReportReply) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if args.TaskType == MapTask {
		if args.Id < 0 || args.Id >= len(c.MapTasks) {
			return fmt.Errorf("invalid map task id %d", args.Id)
		}

		c.MapTasks[args.Id].State = Completed
		c.MapTasks[args.Id].AssignedAt = time.Time{} // Reset AssignedAt when completed
	} else if args.TaskType == ReduceTask {
		if args.Id < 0 || args.Id >= len(c.ReduceTasks) {
			return fmt.Errorf("invalid reduce task id %d", args.Id)
		}

		c.ReduceTasks[args.Id].State = Completed
		c.ReduceTasks[args.Id].AssignedAt = time.Time{} // Reset AssignedAt when completed
	}
	return nil
}
