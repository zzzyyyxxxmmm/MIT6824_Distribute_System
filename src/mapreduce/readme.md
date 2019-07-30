## Backgroud
Package mapreduce provides a simple mapreduce library with a sequential
implementation. Applications should normally call Distributed() [located in
master.go] to start a job, but may instead call Sequential() [also in
master.go] to get a sequential execution for debugging purposes.

The flow of the mapreduce implementation is as follows:

1. The application provides a number of input files, a map function, a
   reduce function, and the number of reduce tasks (nReduce).
2. A master is created with this knowledge. It spins up an RPC server (see
   master_rpc.go), and waits for workers to register (using the RPC call
   Register() [defined in master.go]). As tasks become available (in steps
   4 and 5), schedule() [schedule.go] decides how to assign those tasks to
   workers, and how to handle worker failures.
3. The master considers each input file one map tasks, and makes a call to
   doMap() [common_map.go] at least once for each task. It does so either
   directly (when using Sequential()) or by issuing the DoJob RPC on a
   worker [worker.go]. Each call to doMap() reads the appropriate file,
   calls the map function on that file's contents, and produces nReduce
   files for each map file. Thus, there will be #files x nReduce files
   after all map tasks are done:

      f0-0, ..., f0-0, f0-<nReduce-1>, ...,
      f<#files-1>-0, ... f<#files-1>-<nReduce-1>.

4. The master next makes a call to doReduce() [common_reduce.go] at least
   once for each reduce task. As for doMap(), it does so either directly or
   through a worker. doReduce() collects nReduce reduce files from each
   map (f-*-<reduce>), and runs the reduce function on those files. This
   produces nReduce result files.
5. The master calls mr.merge() [master_splitmerge.go], which merges all
   the nReduce files produced by the previous step into a single output.
6. The master sends a Shutdown RPC to each of its workers, and then shuts
   down its own RPC server.

TODO:
You will have to write/modify doMap, doReduce, and schedule yourself. These
are located in common_map.go, common_reduce.go, and schedule.go
respectively. You will also have to write the map and reduce functions in
../main/wc.go.

You should not need to modify any other files, but reading them might be
useful in order to understand how the other methods fit into the overall
architecture of the system.
package mapreduce

## domap
好了，如果有读过map reduce这篇论文的话，应该对整个工作过程有个大致了解，其实一开始拿到代码我也是一头雾水，不知道从什么地方下手，有几个问题尤其需要弄清：

* 整个工作流程
* 到底读哪个文件，map完之后需要转化成什么形式

如果我们写完了代码，PartI需要们运行`go test -run Sequential`来进行测试, go test 命令如下

```
go test -run ''      # Run all tests.
go test -run Foo     # Run top-level tests matching "Foo", such as "TestFooBar".
go test -run Foo/A=  # For top-level tests matching "Foo", run subtests matching "A=".
go test -run /A=1    # For all top-level tests, run subtests matching "A=1".
```

显然我们需要定位到test_test.go文件中，里面有两个test方法：TestSequentialSingle，TestSequentialMany
显然最终我们就是运行这两个方法来进行测试
```go
func TestSequentialMany(t *testing.T) {
	mr := Sequential("test", makeInputs(5), 3, MapFunc, ReduceFunc)
	mr.Wait()
	check(t, mr.files)
	checkWorker(t, mr.stats)
	cleanup(mr)
}
```

里面创建了一个Sequential来运行，makeInputs(5)就是创建一些包含数字的以824开头的文件并返回文件名，MapFunc就是将string序列split成一个一个单词的key value对

再来看一下Sequential方法：
```go
// Sequential runs map and reduce tasks sequentially, waiting for each task to
// complete before scheduling the next.
func Sequential(jobName string, files []string, nreduce int,
	mapF func(string, string) []KeyValue,
	reduceF func(string, []string) string,
) (mr *Master) {
	mr = newMaster("master")
	go mr.run(jobName, files, nreduce, func(phase jobPhase) {
		switch phase {
		case mapPhase:
			for i, f := range mr.files {
				doMap(mr.jobName, i, f, mr.nReduce, mapF)
			}
		case reducePhase:
			for i := 0; i < mr.nReduce; i++ {
				doReduce(mr.jobName, i, len(mr.files), reduceF)
			}
		}
	}, func() {
		mr.stats = []int{len(files) + nreduce}
	})
	return
}
```
这个方法里创建了一个类似与Runnable的Master来进行工作，工作经历了mapPhase和reducePhase阶段，那么doMap就是在这里进行的。

