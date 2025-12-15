/*
 * Copyright (c) Kia Shakiba
 *
 * This source code is licensed under the GNU AGPLv3 license found in the
 * LICENSE file in the root directory of this source tree.
 */

package paperclient

import (
	"math"
	"encoding/binary"
)

type sheetReader struct {
	tcp_client *tcpClient
}

func initSheetReader(tcp_client *tcpClient) *sheetReader {
	return &sheetReader {
		tcp_client,
	}
}

func (sheet *sheetReader) readU8() (uint8, error) {
	data := make([]byte, 1)
	_, err := sheet.tcp_client.getConn().Read(data)

	if err != nil {
		return 0, err
	}

	return data[0], nil
}

func (sheet *sheetReader) readU32() (uint32, error) {
	data := make([]byte, 4)
	_, err := sheet.tcp_client.getConn().Read(data)

	if err != nil {
		return 0, err
	}

	return binary.LittleEndian.Uint32(data), nil
}

func (sheet *sheetReader) readU64() (uint64, error) {
	data := make([]byte, 8)
	_, err := sheet.tcp_client.getConn().Read(data)

	if err != nil {
		return 0, err
	}

	return binary.LittleEndian.Uint64(data), nil
}

func (sheet *sheetReader) readF64() (float64, error) {
	data := make([]byte, 8)
	_, err := sheet.tcp_client.getConn().Read(data)

	if err != nil {
		return 0, err
	}

	bits := binary.LittleEndian.Uint64(data)
	return math.Float64frombits(bits), nil
}

func (sheet *sheetReader) readBool() (bool, error) {
	data, err := sheet.readU8()

	if err != nil {
		return false, err
	}

	return data == '!', nil
}

func (sheet *sheetReader) readString() (string, error) {
	length, err := sheet.readU32()

	if err != nil {
		return "", err
	}

	data := make([]byte, length)
	_, err = sheet.tcp_client.getConn().Read(data)

	if err != nil {
		return "", err
	}

	return string(data), nil
}
