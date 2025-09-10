/*
 * Copyright (c) Kia Shakiba
 *
 * This source code is licensed under the GNU AGPLv3 license found in the
 * LICENSE file in the root directory of this source tree.
 */

package paperclient

import (
	"errors"
	"strings"
)

const (
	pingByte uint8 = 0
	versionByte uint8 = 1

	authByte uint8 = 2

	getByte uint8 = 3
	setByte uint8 = 4
	delByte uint8 = 5

	hasByte uint8 = 6
	peekByte uint8 = 7
	ttlByte uint8 = 8
	sizeByte uint8 = 9

	wipeByte uint8 = 10

	resizeByte uint8 = 11
	policyByte uint8 = 12

	statusByte uint8 = 13
)

const maxReconnectAttempts = 3

type PaperClient struct {
	addr string

	auth_token *string
	reconnect_attempts uint32

	tcp_client *tcpClient
}

// Connects to PaperCache server at the provided address.
func ClientConnect(paper_addr string) (*PaperClient, error) {
	addr_ptr, err := parsePaperAddr(paper_addr)

	if err != nil {
		return nil, err
	}

	addr := *addr_ptr
	tcp_client, err := tcpClientConnect(addr)

	if err != nil {
		return nil, err
	}

	var auth_token *string = nil
	var reconnect_attempts uint32 = 0

	client := PaperClient {
		addr,

		auth_token,
		reconnect_attempts,

		tcp_client,
	}

	_, ping_err := client.Ping()

	if ping_err != nil {
		return nil, errors.New("Connection refused.")
	}

	return &client, nil
}

// Disconnects from the server.
func (client *PaperClient) Disconnect() {
	client.tcp_client.getConn().Close()
}

// Pings the server.
func (client *PaperClient) Ping() (string, error) {
	writer := initSheetWriter()
	writer.writeU8(pingByte)

	return client.processData(writer)
}

// Gets the cache version.
func (client *PaperClient) Version() (string, error) {
	writer := initSheetWriter()
	writer.writeU8(versionByte)

	return client.processData(writer)
}

// Attempts to authorize the connection with the supplied auth token.
// This must match the auth token specified in the server's configured
// to be successful.
func (client *PaperClient) Auth(token string) error {
	client.auth_token = &token

	writer := initSheetWriter()
	writer.writeU8(authByte)
	writer.writeString(token)

	return client.process(writer)
}

// Gets the value of the supplied key from the cache.
func (client *PaperClient) Get(key string) (string, error) {
	writer := initSheetWriter()
	writer.writeU8(getByte)
	writer.writeString(key)

	return client.processData(writer)
}

// Sets the supplied key, value, and TTL to the cache.
func (client *PaperClient) Set(key string, value string, ttl uint32) error {
	writer := initSheetWriter()
	writer.writeU8(setByte)
	writer.writeString(key)
	writer.writeString(value)
	writer.writeU32(ttl)

	return client.process(writer)
}

// Deletes the value of the supplied key from the cache.
func (client *PaperClient) Del(key string) error {
	writer := initSheetWriter()
	writer.writeU8(delByte)
	writer.writeString(key)

	return client.process(writer)
}

// Checks if the cache contains an object with the supplied key
// without altering the eviction order of the objects.
func (client *PaperClient) Has(key string) (bool, error) {
	writer := initSheetWriter()
	writer.writeU8(hasByte)
	writer.writeString(key)

	return client.processHas(writer)
}

// Gets (peeks) the value of the supplied key from the cache without
// altering the eviction order of the objects.
func (client *PaperClient) Peek(key string) (string, error) {
	writer := initSheetWriter()
	writer.writeU8(peekByte)
	writer.writeString(key)

	return client.processData(writer)
}

// Sets the TTL associated with the supplied key.
func (client *PaperClient) Ttl(key string, ttl uint32) error {
	writer := initSheetWriter()
	writer.writeU8(ttlByte)
	writer.writeString(key)
	writer.writeU32(ttl)

	return client.process(writer)
}

// Gets the size of the value of the supplied key from the cache in bytes.
func (client *PaperClient) Size(key string) (uint32, error) {
	writer := initSheetWriter()
	writer.writeU8(sizeByte)
	writer.writeString(key)

	return client.processSize(writer)
}

