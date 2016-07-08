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
	"compress/gzip"
	"encoding/binary"
	"encoding/gob"
	"fmt"
	"io"
	"net/http"
	"os"
	"reflect"
	"strings"
)

// This function is used by the replay component of this library to determine
// if an incoming request matches a request from the archive. If this function
// returns true then the requests are deemed to have "matched". Note that
// mutation of either object (other than the UserData field) will likely
// result in a panic or crash. The left object will not have Response* fields
// populated.
//
// The default matcher will match a request if it Request's URL, Body, Headers
// and Trailers are all the same.
var Matcher func(left, right *RequestResponse) bool

// This is the default implementation of Matcher()
func matcher(left, right *RequestResponse) bool {
	// For the default match we use UserData purely as a boolean where "nil"
	// means "unseen" and "non nil" means seen.
	if right == nil || left == nil {
		return false
	} else if right.UserData != nil {
		return false
	} else if right.Request == nil || left.Request == nil {
		return false
	}

	lreq := left.Request
	rreq := right.Request

	// Case 1: URL elements match.
	if lreq.URL == nil {
		return false
	} else if lreq.URL.Scheme != rreq.URL.Scheme {
		return false
	} else if lreq.URL.Opaque != rreq.URL.Opaque {
		return false
	} else if lreq.URL.Host != rreq.URL.Host {
		return false
	} else if lreq.URL.Path != rreq.URL.Path {
		return false
	} else if lreq.URL.RawQuery != rreq.URL.RawQuery {
		return false
	} else if lreq.URL.Fragment != rreq.URL.Fragment {
		return false
	}

	// Case 1: URL.User
	if lreq.URL.User != nil && rreq.URL.User == nil {
		return false
	} else if lreq.URL.User == nil && rreq.URL.User != nil {
		return false
	} else if lreq.URL.User != nil {
		if lreq.URL.User.String() != rreq.URL.User.String() {
			return false
		}
	}

	// Case 2: Request Body match.
	if bytes.Compare(left.RequestBody, right.RequestBody) != 0 {
		return false
	}

	// Case 3: Headers and Trailers match.
	if !reflect.DeepEqual(lreq.Header, rreq.Header) {
		return false
	}
	if !reflect.DeepEqual(lreq.Trailer, rreq.Trailer) {
		return false
	}

	right.UserData = right
	return true
}

// the contents of the request are matched to ensure that the request is
// appropriate.
func (r *roundTripper) replaySetup() {
	// Open the tar file for reading.
	fd, err := os.OpenFile(fileName, os.O_RDONLY, os.FileMode(755))
	panicIfError(err)

	// Read the file version in.
	version := uint32(0)
	err = binary.Read(fd, binary.BigEndian, &version)
	panicIfError(err)
	if version != 1 {
		panic(fmt.Errorf("Unknown version: %d", version))
	}

	// Make a gzip reader.
	gzipReader, err := gzip.NewReader(fd)
	panicIfError(err)

	// Create the tar reader and the list used to store the results.
	reader := tar.NewReader(gzipReader)
	requestList = make([]*RequestResponse, 0, 100)

	// While the archive has elements in it we loop through decoding them
	// and adding them to a list.
	for {
		// Read the next header.
		if _, err := reader.Next(); err == io.EOF {
			break
		} else {
			panicIfError(err)
		}

		// Create a decoder and a list for us to store the results in.
		gobDecoder := gob.NewDecoder(reader)

		// Read the results from the stream.
		gobQuery := gobQuery{}
		panicIfError(gobDecoder.Decode(&gobQuery))

		// Add the query to the list.
		requestList = append(requestList, gobQuery.RequestResponse())
	}

	// Close the file.
	panicIfError(fd.Close())
}