在common_map.go文件里：
```go
func doMap(
	jobName string, // the name of the MapReduce job
	mapTaskNumber int, // which map task this is
	inFile string,
	nReduce int, // the number of reduce task that will be run ("R" in the paper)
	mapF func(file string, contents string) []KeyValue,
) {

}
```

传入了之前读到的文件，有个mapF方法可以将文件分成单个的word key value对，nReduce告诉我们需要将读到的文件分为几个部分


总之，doMap应该做的工作：
1. 建立nReduce个中间文件
2. 将读入的word 映射，并且保存在中间文件中

### 读取文件
读取文件后通过mapF方法将其转化为键值对
```go
data, err := ioutil.ReadFile(inFile)
if err != nil {
   fmt.Println("File reading error", err)
   return
}
result := mapF(inFile, string(data))
```

### 创建reduce文件
根据nReduce创建文件，创建方法在注释里已经告诉我们了
```go
file := make([]*os.File, nReduce)
for i := 0; i < nReduce; i++ {
   file[i], err = os.Create(reduceName(jobName, mapTaskNumber, i))
   if err != nil {
      log.Fatal(err)
   }
}
```
### 将键值对写入对应文件
```
// You can find the filename for this map task's input to reduce task number
// r using reduceName(jobName, mapTaskNumber, r). The ihash function (given
// below doMap) should be used to decide which file a given key belongs into.
```
word 键值对写到哪个文件是由ihash决定的，另外需要以json格式写入
```go
for _, word := range result {
		enc := json.NewEncoder(file[ihash(word.Key)%uint32(nReduce)])
		err := enc.Encode(&word)
		if err != nil {
			log.Fatal(err)
		}
   }
```

### 关闭文件
```go
for _, file := range file {
		file.Close()
   }
```

现在我们运行完，目录下应该出现了一堆mrtmp.test-1-1里面是一堆键值对

## doReduce

doReduce 其实已经和doMap差不多了
1. 读取中间文件
2. 排序
3. 写入

# Part III: Distributing MapReduce tasks

这部分就是把之前Sequential执行的转换成分布式执行

之前的这段：
```go
go mr.run(jobName, files, nreduce, func(phase jobPhase) {
		switch phase {
		case mapPhase:
			for i, f := range mr.files {
				doMap(mr.jobName, i, f, mr.nReduce, mapF)
			}
		case reducePhase:
			for i := 0; i < mr.nReduce; i++ {
				doReduce(mr.jobName, i, len(mr.files), reduceF)
			}
		}
	}, func() {
		mr.stats = []int{len(files) + nreduce}
	})
```

在一个for循环里依次执行，这次我们需要将这些依次执行的代码变成在不同机器上并发执行的代码

To coordinate the parallel execution of tasks, we will use a special master thread, which hands out work to the workers and waits for them to finish. To make the lab more realistic, the master should only communicate with the workers via RPC. We give you the worker code (mapreduce/worker.go), the code that starts the workers, and code to deal with RPC messages (mapreduce/common_rpc.go).

上面这句话告诉我们，Master会将task分给不同的worker来执行，由于worker在不同的机器上，因此我们需要RPC来进行通信

看一下common_rpc.go
```go
// DoTaskArgs holds the arguments that are passed to a worker when a job is
// scheduled on it.
type DoTaskArgs struct {
	JobName    string
	File       string   // the file to process
	Phase      jobPhase // are we in mapPhase or reducePhase?
	TaskNumber int      // this task's index in the current phase

	// NumOtherPhase is the total number of tasks in other phase; mappers
	// need this to compute the number of output bins, and reducers needs
	// this to know how many input files to collect.
	NumOtherPhase int
}
```

可以看到上面就是我们通过master发送给worker的信息

