// 这个程序启动 N 个并发协程，对互斥锁 RPC 服务进行访问。
//
// 协程通过 Lock RPC 调用，获取互斥锁，进入临界区，对共享的 critical 变量进行自增操作。
// 完成临界操作后，通过 Unlock RPC 调用释放锁，退出临界区。
//
// 如果一切正确，那么最终 critical 变量的值应该等于 N。例如 N = 1000 时：
//
//	✅ critical = 1000, expected = 1000
//
// 若锁服务实现有误，那么 critical 变量的值可能小于 N，例如：
//
//	❌ critical = 992, expected = 1000
//
// 注释掉 tryLock 中的两行 RPC 调用代码（mutex.Call），再次运行程序，即可看到这种错误情况。
package main

import (
	"flag"
	"fmt"
	"simpleRpc/jsonrpc2"
	"simpleRpc/lock"
	"sync"
)

// N is the number of concurrent goroutines.
// You can change it by passing -n=10 to the program.
var N = flag.Int("n", 1000, "number of goroutines")
var critical = 0

func tryLock(mutex jsonrpc2.Client) {
	must(mutex.Call(lock.MethodLock, &lock.LockRequest{}, &lock.LockResponse{}))

	// critical section
	critical += 1

	must(mutex.Call(lock.MethodUnlock, &lock.UnlockRequest{}, &lock.UnlockResponse{}))
}

func main() {
	flag.Parse()

	mutexRpcClient := jsonrpc2.NewClient(
		jsonrpc2.NewHttpClientTransport("http://localhost" + lock.ServerAddr))

	wg := sync.WaitGroup{}

	for i := 0; i < *N; i++ {
		go func() {
			wg.Add(1)
			defer wg.Done()
			tryLock(mutexRpcClient)
		}()
	}

	wg.Wait()

	correct := "❌"
	if critical == *N {
		correct = "✅"
	}
	fmt.Printf("%s critical = %d, expected = %d", correct, critical, *N)
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