// This is the RoundTrip() call when we are in replay mode.
func (r *roundTripper) replay(req *http.Request) (*http.Response, error) {
	// Ensure that the replay system is setup.
	isSetup.Do(r.replaySetup)

	// Read the body into a buffer.
	buffer := &bytes.Buffer{}
	var reqErr error
	if req.Body != nil {
		_, reqErr = io.Copy(buffer, req.Body)
	}

	// Since this function deals with the requestList we need to lock.
	requestLock.Lock()
	defer requestLock.Unlock()

	// Figure out which match function to use.
	f := Matcher
	if f == nil {
		f = matcher
	}

	// Walk through the objects in our archive list and see if any of them
	// match the incoming request.
	rrSource := &RequestResponse{
		Request:          req,
		RequestBody:      buffer.Bytes(),
		RequestBodyError: reqErr,
	}

	var rrMatch *RequestResponse
	for _, rr := range requestList {
		// copy requestresponse obj, so it can be modified in matcher
		copyrr := new(RequestResponse)
		*copyrr = *rr
		copyrr.Response = new(http.Response)
		*copyrr.Response = *rr.Response
		// copy body
		copyrr.RequestBody = make([]byte, len(rr.RequestBody))
		// copy header
		copyrr.Response.Header = http.Header{}
		for k, vals := range rr.Response.Header {
			for _, v := range vals {
				copyrr.Response.Header.Add(k, v)
			}
		}
		copy(copyrr.RequestBody, rr.RequestBody)
		if f(rrSource, copyrr) {
			rrMatch = copyrr
			break
		}
	}
	if rrMatch == nil {
		messageLines := []string{
			"Matcher didn't match any execeted queries.\n",
			"Details of the failed request:",
			"",
			fmt.Sprintf("URL: %s", req.URL.String()),
			fmt.Sprintf("Method: %s", req.Method),
		}
		if len(req.Header) > 0 {
			messageLines = append(messageLines, "\nHeaders:")
			for key, value := range req.Header {
				messageLines = append(messageLines,
					fmt.Sprintf("    %s: %s", key, strings.Join(value, ", ")))
			}
		}
		if len(req.Trailer) > 0 {
			messageLines = append(messageLines, "\nTrailers:")
			for key, value := range req.Trailer {
				messageLines = append(messageLines,
					fmt.Sprintf("    %s: %s", key, strings.Join(value, ", ")))
			}
		}
		if len(buffer.Bytes()) > 0 {
			// This block is written a little funky in order to make testing
			// easier since it doesn't if/else as much.
			messageLines = append(messageLines, "Body:")
			length := len(buffer.Bytes())
			warning := ""
			if length > 512 {
				length = 512
				warning = "... (content truncated by dvr)"
			}
			messageLines = append(messageLines,
				string(buffer.Bytes()[:length])+warning)
		}
		panicIfError(fmt.Errorf(strings.Join(messageLines, "\n")))
	}

	// Check to see if the response was an error when recorded.
	if rrMatch.Response == nil {
		return nil, rrMatch.Error
	}

	// Setup our response object.
	resp := new(http.Response)
	*resp = *rrMatch.Response
	resp.Request = req

	// Lastly we need to setup a bodyWriter for the Body. This will allow the
	// client to read the body we captured and it will return the error we
	// captured (if any) rather than EOF.
	resp.Body = &bodyWriter{
		data: rrMatch.ResponseBody,
		err:  rrMatch.ResponseBodyError,
	}

	// And lastly we return the response.
	return resp, rrMatch.Error
}

//
// bodyWriter
//

// This structure is used for writing the output from the server back to the
// caller. It repeats the bytes we recorded and returns the error we initially
// captured.
type bodyWriter struct {
	offset int
	data   []byte
	err    error
}

// io.Reader
func (b *bodyWriter) Read(input []byte) (int, error) {
	if b.offset >= len(b.data) {
		if b.err == nil {
			return 0, io.EOF
		} else {
			return 0, b.err
		}
	}
	n := copy(input, b.data[b.offset:])
	b.offset += n
	return n, nil
}

// io.Closer
func (b *bodyWriter) Close() error {
	return nil
}
