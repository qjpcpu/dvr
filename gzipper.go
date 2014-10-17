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
	"io"
	"os"
)

// This token allows us to intercept startup and therefor act as a command
// line "gzip" process. If os.Argv[1] is this value then we simply take
// os.Stdin and gzip it into os.Stdout.
//
// This is necessary since Go's test library doesn't have any way of
// indicating or calling out when all the tests have finished which means
// that it is impossible to close a gzip file.
const InterceptorToken = "dvr_gzipper_token_a9s87d9aish2"

// At startup check the args and intercept if necessary.
func init() {
	initGzipper(os.Args, os.Stdin, os.Stdout, os.Exit)
}

// This function is setup to be tested, hence the awkward footprint.
func initGzipper(args []string, in, out *os.File, exit func(int)) {
	if len(args) != 2 {
		return
	} else if args[1] != InterceptorToken {
		return
	}

	// We are in interceptor mode. Intercept and gzip stdin to stdout.
	compressor, err := gzip.NewWriterLevel(out, 9)
	panicIfError(err)

	// Compress.
	_, err = io.Copy(compressor, in)
	panicIfError(err)

	// Close.
	err = compressor.Close()
	panicIfError(err)

	// Success!
	exit(0)
}
