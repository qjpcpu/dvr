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
	"encoding/base64"
	"fmt"
	"net/url"
)

// This is the type used to store the values, but not the stack from the call
// to BasicAuthObfuscator.
type basicAuthObfuscator struct {
	username string
	password string
}

// This is the Obfuscator function attached to the above.
func (b *basicAuthObfuscator) Obfuscator(rr *RequestResponse) {
	// If the Authorization header is set then we need to replace it with
	// the username/password given above.
	if rr.Request.Header.Get("Authorization") != "" {
		up := fmt.Sprintf("%s:%s", b.username, b.password)
		value := "Basic " + base64.StdEncoding.EncodeToString([]byte(up))
		rr.Request.Header.Del("Authorization")
		rr.Request.Header.Set("Authorization", value)
	}

	// Next we check to see if the URL object contains a User value, if so
	// then it will get replaced with one that has the username and
	// password set to the replacement values.
	if rr.Request.URL.User != nil {
		if b.password == "" {
			rr.Request.URL.User = url.User(b.username)
		} else {
			rr.Request.URL.User = url.UserPassword(
				b.username, b.password)
		}
	}
}

// This function call will return a function that can act as a Obfuscator
// which removes any HTTP Basic Auth credential and replaces it with the
// given arguments. The results of this call can be directly used with the
// Obfuscator variable.
func BasicAuthObfuscator(username, password string) func(*RequestResponse) {
	return (&basicAuthObfuscator{
		username: username,
		password: password,
	}).Obfuscator
}
