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

// Package dvr attempts to make testing far easier by allowing calls to remote
// HTTP services to be captured and replayed when running the tests a second
// time.
//
// In recording mode (-dvr.record) each request will be captured and recorded
// to a file (-dvr.file, which defaults to testdata/archive.dvr). In replay
// mode each request will be matched against of the requests in the archive.
// This ensures that a unit test can remove all dependencies on remote services
// while running, which is ideal for most testing environments.
//
// Note that this library works be replaying net.http's DefaultTransport
// with one that will intercept queries. If you are using a custom client,
// or replacing the http.DefaultTransport you may need to sub a RoundTripper
// from this package in place.
//
// All common error types will be preserved and returned via the archive,
// however some types can not be restored due to the way that gob works. In
// these cases an error will be returned that satisfies the error interface
// but it will be a different type.
//
// When in replay mode requests are matched if all of the following are the
// same: URL, Body, Headers, Trailers. If all of these elements are the
// same as a recorded request then the response will be returned to the
// client. If no request matches then the test will panic since re-requesting
// may cause all sorts of issues. If this matching strategy is not sufficient
// then you can make value Match() contain a function that can parse two
// requests and establish if they are the same.
//
// This library is intended to be user during unit testing so much of its
// design is wrapped around this, and while it can be used outside of unit
// tests it is strongly not recommended.
package dvr
