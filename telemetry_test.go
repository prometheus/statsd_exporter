// Copyright 2013 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"io/ioutil"
	"testing"
)

func TestProcfsWatching(t *testing.T) {
	filename := "/tmp/procfstest"

	d1 := []byte("  sl  local_address                         remote_address                        st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode ref pointer drops\n42429: 00000000000000000000000000000000:1FBD 00000000000000000000000000000000:0000 07 00000000:00000034 00:00000000 00000000     0        0 1233268343 2 ffff881fc5d32ec0 10000\n")
	err := ioutil.WriteFile(filename, d1, 0644)
	if err != nil {
		t.Fatalf("Should be able to write a procfs-like file: %s", err)
	}

	queued, dropped, err := parseProcfsNetFile(filename)
	if err != nil {
		t.Fatalf("Parsing encountered an error: %s", err)
	}
	if dropped != 10000 {
		t.Fatal("Dropped should be 10000")
	}

	if queued != 52 {
		t.Fatal("Queued should be 52", queued)
	}
}
