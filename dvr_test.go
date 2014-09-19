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
	"bytes"
	"crypto/md5"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/liquidgecka/testlib"
)

// This is the request handler used with the http server.
type httpHandler struct {
}

// Based on the URL requested we respond with a pre-canned response type.
func (h *httpHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Add a 'Date' header so that we don't have time based race conditions.
	w.Header().Add("Date", "Mon, 2 Mar 2001 01:02:03 GMT")

	switch r.URL.Path {

	// The following blocks are all very simple response types used for
	// quick checks of the Status field in Response.
	case "/201":
		w.WriteHeader(201)
	case "/220":
		w.WriteHeader(220)
	case "/404":
		w.WriteHeader(404)
	case "/501":
		w.WriteHeader(501)
	case "/540":
		w.WriteHeader(540)

	// The following call will add a response header.
	case "/resp_header":
		w.Header().Add("X-Test-Header", "X Y Z")
		w.WriteHeader(200)

	// The following call will read the client request body and
	// write back the MD5 sum of its contents.
	case "/body":
		w.WriteHeader(200)
		data, err := ioutil.ReadAll(r.Body)
		if err != nil {
			panic(err)
		}
		h := md5.New()
		str := fmt.Sprintf("%x\n", h.Sum(data))
		if _, err := w.Write([]byte(str)); err != nil {
			panic(err)
		}

	// And lastly is the case where the server commits an error by
	// closing the socket before sending a reply.
	case "/error":
		if conn, _, err := w.(http.Hijacker).Hijack(); err != nil {
			panic(err)
		} else if err := conn.Close(); err != nil {
			panic(err)
		}

	// The user sent us a bad request.. This is a testing error.
	default:
		panic(fmt.Errorf("Unknown request to %s: %#v", r.URL.Path, r))
	}
}

// Tuns function starts an HTTP server on localhost on a random port. This
// server will run in a goroutine until the listener returned here gets
// closed. Its in the best interest of the caller to defer a close to ensure
// that the http server goroutine gets shut down properly.
func runHttpServer(T *testlib.T) net.Listener {
	listener, err := net.Listen("tcp", ":0")
	T.ExpectSuccess(err)
	T.NotEqual(listener, nil)

	// Setup the HTTP server.
	s := &http.Server{
		Handler:        &httpHandler{},
		ReadTimeout:    1 * time.Second,
		WriteTimeout:   1 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}
	go func() {
		s.Serve(listener)
	}()

	// Return the listener object.
	return listener
}

// This structure saves the results of a query.
type savedQuery struct {
	Request           *http.Request
	RequestBody       []byte
	RequestBodyError  error
	Response          *http.Response
	ResponseBody      []byte
	ResponseBodyError error
	Error             error
}

// Issues a single request against the given RoundTripper and returns the
// results as a RecordedRequest object. If any error is encountered then
// this will Fatal the given testlib.T object.

