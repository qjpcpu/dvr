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
	"archive/tar"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"sync"
)

var (
	// Set to true if we want to capture and save the various HTTP
	// calls the are made during the testing cycle.
	record bool

	// Set to true if we want to read the recordFile requests and serve
	// them rather than allowing requests to be made to the real internet
	// service.
	replay bool

	// If this is true then the user forced the client into pass through
	// mode which disables record and replay.
	passThrough bool

	// This is the file that test recordings will be saved into.
	fileName string

	// If this is set to true then -dvr.replay becomes default if not
	// other flags are provided. If this is falls then the default will be
	// to pass queries through without recording or replaying them
	DefaultReplay bool

	// On the first call to the RoundTripper we ensure that everything is
	// setup and loaded. We only do this once, and only on the very first call.
	isSetup sync.Once

	// The file descriptor of the record file. This will exist in either
	// record or replay mode.
	fd *os.File

	// This is the tar.Writer that is used for writing the request gob's
	// into the file. We also keep a mutex to ensure that we only write
	// one request at a time to the file.
	writer      *tar.Writer
	writerLock  sync.Mutex
	writerCount int
	writerCmd   *exec.Cmd

	// This is the list of object read from the gob file.
	requestList []*RequestResponse
	requestLock sync.Mutex
)

// This is the round tripper that replaced the default round tripper in the
// net/http library. Its stored here so it can be retrieved if lost.
var DefaultRoundTripper http.RoundTripper

// This is the round tripper that we overwrote in net/http. It is preserved
// here in case it needs be recovered.
var OriginalDefaultTransport http.RoundTripper

// Initialize the flags.
func init() {
	flag.BoolVar(&record, "dvr.record", false,
		"Record HTTP calls into -dvr.record_file for use later.")
	flag.BoolVar(&replay, "dvr.replay", false,
		"Replay HTTP calls from -svr.record_file.")
	flag.BoolVar(&passThrough, "dvr.passthrough", false,
		"Allow queries to pass through without being recorded or replayed.")
	flag.StringVar(&fileName, "dvr.file",
		"testdata/archive.dvr",
		"The file that stores recorded HTTP calls.")

	// Replace DefaultTransport!
	OriginalDefaultTransport = http.DefaultTransport
	DefaultRoundTripper = NewRoundTripper(http.DefaultTransport)
	http.DefaultTransport = DefaultRoundTripper
}

// Returns booleans representing the current running mode. If none of the
// returns are true then the library is in pass through mode.
func mode() (rec bool, rep bool) {
	switch {
	case record:
		return true, false
	case replay:
		return false, true
	case passThrough:
		return false, false
	case DefaultReplay:
		return false, true
	default:
		return false, false
	}
}

// Returns true if the DVR library is in recording mode.
func IsRecording() bool {
	b, _ := mode()
	return b
}

// Returns true if the DVR library is in pass through mode.
func IsPassingThrough() bool {
	a, b := mode()
	return !a && !b
}

// Returns true if the DVR library is in replay mode.
func IsReplay() bool {
	_, b := mode()
	return b
}

// A simple error type that wraps all library panics.
type dvrFailure struct {
	Err error
}

// Return the underlying error's message
func (d *dvrFailure) Error() string {
	return d.Err.Error()
}

// This is the channel that the panic routine should be writing to for errors.
// Basically this allows us to mute the error during tests.
var panicOutput io.Writer = os.Stdout

// This function is used when the library can not continue safely. The idea
// is that the user has requested a specific configuration (say replaying
// requests) and that is not possible. We have no way of reporting this
// back to the user in a meaningful way since returning an error might only
// fail a specific test (or worse, cause the test to pass when it shouldn't
// since it expects an error). As such we panic with a verify specific header
// message to let the user know that something has gone wrong.
//
// If err is nil this will do nothing, otherwise it will panic with a
// error message to the user knows what is happening.
func panicIfError(err error) {
	if err == nil {
		return
	}
	fmt.Fprintf(panicOutput, ""+
		"An error was encounteded in the DVR library.\n"+
		"This error will cause potentially inconsistent results from tests\n"+
		"so the entire testing process will be terminated (sorry).\n"+
		"Please correct the error in the message below and re-run the test.\n"+
		"If this is an internal issue please feel free to file a bug report\n"+
		"with the dvr developers at: https://github.com/orchestrate-io/dvr\n\n"+
		"The error encountered is:\n%s\n", err)
	panic(&dvrFailure{Err: err})
}

// This is the roundTripper that will be installed as part of the HTTP
// client interception routines. This is an implementation of
// net/http.RoundTripper.
type roundTripper struct {
	// This is the real http.RoundTripper interface that will be used
	// when not in replay mode. All calls will be passed through to this
	// handler.
	realRoundTripper http.RoundTripper
}

// This creates a new RoundTripper object with the given RoundTripper object
// as its fall back (for pass through and recording modes).
func NewRoundTripper(fallback http.RoundTripper) http.RoundTripper {
	r := new(roundTripper)
	r.realRoundTripper = fallback
	return r
}

// This is the call that is expected to actually perform the HTTP request.
// In our case we can either pass the request through, record it, or return
// the data from a request in the recorded file.
func (r *roundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	rec, rep := mode()
	switch {
	case rec:
		return r.record(req)
	case rep:
		return r.replay(req)
	default:
		return r.realRoundTripper.RoundTrip(req)
	}
}

// Interface to match http.Transport's CancelRequest method.
type httpCancelRequest interface {
	CancelRequest(*http.Request)
}

// This call will cancel an in flight request.. It does nothing in replay
// mode and is passed through if possible in record mode.
func (r *roundTripper) CancelRequest(req *http.Request) {
	if c, ok := r.realRoundTripper.(httpCancelRequest); ok {
		c.CancelRequest(req)
	}
}

// This structure stores information about a Request/Response pairing. It is
// intended to keep the data together for use when matching and such.
type RequestResponse struct {
	// This is the Request object that was passed in to the RoundTripper
	// call. This object will have it's Body field set to nil, with the body
	// being represented in the byte array 'RequestBody'. The error (if any)
	// returned when reading from Body is stored in RequestBodyError.
	Request          *http.Request
	RequestBody      []byte
	RequestBodyError error

	// This is the Response object that was returned to the caller. Note that
	// Body in this field is saved in the ResponseBody field, and the error
	// returned from the server is stored in ResponseBodyError.
	Response          *http.Response
	ResponseBody      []byte
	ResponseBodyError error

	// This is the error returned from the RountTrip() call.
	Error error

	// This stores any user data that is necessary for the Matcher() function.
	UserData interface{}
}