// Wipes the contents of the cache.
func (client *PaperClient) Wipe() error {
	writer := initSheetWriter()
	writer.writeU8(wipeByte)

	return client.process(writer)
}

// Resizes the cache to the supplied size.
func (client *PaperClient) Resize(size uint64) error {
	writer := initSheetWriter()
	writer.writeU8(resizeByte)
	writer.writeU64(size)

	return client.process(writer)
}

// Sets the cache's eviction policy.
func (client *PaperClient) Policy(policy string) error {
	writer := initSheetWriter()
	writer.writeU8(policyByte)
	writer.writeString(policy)

	return client.process(writer)
}

// Gets the cache's status.
func (client *PaperClient) Status() (*PaperStatus, error) {
	writer := initSheetWriter()
	writer.writeU8(statusByte)

	return client.processStatus(writer)
}

func (client *PaperClient) reconnect() (error) {
	client.reconnect_attempts += 1

	if client.reconnect_attempts > maxReconnectAttempts {
		return PaperErrorMaxConnectionsExceeded
	}

	tcp_client, err := tcpClientConnect(client.addr)

	if err != nil {
		return err
	}

	client.tcp_client = tcp_client

	if client.auth_token != nil {
		if err := client.Auth(*client.auth_token); err != nil {
			return err
		}
	}

	return nil
}

func (client *PaperClient) process(writer *sheetWriter) error {
	err := client.tcp_client.send(writer)

	if err != nil {
		if err := client.reconnect(); err != nil {
			return err
		}

		return client.process(writer)
	}

	client.reconnect_attempts = 0
	reader := initSheetReader(client.tcp_client)

	is_ok, err := reader.readBool()

	if err != nil {
		return err
	}

	if !is_ok {
		return errorFromReader(reader)
	}

	return nil
}

func (client *PaperClient) processData(writer *sheetWriter) (string, error) {
	err := client.tcp_client.send(writer)

	if err != nil {
		if err := client.reconnect(); err != nil {
			return "", err
		}

		return client.processData(writer)
	}

	client.reconnect_attempts = 0
	reader := initSheetReader(client.tcp_client)

	is_ok, err := reader.readBool()

	if err != nil {
		return "", err
	}

	if !is_ok {
		return "", errorFromReader(reader)
	}

	return reader.readString()
}

func (client *PaperClient) processHas(writer *sheetWriter) (bool, error) {
	err := client.tcp_client.send(writer)

	if err != nil {
		if err := client.reconnect(); err != nil {
			return false, err
		}

		return client.processHas(writer)
	}

	client.reconnect_attempts = 0
	reader := initSheetReader(client.tcp_client)

	is_ok, err := reader.readBool()

	if err != nil {
		return false, err
	}

	if !is_ok {
		return false, errorFromReader(reader)
	}

	return reader.readBool()
}

func (client *PaperClient) processSize(writer *sheetWriter) (uint32, error) {
	err := client.tcp_client.send(writer)

	if err != nil {
		if err := client.reconnect(); err != nil {
			return 0, err
		}

		return client.processSize(writer)
	}

	client.reconnect_attempts = 0
	reader := initSheetReader(client.tcp_client)

	is_ok, err := reader.readBool()

	if err != nil {
		return 0, err
	}

	if !is_ok {
		return 0, errorFromReader(reader)
	}

	return reader.readU32()
}

func (client *PaperClient) processStatus(writer *sheetWriter) (*PaperStatus, error) {
	err := client.tcp_client.send(writer)

	if err != nil {
		if err := client.reconnect(); err != nil {
			return nil, err
		}

		return client.processStatus(writer)
	}

	client.reconnect_attempts = 0
	reader := initSheetReader(client.tcp_client)

	is_ok, err := reader.readBool()

	if err != nil {
		return nil, err
	}

	if !is_ok {
		return nil, errorFromReader(reader)
	}

	return statusFromReader(reader)
}

func parsePaperAddr(paper_addr string) (*string, error) {
	if !strings.HasPrefix(paper_addr, "paper://") {
		return nil, errors.New("Invalid paper address.");
	}

	addr := strings.Replace(paper_addr, "paper://", "", 1)

	return &addr, nil
}
