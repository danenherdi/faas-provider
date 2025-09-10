/*
 * Copyright (c) Kia Shakiba
 *
 * This source code is licensed under the GNU AGPLv3 license found in the
 * LICENSE file in the root directory of this source tree.
 */

package paperclient

import "errors"

var PaperErrorInternal = errors.New("PaperError: internal")

var PaperErrorUnreachableServer = errors.New("PaperError: unreachable server")
var PaperErrorMaxConnectionsExceeded = errors.New("PaperError: max connections exceeded")
var PaperErrorUnauthorized = errors.New("PaperError: unauthorized")

var PaperErrorKeyNotFound = errors.New("PaperError: key not found")

var PaperErrorZeroValueSize = errors.New("PaperError: zero value size")
var PaperErrorExceedingValueSize = errors.New("PaperError: exceeding value size")

var PaperErrorUnconfiguredPolicy = errors.New("PaperError: unconfigured policy")
var PaperErrorInvalidPolicy = errors.New("PaperError: invalid policy")

var PaperErrorZeroCacheSize = errors.New("PaperError: zero cache size")

func errorFromReader(reader *sheetReader) error {
	code, err := reader.readU8()

	if err != nil {
		return err
	}

	if code == 0 {
		cache_code, err := reader.readU8()

		if err != nil {
			return err
		}

		return errorFromCacheCode(cache_code)
	}

	return errorFromCode(code)
}

func errorFromCode(code uint8) error {
	switch code {
		case 2: return PaperErrorMaxConnectionsExceeded
		case 3: return PaperErrorUnauthorized

		default: return PaperErrorInternal
	}
}

func errorFromCacheCode(code uint8) error {
	switch code {
		case 1: return PaperErrorKeyNotFound

		case 2: return PaperErrorZeroValueSize
		case 3: return PaperErrorExceedingValueSize

		case 4: return PaperErrorZeroCacheSize

		case 5: return PaperErrorUnconfiguredPolicy
		case 6: return PaperErrorInvalidPolicy

		default: return PaperErrorInternal
	}
}
