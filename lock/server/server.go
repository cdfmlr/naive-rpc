// 这个程序实现了一个 RPC 锁服务 LockServer。
// 该服务提供两个远程过程：Lock 和 Unlock，分别用于获取和释放锁。
//
// 在 main 函数中，我们创建了一个 delta 值为 1 的 LockServer 实例，然后将其注册到 JSON-RPC 服务端。
// 初始化参数 delta=1 表示该锁服务最多允许一个客户端获取锁，即这是一个互斥锁服务。
package main

import (
	"simpleRpc/jsonrpc2"
	"simpleRpc/lock"
)

type LockServer struct {
	mu chan struct{}
}

func NewLockServer(delta int) *LockServer {
	return &LockServer{
		mu: make(chan struct{}, delta),
	}
}

func (s *LockServer) Lock(req *lock.LockRequest) (*lock.LockResponse, error) {
	s.mu <- struct{}{}
	return &lock.LockResponse{}, nil
}

func (s *LockServer) Unlock(req *lock.UnlockRequest) (*lock.UnlockResponse, error) {
	<-s.mu
	return &lock.UnlockResponse{}, nil
}

func main() {
	mutex := NewLockServer(1)

	s := jsonrpc2.NewServer()
	jsonrpc2.Verbose = true

	must(s.Register(lock.MethodLock, mutex.Lock))
	must(s.Register(lock.MethodUnlock, mutex.Unlock))

	st := jsonrpc2.NewHttpServerTransport(lock.ServerAddr)
	must(st.Serve(s))
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
