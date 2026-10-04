package lock

import (
	"time"

	"6.5840/kvtest1"
	"6.5840/kvsrv1/rpc"
)

type Lock struct {
	// IKVClerk is a go interface for k/v clerks: the interface hides
	// the specific Clerk type of ck but promises that ck supports
	// Put and Get.  The tester passes the clerk in when calling
	// MakeLock().
	ck kvtest.IKVClerk
	// You may add code here
	lockname string
	clientID string
}

// The tester calls MakeLock() and passes in a k/v clerk; your code can
// perform a Put or Get by calling lk.ck.Put() or lk.ck.Get().
//
// This interface supports multiple locks by means of the
// lockname argument; locks with different names should be
// independent.
func MakeLock(ck kvtest.IKVClerk, lockname string) *Lock {
	// You may add code here
	lk := &Lock{ck: ck, lockname: lockname, clientID: kvtest.RandValue(8)}
	return lk
}

func (lk *Lock) Acquire() {
	// Your code here
	for {
		value, version, err := lk.ck.Get(lk.lockname)
		if err == rpc.ErrNoKey || (err == rpc.OK && value == "") {
			// Lock is free, try to acquire it
			putErr := lk.ck.Put(lk.lockname, lk.clientID, version)
			if putErr == rpc.OK {
				// Successfully acquired the lock
				return
			}

			// ErrVersion means someone else acquired the lock before us, so we retry
			// ErrMaybe means we don't know if we acquired the lock, so we retry
		}
	
		if value == lk.clientID {
			// The caller already owns this lock. Treat a repeated Acquire as
			// already satisfied; this implementation does not track recursion depth.
			return
		}

		time.Sleep(10 * time.Millisecond) // Sleep for a short duration before retrying
	}
}

func (lk *Lock) Release() {
	// Your code here
	for {
		value, version, err := lk.ck.Get(lk.lockname)
		if err == rpc.ErrNoKey || (err == rpc.OK && value != lk.clientID) {
			// Lock is already free, nothing to do
			return
		}

		if value == lk.clientID {
			// We own the lock, try to release it
			putErr := lk.ck.Put(lk.lockname, "", version)
			if putErr == rpc.OK {
				// Successfully released the lock
				return
			}

			// ErrVersion means someone else modified the lock before us, so we retry
			// ErrMaybe means we don't know if we released the lock, so we retry
		}

		time.Sleep(10 * time.Millisecond) // Sleep for a short duration before retrying
	}
}
