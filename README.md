# MIT 6824

## [Part1:Map/Reduce input and output](https://github.com/zzzyyyxxxmmm/MIT6824_Distribute_System/tree/master/src/mapreduce)

这部分主要是最基础的mapreduce流程，从创建master，然后经历map，reduce，merge

了解go语言如何创建读取写入文件，键值对是如何创建，读取，合并，排序

## [Part II: Single-worker word count](https://github.com/zzzyyyxxxmmm/MIT6824_Distribute_System/tree/master/src/main)

其实就是PartI的应用，修改mapF和reduceF来进行word count

## [Part III: Distributing MapReduce tasks](https://github.com/zzzyyyxxxmmm/MIT6824_Distribute_System/tree/master/src/mapreduce#part-iii-distributing-mapreduce-tasks)

改成分布式，master通过RPC的方式将任务分发到各个worker来执行

## [Part IV: Handling worker failures](https://github.com/zzzyyyxxxmmm/MIT6824_Distribute_System/tree/master/src/mapreduce#part-iv-handling-worker-failures)

失败重传机制, 对于失败的任务重新执行