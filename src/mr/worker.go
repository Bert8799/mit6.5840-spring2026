package mr

import "fmt"
import "log"
import "net/rpc"
import "hash/fnv"
import "os"
import "time"
import "io"
import "sort"
import "encoding/json"

// Map functions return a slice of KeyValue.
type KeyValue struct {
	Key   string
	Value string
}

type ByKey []KeyValue

// for sorting by key.
func (a ByKey) Len() int           { return len(a) }
func (a ByKey) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByKey) Less(i, j int) bool { return a[i].Key < a[j].Key }

// use ihash(key) % NReduce to choose the reduce
// task number for each KeyValue emitted by Map.
func ihash(key string) int {
	h := fnv.New32a()
	h.Write([]byte(key))
	return int(h.Sum32() & 0x7fffffff)
}

var coordSockName string // socket for coordinator

// main/mrworker.go calls this function.
func Worker(sockname string, mapf func(string, string) []KeyValue,
	reducef func(string, []string) string) {

	coordSockName = sockname

	// Your worker implementation here.
	var taskArgs TaskArgs
	var taskReply TaskReply

	for {
		taskArgs = TaskArgs{}
		taskReply = TaskReply{}
		ok := call("Coordinator.FetchTask", &taskArgs, &taskReply)
		if !ok {
			log.Printf("%d: call failed err %v", os.Getpid(), "FetchTask")
			break
		}

		if taskReply.TaskType == ExitTask {
			log.Printf("%d: exit", os.Getpid())
			break
		}

		if taskReply.TaskType == WaitTask {
			log.Printf("%d: wait", os.Getpid())
			time.Sleep(time.Second)
			continue
		}

		var id int
		var err error
		if taskReply.TaskType == MapTask {
			log.Printf("%d: map task %v", os.Getpid(), taskReply.MapInfo)
			id = taskReply.MapInfo.Id
			err = doMapTask(taskReply.MapInfo, mapf)
		} else if taskReply.TaskType == ReduceTask {
			log.Printf("%d: reduce task %v", os.Getpid(), taskReply.ReduceInfo)
			id = taskReply.ReduceInfo.Id
			err = doReduceTask(taskReply.ReduceInfo, reducef)
		} else {
			log.Printf("%d: unknown task type %v", os.Getpid(), taskReply.TaskType)
			break
		}

		if err != nil {
			log.Printf("%d: task %d failed err %v", os.Getpid(), id, err)
			continue
		}

		reportArgs := ReportArgs{
			TaskType: taskReply.TaskType,
			Id:       id,
		}
		reportReply := ReportReply{}
		ok = call("Coordinator.ReportTask", &reportArgs, &reportReply)
		if !ok {
			log.Printf("%d: call failed err %v", os.Getpid(), "ReportTask")
			break
		}
	}

	// uncomment to send the Example RPC to the coordinator.
	// CallExample()

}

func doMapTask(mapInfo MapTaskInfo, mapf func(string, string) []KeyValue) error {
	// Your code here (Part I).
	// read the input file
	content, err := os.ReadFile(mapInfo.Filename)
	if err != nil {
		return fmt.Errorf("cannot read %v: %v", mapInfo.Filename, err)
	}

	kva := mapf(mapInfo.Filename, string(content))
	buckets := make([][]KeyValue, mapInfo.NReduce)
	for _, kv := range kva {
		reduceTaskNum := ihash(kv.Key) % mapInfo.NReduce
		buckets[reduceTaskNum] = append(buckets[reduceTaskNum], kv)
	}

	for i := 0; i < mapInfo.NReduce; i++ {
		oname := fmt.Sprintf("mr-%d-%d", mapInfo.Id, i)
		if err := writeIntermediateFile(oname, buckets[i]); err != nil {
			return fmt.Errorf("cannot write intermediate file %v: %v", oname, err)
		}
	}

	return nil
}

