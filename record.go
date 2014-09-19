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
	"bytes"
	"encoding/gob"
	"fmt"
	"io"
	"net/http"
	"os"
)

// If this value is anything other than nil it will be called on a copy
// of the passed in *http.Request and a copy of the returned
// *http.Response object. Any mutations to these objects will be stored in the
// archive, but NOT be altered in the recording unit test. The intention of
// this function is to allow obfuscation of data you do not want recorded.
//
// An example usage of this function is to change the password used to
// authenticate against a web service in order to allow any user to
// run the test. See the "RequestObfuscation" example for details.
var Obfuscator func(*RequestResponse)

// This function setups up the rountTripper in recording mode. This will open
// the output file as a zip stream so each follow up call can write an
// individual call to the output.
func (r *roundTripper) recordSetup() {
	var err error

	// Open the zip file for writing.
	r.fd, err = os.OpenFile(fileName, os.O_WRONLY|os.O_CREATE|os.O_TRUNC,
		os.FileMode(0755))
	panicIfError(err)

	// Create the new zip writer that will store our results.
	r.writer = tar.NewWriter(r.fd)
}

// This function is called if the testing library is in recording mode.
// In recording mode we will automatically catch the data from all HTTP
// requests and save them so they can be replayed later.
func (r *roundTripper) record(req *http.Request) (*http.Response, error) {
	// Ensure that recording is setup.
	r.isSetup.Do(r.recordSetup)

	// The structure that saves all of our transmitted data.
	q := &gobQuery{}
	q.Request = newGobRequest(req)

	if req.Body != nil {
		// Read the body into a buffer for us to save.
		buffer := &bytes.Buffer{}
		_, q.Request.Error.Error = io.Copy(buffer, req.Body)
		q.Request.Body = buffer.Bytes()
		req.Body = &bodyWriter{
			offset: 0,
			data:   q.Request.Body,
			err:    q.Request.Error.Error,
		}
	}

	// Use the underlying round tripper to actually complete the request.
	resp, realErr := r.realRoundTripper.RoundTrip(req)

	// Save the data we were returned.
	q.Error.Error = realErr
	q.Response = newGobResponse(resp)

	// Encode the body if necessary.
	if resp != nil && resp.Body != nil {
		buffer := &bytes.Buffer{}
		_, q.Response.Error.Error = io.Copy(buffer, resp.Body)
		q.Response.Body = buffer.Bytes()
		resp.Body = &bodyWriter{
			offset: 0,
			data:   q.Response.Body,
			err:    q.Response.Error.Error,
		}
	}

	// Gob encode the request into a byte buffer so that we know the size.
	buffer := &bytes.Buffer{}
	encoder := gob.NewEncoder(buffer)
	panicIfError(encoder.Encode(q))

	// If an Obfuscator is present then we need to do a bunch of extra work.
	f := Obfuscator
	if f != nil {
		// First we decode the encoded object back over its self. This allows
		// us to know that we have copies of all data, so mutation won't impact
		// the Request or Response we return from this function.
		decoder := gob.NewDecoder(buffer)
		panicIfError(decoder.Decode(q))

		// Convert this to a RequestResponse object, then allow the Obfuscator
		// to mutate it in what ever way it sees fit.
		rr := q.RequestResponse()
		f(rr)

		// Now we need to re-encode the object back into a gobQuery.
		q.Request = newGobRequest(rr.Request)
		if q.Request != nil {
			q.Request.Body = rr.RequestBody
			q.Request.Error.Error = rr.RequestBodyError
		}
		q.Response = newGobResponse(rr.Response)
		if q.Response != nil {
			q.Response.Body = rr.ResponseBody
			q.Response.Error.Error = rr.ResponseBodyError
		}

		// And lastly we encode this back into the buffer.
		buffer = &bytes.Buffer{}
		encoder := gob.NewEncoder(buffer)
		panicIfError(encoder.Encode(q))
	}

	// Lock the writer output so that we don't have race conditions adding
	// to the zip file.
	r.writerLock.Lock()
	defer r.writerLock.Unlock()

	// Add a "Header" for the nea request. Headers are functionally virtual
	// files in the tar stream.
	header := &tar.Header{
		Name: fmt.Sprintf("%d", r.writerCount),
		Size: int64(buffer.Len()),
	}
	r.writerCount = r.writerCount + 1
	panicIfError(r.writer.WriteHeader(header))

	// Write the buffer into the tar stream.
	_, err := io.Copy(r.writer, buffer)
	panicIfError(err)

	// Next we need to ensure that the full object is flushed to the tar
	// stream. We do this by flushing the writer and then syncing the
	// underlying file descriptor.. This is necessary since we don't know
	// when the program is going to exit.
	panicIfError(r.writer.Flush())
	panicIfError(r.fd.Sync())

	// Success!
	return resp, realErr
}
