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

package dvr

import (
	"compress/gzip"
	"os"
	"testing"

	"github.com/liquidgecka/testlib"
)

func TestGzipper(t *testing.T) {
	T := testlib.NewT(t)
	defer T.Finish()

	// Ensure that the bad arg conditions to not work.
	func() {
		forcePanic := func(i int) { panic(i) }
		defer func() {
			err := recover()
			if err != nil {
				T.Fatalf("Unexpected panic: %#v\n", err)
			}
		}()
		initGzipper([]string{"XXX"}, nil, nil, forcePanic)
		initGzipper([]string{"XXX", "YYY"}, nil, nil, forcePanic)
	}()

	// Setup files that we will use for input/output for the function.
	data := []byte("A test string that should always make it through.")
	in := T.TempFile()
	out := T.TempFile()

	// Write the uncompressed data to the input file.
	_, err := in.Write(data)
	T.ExpectSuccess(err)
	_, err = in.Seek(0, 0)
	T.ExpectSuccess(err)

	// call the gzipper initializer.
	func() {
		defer func() {
			err := recover()
			if err != nil {
				T.Fatalf("Unexpected error: %#v\n", err)
			}
		}()
		exited := false
		exit := func(i int) {
			exited = true
		}
		initGzipper([]string{os.Args[0], InterceptorToken}, in, out, exit)
		T.Equal(exited, true)
	}()

	// Read the data back and ensure that it matches what we wrote in.
	fd, err := os.Open(out.Name())
	T.ExpectSuccess(err)
	reader, err := gzip.NewReader(fd)
	T.ExpectSuccess(err)
	var readData []byte = make([]byte, 1024)
	n, err := reader.Read(readData)
	T.ExpectSuccess(err)
	T.Equal(n, len(data))
	T.Equal(readData[0:n], data)
}