func writeIntermediateFile(name string, kva []KeyValue) error {
	file, err := os.CreateTemp(".", "mr-tmp-*")
	if err != nil {
		return fmt.Errorf("cannot create temp file for intermediate file %v: %v", name, err)
	}
	defer file.Close()
	defer os.Remove(file.Name())

	enc := json.NewEncoder(file)
	for _, kv := range kva {
		if err := enc.Encode(&kv); err != nil {
			return fmt.Errorf("cannot encode kv to intermediate file %v: %v", name, err)
		}
	}

	err = file.Close()
	if err != nil {
		return fmt.Errorf("cannot close temp file for intermediate file %v: %v", name, err)
	}

	err = os.Rename(file.Name(), name)
	if err != nil {
		return fmt.Errorf("cannot rename temp file to intermediate file %v: %v", name, err)
	}

	return nil
}

func doReduceTask(reduceInfo ReduceTaskInfo, reducef func(string, []string) string) error {
	// Your code here (Part I).
	// read the intermediate files
	intermediate := []KeyValue{}
	for i := 0; i < reduceInfo.NMap; i++ {
		iname := fmt.Sprintf("mr-%d-%d", i, reduceInfo.Id)
		kv, err := readIntermediateFile(iname)
		if err != nil {
			return fmt.Errorf("cannot read intermediate file %v: %v", iname, err)
		}
		intermediate = append(intermediate, kv...)
	}

	sort.Sort(ByKey(intermediate))

	ofile, err := os.CreateTemp(".", "mr-tmp-out-*")
	if err != nil {
		return fmt.Errorf("cannot create temp file for output file %d: %v", reduceInfo.Id, err)
	}
	defer ofile.Close()
	defer os.Remove(ofile.Name())

	i := 0
	for i < len(intermediate) {
		j := i + 1
		for j < len(intermediate) && intermediate[j].Key == intermediate[i].Key {
			j++
		}
		values := []string{}
		for k := i; k < j; k++ {
			values = append(values, intermediate[k].Value)
		}
		output := reducef(intermediate[i].Key, values)

		_, err = fmt.Fprintf(ofile, "%v %v\n", intermediate[i].Key, output)
		if err != nil {
			return fmt.Errorf("cannot write to temp file for output file %d: %v", reduceInfo.Id, err)
		}

		i = j
	}

	err = ofile.Close()
	if err != nil {
		return fmt.Errorf("cannot close temp file for output file %d: %v", reduceInfo.Id, err)
	}

	err = os.Rename(ofile.Name(), fmt.Sprintf("mr-out-%d", reduceInfo.Id))
	if err != nil {
		return fmt.Errorf("cannot rename temp file to output file %d: %v", reduceInfo.Id, err)
	}

	return nil
}

func readIntermediateFile(name string) ([]KeyValue, error) {
	intermediate := []KeyValue{}
	file, err := os.Open(name)
	if err != nil {
		return nil, fmt.Errorf("cannot open %v: %v", name, err)
	}
	defer file.Close()

	dec := json.NewDecoder(file)
	for {
		var kv KeyValue
		if err := dec.Decode(&kv); err == io.EOF {
			break
		} else if err != nil {
			return nil, fmt.Errorf("cannot decode kv from %v: %v", name, err)
		}
		intermediate = append(intermediate, kv)
	}

	return intermediate, nil
}

// example function to show how to make an RPC call to the coordinator.
//
// the RPC argument and reply types are defined in rpc.go.
func CallExample() {

	// declare an argument structure.
	args := ExampleArgs{}

	// fill in the argument(s).
	args.X = 99

	// declare a reply structure.
	reply := ExampleReply{}

	// send the RPC request, wait for the reply.
	// the "Coordinator.Example" tells the
	// receiving server that we'd like to call
	// the Example() method of struct Coordinator.
	ok := call("Coordinator.Example", &args, &reply)
	if ok {
		// reply.Y should be 100.
		fmt.Printf("reply.Y %v\n", reply.Y)
	} else {
		fmt.Printf("call failed!\n")
	}
}

// send an RPC request to the coordinator, wait for the response.
// usually returns true.
// returns false if something goes wrong.
func call(rpcname string, args interface{}, reply interface{}) bool {
	// c, err := rpc.DialHTTP("tcp", "127.0.0.1"+":1234")
	c, err := rpc.DialHTTP("unix", coordSockName)
	if err != nil {
		log.Fatal("dialing:", err)
	}
	defer c.Close()

	if err := c.Call(rpcname, args, reply); err == nil {
		return true
	}
	log.Printf("%d: call failed err %v", os.Getpid(), err)
	return false
}