// Runs through all of the known tests and collects the results from each
// one to be returned in the savedQuery slice.
func runTests(
	T *testlib.T, rt http.RoundTripper, addr, username, password string,
) []*savedQuery {
	makeRequest := func(addr, path, method, body string) *savedQuery {
		sq := new(savedQuery)
		var err error

		// Setup the Request
		sq.Request = new(http.Request)
		sq.Request.Method = method
		sq.Request.URL, err = url.Parse(fmt.Sprintf("http://%s%s", addr, path))
		T.ExpectSuccess(err)
		sq.Request.Header = make(http.Header, 1)
		sq.Request.Header.Add("X-Client-Header", "test")
		sq.Request.ContentLength = int64(len(body))
		if username != "" || password != "" {
			sq.Request.SetBasicAuth(username, password)
		}

		// Setup the Body
		bodyBuffer := &bytesBufferCloser{}
		_, err = bodyBuffer.Write([]byte(body))
		T.ExpectSuccess(err)
		sq.Request.Body = bodyBuffer
		sq.RequestBody = []byte(body)

		// Setup the HTTP client.
		client := &http.Client{}
		client.Transport = rt
		resp, err := client.Do(sq.Request)

		// Copy the results into the rr object.
		sq.Response = resp
		sq.Error = err
		if resp != nil {
			buffer := bytes.NewBuffer(nil)
			_, sq.ResponseBodyError = io.Copy(buffer, resp.Body)
			sq.ResponseBody = buffer.Bytes()
			sq.Response.Body = nil
		}

		// Nil out the request body.
		sq.Request.Body = nil

		// Success
		return sq
	}

	// Makes a request with an erroring client body.
	makeErrored := func(addr, path string) *savedQuery {
		sq := new(savedQuery)
		var err error

		// Setup the Request
		sq.Request = new(http.Request)
		sq.Request.Method = "GET"
		sq.Request.URL, err = url.Parse(fmt.Sprintf("http://%s%s", addr, path))
		T.ExpectSuccess(err)
		sq.Request.Header = make(http.Header, 1)
		sq.Request.Header.Add("X-Client-Header", "test")

		// Setup the Body
		bodyReader, bodyWriter := io.Pipe()
		sq.Request.Body = bodyReader
		sq.RequestBody = []byte("testfailure")
		sq.RequestBodyError = fmt.Errorf("expected")
		go func() {
			bodyWriter.Write(sq.RequestBody)
			bodyWriter.CloseWithError(sq.RequestBodyError)
		}()

		// Setup the HTTP client.
		client := &http.Client{}
		client.Transport = rt
		resp, err := client.Do(sq.Request)

		// Copy the results into the rr object.
		sq.Response = resp
		sq.Error = err
		if resp != nil {
			buffer := bytes.NewBuffer(nil)
			_, sq.ResponseBodyError = io.Copy(buffer, resp.Body)
			sq.ResponseBody = buffer.Bytes()
			sq.Response.Body = nil
		}

		// Nil out the request body.
		sq.Request.Body = nil

		// Success
		return sq
	}

	r := make([]*savedQuery, 0, 100)

	// Start with the simple status requests.
	for _, method := range []string{"GET", "POST", "HEAD"} {
		for _, path := range []string{"/201", "/220", "/404", "/501", "/540"} {
			r = append(r, makeRequest(addr, path, method, "body1"))
			r = append(r, makeRequest(addr, path, method, "body2"))
		}
	}

	for _, path := range []string{"/resp_header", "/body", "/error"} {
		r = append(r, makeRequest(addr, path, "GET", "body1"))
		r = append(r, makeRequest(addr, path, "GET", "body2"))
	}

	// Test the error on Request.Body.Read()
	r = append(r, makeErrored(addr, "/"))

	// Return the results.
	return r
}

func TestFullCycle(t *testing.T) {
	// Reset default settings,
	defer func() {
		record = false
		replay = false
		passThrough = false
		DefaultReplay = false
		fileName = "testdata/archive.dvr"
	}()
	T := testlib.NewT(t)
	defer T.Finish()

	// Start an HTTP server and ensure that the listener is closed when
	// the test finishes. This will shut down the server.
	listener := runHttpServer(T)
	defer listener.Close()
	addr := listener.Addr().String()

	//
	// Pass Through Tests
	//

	// Fun a bunch of queries against the server directly (no dvr at all) as
	// we as with a pass through dvr Transport in place. Once we are done
	// we compare all of the results of the two tests against each other.
	record = false
	replay = false
	directResponses := runTests(T, OriginalDefaultTransport, addr, "", "")
	passthroughTripper := &roundTripper{
		realRoundTripper: OriginalDefaultTransport,
	}
	T.Equal(runTests(T, passthroughTripper, addr, "", ""), directResponses)

	//
	// Record Tests
	//

	// Next we setup a temp file and record the same tests as above into it.
	// We setup a recording RoundTripper and run the same tests as above into
	// it. Once done we compare the results against the direct responses.
	fileName = T.TempFile().Name()
	record = true
	replay = false
	recordTripper := &roundTripper{realRoundTripper: OriginalDefaultTransport}
	T.Equal(runTests(T, recordTripper, addr, "", ""), directResponses)

	//
	// Replay tests
	//

	// Now we need to validate the file by attempting to "replay" it. To do
	// this we create a new roundTripper with the replay option set to the file
	// we just created and reset the flags.
	record = false
	replay = true
	replayTripper := &roundTripper{realRoundTripper: OriginalDefaultTransport}
	T.Equal(runTests(T, replayTripper, addr, "", ""), directResponses)

	// Make sure that a new, completely unknown requests results in a panic
	// with the right types and such.
	var err error
	req := &http.Request{}
	req.URL, err = url.Parse(fmt.Sprintf("http://%s/wtf", addr))
	T.ExpectSuccess(err)
	client := &http.Client{}
	client.Transport = replayTripper
	func() {
		defer func() {
			err := recover()
			if err == nil {
				T.Fatalf("An expected panic didn't happen!")
			} else if _, ok := err.(*dvrFailure); !ok {
				panic(err)
			}
		}()
		panicOutput = ioutil.Discard
		resp, err := client.Do(req)
		T.Fatalf("The previous call should have paniced. It Returned: %#v %#v",
			resp, err)
	}()

	//
	// Obfuscator
	//

	// Setup a BasicAuthOfbuscator
	Obfuscator = BasicAuthObfuscator("user2", "pass2")

	// Now setup a recorder that will recurd all requests with one username
	// and password.
	fileName = T.TempFile().Name()
	record = true
	replay = false
	recordTripper = &roundTripper{realRoundTripper: OriginalDefaultTransport}
	runTests(T, recordTripper, addr, "user1", "pass1")

	// Now we attempt to play the results back, only using a replay session,
	// only this time we use a different username and password but it should
	// still work. Note that we can not compare them to the direct results
	// since they will contain auth headers and such. Instead we fail if a panic
	// is raised.
	record = false
	replay = true
	replayTripper = &roundTripper{realRoundTripper: OriginalDefaultTransport}
	func() {
		defer func() {
			err := recover()
			if err == nil {
				return
			}
			T.Fatalf("Unexpected panic: %s", err)
		}()
		runTests(T, replayTripper, addr, "user2", "pass2")
	}()
}

