# DVR

DVR is a library for capturing and replaying HTTP calls for golang tests.

## Installation

```bash
go get github.com/qjpcpu/dvr
```

## Usage

The functionality is primarily documented in the [godoc documentation](http://godoc.org/github.com/orchestrate-io/dvr), however
a high level is provided here.

This library is intended to allow off line testing of remote API calls. It can
be run against the test once in a "record" mode which captures all the queries
and stores them in a file. A second test run can then be put into "replay"
mode which will match incoming queries against those in the stored file.

The inspiration for this library came from the
[Python VCR library.](https://github.com/kevin1024/vcrpy). Though the concept
is the same several key components have been change to make it more
idiomatic golang.

## Testing

[![Continuous Integration](https://secure.travis-ci.org/orchestrate-io/dvr.svg?branch=master)](http://travis-ci.org/orchestrate-io/dvr)
[![Documentation](http://godoc.org/github.com/orchestrate-io/dvr?status.png)](http://godoc.org/github.com/orchestrate-io/dvr)
[![Coverage](https://img.shields.io/coveralls/orchestrate-io/dvr.svg)](https://coveralls.io/r/orchestrate-io/dvr)

## Contribution

Pull requests will be reviewed and accepted via github.com, and issues will be
worked on as time permits. New feature requests should be filed as an issue
against the github repo.

## License (Apache 2)

Copyright 2014 Orchestrate, Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

  http://www.apache.org/licenses/LICENSE-2.0

  Unless required by applicable law or agreed to in writing, software
  distributed under the License is distributed on an "AS IS" BASIS,
  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
  See the License for the specific language governing permissions and
  limitations under the License.