```go
// call() sends an RPC to the rpcname handler on server srv
// with arguments args, waits for the reply, and leaves the
// reply in reply. the reply argument should be the address
// of a reply structure.
func call(srv string, rpcname string,
	args interface{}, reply interface{}) bool {
	}
```
而call方法就是具体的发送方法, 指定发送的worker，参数

Look at run() in master.go. It calls your schedule() to run the map and reduce tasks, then calls merge() to assemble the per-reduce-task outputs into a single output file. schedule only needs to tell the workers the name of the original input file (mr.files[task]) and the task task; each worker knows from which files to read its input and to which files to write its output. The master tells the worker about a new task by sending it the RPC call Worker.DoTask, giving a DoTaskArgs object as the RPC argument.

这段话告诉了我们基本的执行流程
1. schedule(mapPhase)
2. schedule(reducePhase)
3. mr.merge()

发送DoTaskArgs给worker,等待返回所有结果，然后执行下一个阶段，等待，合并

When a worker starts, it sends a Register RPC to the master. mapreduce.go already implements the master's Master.Register RPC handler for you, and passes the new worker's information to mr.registerChannel. Your schedule should process new worker registrations by reading from this channel.

这段告诉我们需要通过channel注册新的worker，worker的结果也是通过channel返回的

```go
ch <- v    // Send v to channel ch.
v := <-ch  // Receive from ch, and
		   // assign value to v.
```

好了通过分析上面的内容，代码的基本流程应该清楚了，首先为每个task构件好DoTaskArgs参数，然后注册worker，然后发送，等待获取结果，这些都是在异步条件下完成的

注意要等待上一阶段完成才能开始下一个阶段
这部分功能我们使用sync.WaitGroup。WaitGroup顾名思义，就是用来等待一组操作完成的。WaitGroup内部实现了一个计数器，用来记录未完成的操作个数，它提供了三个方法，Add()用来添加计数。Done()用来在操作结束时调用，使计数减一。Wait()用来等待所有的操作结束，即计数变为0，该函数会在计数不为0时等待，在计数为0时立即返回。

示例：
```go
func main() {
    var wg sync.WaitGroup
    wg.Add(2) // 因为有两个动作，所以增加2个计数
    go func() {
        fmt.Println("Goroutine 1")
        wg.Done() // 操作完成，减少一个计数
    }()
    go func() {
        fmt.Println("Goroutine 2")
        wg.Done() // 操作完成，减少一个计数
    }()
    wg.Wait() // 等待，直到计数为0
}
```

代码：
```go
var wg sync.WaitGroup
wg.Add(ntasks)
execute := func(worker string, taskId int) {
	args := DoTaskArgs{mr.jobName, mr.files[taskId], phase, taskId, nios}
	result := call(worker, "Worker.DoTask", args, nil)
	if result {
		wg.Done()
	}
	mr.registerChannel <- worker
}

for i := 0; i < ntasks; i++ {
	worker := <-mr.registerChannel
	go execute(worker, i)
}
```

# Part IV: Handling worker failures
In this part you will make the master handle failed workers. MapReduce makes this relatively easy because workers don't have persistent state. If a worker fails, any RPCs that the master issued to that worker will fail (e.g., due to a timeout). Thus, if the master's RPC to the worker fails, the master should re-assign the task given to the failed worker to another worker.

其实就是失败重发机制

这里不管成功失败都要释放worker

失败的时候要重新选择worker

```go
var wg sync.WaitGroup
wg.Add(ntasks)
var execute func(worker string, taskNum int)
execute = func(worker string, taskId int) {
	args := DoTaskArgs{mr.jobName, mr.files[taskId], phase, taskId, nios}
	result := call(worker, "Worker.DoTask", args, nil)
	if result {
		wg.Done()
	} else {
		newWorker := <-mr.registerChannel
		go execute(newWorker, taskId)
	}
	mr.registerChannel <- worker
}
for i := 0; i < ntasks; i++ {
	worker := <-mr.registerChannel
	go execute(worker, i)
}
wg.Wait()
fmt.Printf("Schedule: %v phase done\n", phase)
```