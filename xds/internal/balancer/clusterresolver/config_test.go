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

package clusterresolver

import (
	"encoding/json"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/qiaohao9/grpc/internal/balancer/stub"
	internalserviceconfig "github.com/qiaohao9/grpc/internal/serviceconfig"
)

func TestDiscoveryMechanismTypeMarshalJSON(t *testing.T) {
	tests := []struct {
		name string
		typ  DiscoveryMechanismType
		want string
	}{
		{
			name: "eds",
			typ:  DiscoveryMechanismTypeEDS,
			want: `"EDS"`,
		},
		{
			name: "dns",
			typ:  DiscoveryMechanismTypeLogicalDNS,
			want: `"LOGICAL_DNS"`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got, err := json.Marshal(tt.typ); err != nil || string(got) != tt.want {
				t.Fatalf("DiscoveryMechanismTypeEDS.MarshalJSON() = (%v, %v), want (%s, nil)", string(got), err, tt.want)
			}
		})
	}
}
func TestDiscoveryMechanismTypeUnmarshalJSON(t *testing.T) {
	tests := []struct {
		name    string
		js      string
		want    DiscoveryMechanismType
		wantErr bool
	}{
		{
			name: "eds",
			js:   `"EDS"`,
			want: DiscoveryMechanismTypeEDS,
		},
		{
			name: "dns",
			js:   `"LOGICAL_DNS"`,
			want: DiscoveryMechanismTypeLogicalDNS,
		},
		{
			name:    "error",
			js:      `"1234"`,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got DiscoveryMechanismType
			err := json.Unmarshal([]byte(tt.js), &got)
			if (err != nil) != tt.wantErr {
				t.Fatalf("DiscoveryMechanismTypeEDS.UnmarshalJSON() error = %v, wantErr %v", err, tt.wantErr)
			}
			if diff := cmp.Diff(got, tt.want); diff != "" {
				t.Fatalf("DiscoveryMechanismTypeEDS.UnmarshalJSON() got unexpected output, diff (-got +want): %v", diff)
			}
		})
	}
}

func init() {
	// This is needed now for the config parsing tests to pass. Otherwise they
	// will fail with "RING_HASH unsupported".
	//
	// TODO: delete this once ring-hash policy is implemented and imported.
	stub.Register(rhName, stub.BalancerFuncs{})
}

const (
	testJSONConfig1 = `{
  "discoveryMechanisms": [{
    "cluster": "test-cluster-name",
    "lrsLoadReportingServerName": "test-lrs-server",
    "maxConcurrentRequests": 314,
    "type": "EDS",
    "edsServiceName": "test-eds-service-name"
  }]
}`
	testJSONConfig2 = `{
  "discoveryMechanisms": [{
    "cluster": "test-cluster-name",
    "lrsLoadReportingServerName": "test-lrs-server",
    "maxConcurrentRequests": 314,
    "type": "EDS",
    "edsServiceName": "test-eds-service-name"
  },{
    "type": "LOGICAL_DNS"
  }]
}`
	testJSONConfig3 = `{
  "discoveryMechanisms": [{
    "cluster": "test-cluster-name",
    "lrsLoadReportingServerName": "test-lrs-server",
    "maxConcurrentRequests": 314,
    "type": "EDS",
    "edsServiceName": "test-eds-service-name"
  }],
  "xdsLbPolicy":[{"ROUND_ROBIN":{}}]
}`
	testJSONConfig4 = `{
  "discoveryMechanisms": [{
    "cluster": "test-cluster-name",
    "lrsLoadReportingServerName": "test-lrs-server",
    "maxConcurrentRequests": 314,
    "type": "EDS",
    "edsServiceName": "test-eds-service-name"
  }],
  "xdsLbPolicy":[{"RING_HASH":{}}]
}`
	testJSONConfig5 = `{
  "discoveryMechanisms": [{
    "cluster": "test-cluster-name",
    "lrsLoadReportingServerName": "test-lrs-server",
    "maxConcurrentRequests": 314,
    "type": "EDS",
    "edsServiceName": "test-eds-service-name"
  }],
  "xdsLbPolicy":[{"pick_first":{}}]
}`
)

func TestParseConfig(t *testing.T) {
	tests := []struct {
		name    string
		js      string
		want    *LBConfig
		wantErr bool
	}{
		{
			name:    "empty json",
			js:      "",
			want:    nil,
			wantErr: true,
		},
		{
			name: "OK with one discovery mechanism",
			js:   testJSONConfig1,
			want: &LBConfig{
				DiscoveryMechanisms: []DiscoveryMechanism{
					{
						Cluster:                 testClusterName,
						LoadReportingServerName: newString(testLRSServer),
						MaxConcurrentRequests:   newUint32(testMaxRequests),
						Type:                    DiscoveryMechanismTypeEDS,
						EDSServiceName:          testEDSServcie,
					},
				},
				XDSLBPolicy: nil,
			},
			wantErr: false,
		},
		{
			name: "OK with multiple discovery mechanisms",
			js:   testJSONConfig2,
			want: &LBConfig{
				DiscoveryMechanisms: []DiscoveryMechanism{
					{
						Cluster:                 testClusterName,
						LoadReportingServerName: newString(testLRSServer),
						MaxConcurrentRequests:   newUint32(testMaxRequests),
						Type:                    DiscoveryMechanismTypeEDS,
						EDSServiceName:          testEDSServcie,
					},
					{
						Type: DiscoveryMechanismTypeLogicalDNS,
					},
				},
				XDSLBPolicy: nil,
			},
			wantErr: false,
		},
		{
			name: "OK with picking policy round_robin",
			js:   testJSONConfig3,
			want: &LBConfig{
				DiscoveryMechanisms: []DiscoveryMechanism{
					{
						Cluster:                 testClusterName,
						LoadReportingServerName: newString(testLRSServer),
						MaxConcurrentRequests:   newUint32(testMaxRequests),
						Type:                    DiscoveryMechanismTypeEDS,
						EDSServiceName:          testEDSServcie,
					},
				},
				XDSLBPolicy: &internalserviceconfig.BalancerConfig{
					Name:   "ROUND_ROBIN",
					Config: nil,
				},
			},
			wantErr: false,
		},
		{
			name: "OK with picking policy ring_hash",
			js:   testJSONConfig4,
			want: &LBConfig{
				DiscoveryMechanisms: []DiscoveryMechanism{
					{
						Cluster:                 testClusterName,
						LoadReportingServerName: newString(testLRSServer),
						MaxConcurrentRequests:   newUint32(testMaxRequests),
						Type:                    DiscoveryMechanismTypeEDS,
						EDSServiceName:          testEDSServcie,
					},
				},
				XDSLBPolicy: &internalserviceconfig.BalancerConfig{
					Name:   "RING_HASH",
					Config: nil,
				},
			},
			wantErr: false,
		},
		{
			name:    "unsupported picking policy",
			js:      testJSONConfig5,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseConfig([]byte(tt.js))
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
			if diff := cmp.Diff(got, tt.want); diff != "" {
				t.Errorf("parseConfig() got unexpected output, diff (-got +want): %v", diff)
			}
		})
	}
}

func newString(s string) *string {
	return &s
}

func newUint32(i uint32) *uint32 {
	return &i
}
