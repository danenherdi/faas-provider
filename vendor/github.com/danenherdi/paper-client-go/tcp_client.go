/*
 * Copyright (c) Kia Shakiba
 *
 * This source code is licensed under the GNU AGPLv3 license found in the
 * LICENSE file in the root directory of this source tree.
 */

package paperclient

import (
	"net"
	"errors"
)

type tcpClient struct {
	conn *net.TCPConn
}

func tcpClientConnect(addr string) (*tcpClient, error) {
	server, err := net.ResolveTCPAddr("tcp", addr)

	if err != nil {
		return nil, errors.New("Invalid host or port.")
	}

	conn, err := net.DialTCP("tcp", nil, server)

	if err != nil {
		return nil, errors.New("Could not connect to server.")
	}

	client := tcpClient {
		conn,
	}

	return &client, nil
}

func (client *tcpClient) getConn() *net.TCPConn {
	return client.conn
}

func (client *tcpClient) send(sheet *sheetWriter) (error) {
	_, err := client.conn.Write(sheet.getBuf())
	return err
}
