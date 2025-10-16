/*
 * Copyright (c) Kia Shakiba
 *
 * This source code is licensed under the GNU AGPLv3 license found in the
 * LICENSE file in the root directory of this source tree.
 */

package paperclient

import (
	"encoding/binary"
)

type sheetWriter struct {
	buf []byte
}

func initSheetWriter() *sheetWriter {
	return &sheetWriter {
		buf: []byte{},
	}
}

func (sheet *sheetWriter) getBuf() []byte {
	return sheet.buf
}

func (sheet *sheetWriter) writeU8(value uint8) {
	sheet.buf = append(sheet.buf, value)
}

func (sheet *sheetWriter) writeU32(value uint32) {
	data := make([]byte, 4)
	binary.LittleEndian.PutUint32(data, value)
	sheet.buf = append(sheet.buf, data...)
}

func (sheet *sheetWriter) writeU64(value uint64) {
	data := make([]byte, 8)
	binary.LittleEndian.PutUint64(data, value)
	sheet.buf = append(sheet.buf, data...)
}

func (sheet *sheetWriter) writeString(value string) {
	length := len(value)
	sheet.writeU32(uint32(length))
	sheet.buf = append(sheet.buf, value...)
}
