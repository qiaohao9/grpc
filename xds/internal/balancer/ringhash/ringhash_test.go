/*
 *
 * Copyright 2021 gRPC authors.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 */

package ringhash

import (
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/qiaohao9/grpc/xds/internal/testutils"
)

var (
	cmpOpts = cmp.Options{
		cmp.AllowUnexported(testutils.TestSubConn{}, ringEntry{}, subConn{}),
		cmpopts.IgnoreFields(subConn{}, "mu"),
	}
)

const (
	defaultTestShortTimeout = 10 * time.Millisecond
)
