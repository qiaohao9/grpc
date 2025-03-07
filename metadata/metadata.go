/*
 *
 * Copyright 2014 gRPC authors.
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

// Package metadata define the structure of the metadata supported by gRPC library.
// Please refer to https://github.com/grpc/grpc/blob/master/doc/PROTOCOL-HTTP2.md
// for more information about custom-metadata.
package metadata // import "github.com/qiaohao9/grpc/metadata"

import (
	"context"
	"fmt"
	"strings"
)

// DecodeKeyValue returns k, v, nil.
//
// Deprecated: use k and v directly instead.
func DecodeKeyValue(k, v string) (string, string, error) {
	return k, v, nil
}

// MD is a mapping from metadata keys to values. Users should use the following
// two convenience functions New and Pairs to generate MD.
type MD map[string][]string

// New creates an MD from a given key-value map.
//
// Only the following ASCII characters are allowed in keys:
//  - digits: 0-9
//  - uppercase letters: A-Z (normalized to lower)
//  - lowercase letters: a-z
//  - special characters: -_.
// Uppercase letters are automatically converted to lowercase.
//
// Keys beginning with "grpc-" are reserved for grpc-internal use only and may
// result in errors if set in metadata.
func New(m map[string]string) MD {
	md := MD{}
	for k, val := range m {
		key := strings.ToLower(k)
		md[key] = append(md[key], val)
	}
	return md
}

// Pairs returns an MD formed by the mapping of key, value ...
// Pairs panics if len(kv) is odd.
//
// Only the following ASCII characters are allowed in keys:
//  - digits: 0-9
//  - uppercase letters: A-Z (normalized to lower)
//  - lowercase letters: a-z
//  - special characters: -_.
// Uppercase letters are automatically converted to lowercase.
//
// Keys beginning with "grpc-" are reserved for grpc-internal use only and may
// result in errors if set in metadata.
func Pairs(kv ...string) MD {
	if len(kv)%2 == 1 {
		panic(fmt.Sprintf("metadata: Pairs got the odd number of input pairs for metadata: %d", len(kv)))
	}
	md := MD{}
	for i := 0; i < len(kv); i += 2 {
		key := strings.ToLower(kv[i])
		md[key] = append(md[key], kv[i+1])
	}
	return md
}

// Len returns the number of items in md.
func (md MD) Len() int {
	return len(md)
}

// Copy returns a copy of md.
func (md MD) Copy() MD {
	return Join(md)
}

// Get obtains the values for a given key.
//
// k is converted to lowercase before searching in md.
func (md MD) Get(k string) []string {
	k = strings.ToLower(k)
	return md[k]
}

// Set sets the value of a given key with a slice of values.
//
// k is converted to lowercase before storing in md.
func (md MD) Set(k string, vals ...string) {
	if len(vals) == 0 {
		return
	}
	k = strings.ToLower(k)
	md[k] = vals
}

// Append adds the values to key k, not overwriting what was already stored at
// that key.
//
// k is converted to lowercase before storing in md.
func (md MD) Append(k string, vals ...string) {
	if len(vals) == 0 {
		return
	}
	k = strings.ToLower(k)
	md[k] = append(md[k], vals...)
}

// Delete removes the values for a given key k which is converted to lowercase
// before removing it from md.
func (md MD) Delete(k string) {
	k = strings.ToLower(k)
	delete(md, k)
}

// Join joins any number of mds into a single MD.
//
// The order of values for each key is determined by the order in which the mds
// containing those values are presented to Join.
func Join(mds ...MD) MD {
	out := MD{}
	for _, md := range mds {
		for k, v := range md {
			out[k] = append(out[k], v...)
		}
	}
	return out
}

type mdIncomingKey struct{}
type mdOutgoingKey struct{}

// NewIncomingContext creates a new context with incoming md attached.
func NewIncomingContext(ctx context.Context, md MD) context.Context {
	return context.WithValue(ctx, mdIncomingKey{}, md)
}

// NewOutgoingContext creates a new context with outgoing md attached. If used
// in conjunction with AppendToOutgoingContext, NewOutgoingContext will
// overwrite any previously-appended metadata.
func NewOutgoingContext(ctx context.Context, md MD) context.Context {
	return context.WithValue(ctx, mdOutgoingKey{}, rawMD{md: md})
}

// AppendToOutgoingContext returns a new context with the provided kv merged
// with any existing metadata in the context. Please refer to the documentation
// of Pairs for a description of kv.
func AppendToOutgoingContext(ctx context.Context, kv ...string) context.Context {
	if len(kv)%2 == 1 {
		panic(fmt.Sprintf("metadata: AppendToOutgoingContext got an odd number of input pairs for metadata: %d", len(kv)))
	}
	md, _ := ctx.Value(mdOutgoingKey{}).(rawMD)
	added := make([][]string, len(md.added)+1)
	copy(added, md.added)
	added[len(added)-1] = make([]string, len(kv))
	copy(added[len(added)-1], kv)
	return context.WithValue(ctx, mdOutgoingKey{}, rawMD{md: md.md, added: added})
}

// FromIncomingContext returns the incoming metadata in ctx if it exists.
//
// All keys in the returned MD are lowercase.
func FromIncomingContext(ctx context.Context) (MD, bool) {
	md, ok := ctx.Value(mdIncomingKey{}).(MD)
	if !ok {
		return nil, false
	}
	out := MD{}
	for k, v := range md {
		// We need to manually convert all keys to lower case, because MD is a
		// map, and there's no guarantee that the MD attached to the context is
		// created using our helper functions.
		key := strings.ToLower(k)
		out[key] = v
	}
	return out, true
}

// FromOutgoingContextRaw returns the un-merged, intermediary contents of rawMD.
//
// Remember to perform strings.ToLower on the keys, for both the returned MD (MD
// is a map, there's no guarantee it's created using our helper functions) and
// the extra kv pairs (AppendToOutgoingContext doesn't turn them into
// lowercase).
//
// This is intended for gRPC-internal use ONLY. Users should use
// FromOutgoingContext instead.
func FromOutgoingContextRaw(ctx context.Context) (MD, [][]string, bool) {
	raw, ok := ctx.Value(mdOutgoingKey{}).(rawMD)
	if !ok {
		return nil, nil, false
	}

	return raw.md, raw.added, true
}

// FromOutgoingContext returns the outgoing metadata in ctx if it exists.
//
// All keys in the returned MD are lowercase.
func FromOutgoingContext(ctx context.Context) (MD, bool) {
	raw, ok := ctx.Value(mdOutgoingKey{}).(rawMD)
	if !ok {
		return nil, false
	}

	out := MD{}
	for k, v := range raw.md {
		// We need to manually convert all keys to lower case, because MD is a
		// map, and there's no guarantee that the MD attached to the context is
		// created using our helper functions.
		key := strings.ToLower(k)
		out[key] = v
	}
	for _, added := range raw.added {
		if len(added)%2 == 1 {
			panic(fmt.Sprintf("metadata: FromOutgoingContext got an odd number of input pairs for metadata: %d", len(added)))
		}

		for i := 0; i < len(added); i += 2 {
			key := strings.ToLower(added[i])
			out[key] = append(out[key], added[i+1])
		}
	}
	return out, ok
}

type rawMD struct {
	md    MD
	added [][]string
}
