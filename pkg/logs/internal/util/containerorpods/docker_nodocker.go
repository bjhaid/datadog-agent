// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !docker
// +build !docker

package containerorpods

import "time"

// dockerReady checks if the docker service is ready, returning a duration after
// which it should be re-checked if not.
func dockerReady() (bool, time.Duration) {
	// no docker support -> definitely not ready
	return false, 1000 * time.Hour
}
