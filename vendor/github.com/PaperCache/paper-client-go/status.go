/*
 * Copyright (c) Kia Shakiba
 *
 * This source code is licensed under the GNU AGPLv3 license found in the
 * LICENSE file in the root directory of this source tree.
 */

package paperclient

type PaperStatus struct {
	pid uint32

	max_size uint64
	used_size uint64
	num_objects uint64

	rss uint64
	hwm uint64

	total_gets uint64
	total_sets uint64
	total_dels uint64

	miss_ratio float64

	policies []string
	policy string
	is_auto_policy bool

	uptime uint64
}

func statusFromReader(reader *sheetReader) (*PaperStatus, error) {
	pid, err := reader.readU32()

	if err != nil {
		return nil, err
	}

	max_size, err := reader.readU64()

	if err != nil {
		return nil, err
	}

	used_size, err := reader.readU64()

	if err != nil {
		return nil, err
	}

	num_objects, err := reader.readU64()

	if err != nil {
		return nil, err
	}

	rss, err := reader.readU64()

	if err != nil {
		return nil, err
	}

	hwm, err := reader.readU64()

	if err != nil {
		return nil, err
	}

	total_gets, err := reader.readU64()

	if err != nil {
		return nil, err
	}

	total_sets, err := reader.readU64()

	if err != nil {
		return nil, err
	}

	total_dels, err := reader.readU64()

	if err != nil {
		return nil, err
	}

	miss_ratio, err := reader.readF64()

	if err != nil {
		return nil, err
	}

	num_policies, err := reader.readU32()

	if err != nil {
		return nil, err
	}

	var policies []string

	for i := uint32(0); i < num_policies; i++ {
		policy, err := reader.readString()

		if err != nil {
			return nil, err
		}

		policies = append(policies, policy)
	}

	policy, err := reader.readString()

	if err != nil {
		return nil, err
	}

	is_auto_policy, err := reader.readBool()

	if err != nil {
		return nil, err
	}

	uptime, err := reader.readU64()

	if err != nil {
		return nil, err
	}

	status := PaperStatus {
		pid,

		max_size,
		used_size,
		num_objects,

		rss,
		hwm,

		total_gets,
		total_sets,
		total_dels,

		miss_ratio,

		policies,
		policy,
		is_auto_policy,

		uptime,
	}

	return &status, nil
}
