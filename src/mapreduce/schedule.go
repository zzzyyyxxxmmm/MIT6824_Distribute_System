package mapreduce

import (
	"fmt"
	"sync"
)

// schedule starts and waits for all tasks in the given phase (Map or Reduce).
func (mr *Master) schedule(phase jobPhase) {
	var ntasks int
	var nios int // number of inputs (for reduce) or outputs (for map)
	switch phase {
	case mapPhase:
		ntasks = len(mr.files)
		nios = mr.nReduce
	case reducePhase:
		ntasks = mr.nReduce
		nios = len(mr.files)
	}

	fmt.Printf("Schedule: %v %v tasks (%d I/Os)\n", ntasks, phase, nios)

	// All ntasks tasks have to be scheduled on workers, and only once all of
	// them have been completed successfully should the function return.
	// Remember that workers may fail, and that any given worker may finish
	// multiple tasks.
	//
	// TODO TODO TODO TODO TODO TODO TODO TODO TODO TODO TODO TODO TODO
	//

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
}
