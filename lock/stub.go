package lock

const ServerAddr = ":5680"

const MethodLock = "lock"

type LockRequest struct{}

type LockResponse struct{}

const MethodUnlock = "unlock"

type UnlockRequest struct{}

type UnlockResponse struct{}
