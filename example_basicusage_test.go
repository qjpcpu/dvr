// Copyright 2014 Orchestrate, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//   http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package dvr_test

import (
	"net/http"
	"net/url"
	"testing"

	_ "github.com/orchestrate-io/dvr"
)

// When running this test you can run it in one of three modes:
//
// Pass through: In this mode nothing is recorded or replayed.
//   go test .
//
// Recording: In this mode all HTTP calls are recorded into a file (the default
//            is testdata/archive.dvr, and is control by -dvr.file)
//   go test -dvr.record .
//
// Replay: In this mode all HTTP calls are replayed from the recording file
//         captured in Recording mode above.
//   go test -dvr.replay .
//
// * Note that this function needs the leading underscore removed in order
//   to work in the real world. It exists purely because of the rules around
//   "playable" document examples.
func _TestCallToApi(t *testing.T) {
	var err error
	req := &http.Request{}
	req.URL, err = url.Parse("http://golang.org")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Error from net.http: %s", err)
	} else if resp.StatusCode != 200 {
		t.Fatalf("Bad status code: %d", resp.StatusCode)
	}
}

// For the most basic of use cases you need only ensure that the dvr library
// is included in your test file. Including it with an underscore, like in
// this example, is all that is necessary to get it in place.
func Example_basicUsage() {
	// Though this library can be used outside of golang's testing library
	// its not recommended. This main() exists only to make the example
	// render in godoc.

	// Output:
}