func TestDvrFailure_Error(t *testing.T) {
	T := testlib.NewT(t)
	defer T.Finish()
	e := dvrFailure{Err: fmt.Errorf("Expected")}
	T.Equal(e.Error(), "Expected")
}

func TestRoundTripper_CancelRequest(t *testing.T) {
	r := roundTripper{realRoundTripper: OriginalDefaultTransport}
	r.CancelRequest(nil)
}

func TestIsReplay(t *testing.T) {
	T := testlib.NewT(t)
	defer T.Finish()
	record = false
	replay = true
	T.Equal(IsReplay(), true)
	record = true
	replay = false
	T.Equal(IsReplay(), false)
	record = false
	replay = false
}

func TestIsRecording(t *testing.T) {
	T := testlib.NewT(t)
	defer T.Finish()
	record = true
	T.Equal(IsRecording(), true)
	record = false
	T.Equal(IsRecording(), false)
	record = false
}

func TestIsPassingThrough(t *testing.T) {
	T := testlib.NewT(t)
	defer T.Finish()
	record = false
	replay = false
	T.Equal(IsPassingThrough(), true)
	replay = true
	T.Equal(IsPassingThrough(), false)
	replay = false
}

func TestIsMode(t *testing.T) {
	T := testlib.NewT(t)
	defer T.Finish()
	test := func(rec, repl, pass, def, ex1, ex2 bool) {
		record = rec
		replay = repl
		passThrough = pass
		DefaultReplay = def
		got1, got2 := mode()
		T.Equal(got1, ex1)
		T.Equal(got2, ex2)
	}

	// Recording mode.
	test(true, false, false, false, true, false)
	test(true, false, false, true, true, false)
	test(true, false, true, false, true, false)
	test(true, false, true, true, true, false)
	test(true, true, false, false, true, false)
	test(true, true, false, true, true, false)
	test(true, true, true, false, true, false)
	test(true, true, true, true, true, false)

	// Replay mode.
	test(false, true, false, false, false, true)
	test(false, true, false, true, false, true)
	test(false, true, true, false, false, true)
	test(false, true, true, true, false, true)

	// Passthrough (forced)
	test(false, false, true, false, false, false)
	test(false, false, true, true, false, false)

	// Default Replay
	test(false, false, false, true, false, true)

	// Default
	test(false, false, false, false, false, false)

	// Reset the defaults/
	record = false
	replay = false
	passThrough = false
	DefaultReplay = false
}

func TestObfuscator(t *testing.T) {
	T := testlib.NewT(t)
	T.Finish()

	b := &basicAuthObfuscator{
		username: "user1",
		password: "pass1",
	}
	rr := &RequestResponse{
		Request: &http.Request{
			URL: &url.URL{
				User: url.UserPassword("user2", "pass2"),
			},
			Header: http.Header(map[string][]string{
				"Authorization": []string{"test"},
			}),
		},
	}
	b.Obfuscator(rr)

	T.Equal(rr.Request.URL.User.String(), "user1:pass1")
	T.Equal(rr.Request.Header.Get("Authorization"), "Basic dXNlcjE6cGFzczE=")

	b.password = ""
	b.Obfuscator(rr)
	T.Equal(rr.Request.URL.User.String(), "user1")
	T.Equal(rr.Request.Header.Get("Authorization"), "Basic dXNlcjE6")
}
