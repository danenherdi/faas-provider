/*
 * Copyright (c) Kia Shakiba
 *
 * This source code is licensed under the GNU AGPLv3 license found in the
 * LICENSE file in the root directory of this source tree.
 */

package paperclient

import (
	"sync"
	"sync/atomic"
)

type PaperPool struct {
	clients []*LockableClient
	index uint32
}

type LockableClient struct {
	client *PaperClient
	lock *sync.Mutex
}

func PoolConnect(paper_addr string, size uint32) (*PaperPool, error) {
	clients := []*LockableClient{}

	for i := uint32(0); i < size; i++ {
		client, err := ClientConnect(paper_addr)

		if err != nil {
			return nil, err
		}

		lock := &sync.Mutex{}

		locked_client := LockableClient {
			client,
			lock,
		}

		clients = append(clients, &locked_client)
	}

	index := uint32(0)

	pool := PaperPool {
		clients,
		index,
	}

	return &pool, nil
}

func (pool *PaperPool) Disconnect() {
	for _, lockable_client := range pool.clients {
		client := lockable_client.Lock()
		client.Disconnect()
		lockable_client.Unlock()
	}
}

func (pool *PaperPool) Auth(token string) () {
	for _, lockable_client := range pool.clients {
		client := lockable_client.Lock()
		client.Auth(token)
		lockable_client.Unlock()
	}
}

func (pool *PaperPool) LockableClient() (*LockableClient) {
	client := pool.clients[pool.index]

	new_index := (pool.index + 1) % uint32(len(pool.clients))
	atomic.StoreUint32(&pool.index, new_index)

	return client
}

func (lockable_client *LockableClient) Lock() (*PaperClient) {
	lockable_client.lock.Lock()
	return lockable_client.client
}

func (lockable_client *LockableClient) Unlock() {
	lockable_client.lock.Unlock()
}
